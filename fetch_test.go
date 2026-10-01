package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// cannedResponse is one HTTP response served by fakeGitHub.
type cannedResponse struct {
	status int
	body   string
}

// fakeGitHub is an http.RoundTripper that serves canned responses in order
// and records the GraphQL variables of each request.
type fakeGitHub struct {
	t         *testing.T
	responses []cannedResponse
	variables []map[string]interface{}
}

func (f *fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	var payload struct {
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		f.t.Fatalf("decoding request body: %v", err)
	}
	f.variables = append(f.variables, payload.Variables)

	if len(f.responses) == 0 {
		f.t.Fatalf("unexpected request %d with variables %v", len(f.variables), payload.Variables)
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	return &http.Response{
		StatusCode: next.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(next.body)),
		Request:    req,
	}, nil
}

func fixture(t *testing.T, name string) cannedResponse {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return cannedResponse{status: http.StatusOK, body: string(body)}
}

// newFakeClient returns a real go-gh GraphQL client whose transport serves
// the given responses.
func newFakeClient(t *testing.T, responses ...cannedResponse) (*api.GraphQLClient, *fakeGitHub) {
	t.Helper()
	fake := &fakeGitHub{t: t, responses: responses}
	client, err := api.NewGraphQLClient(api.ClientOptions{
		Host:         "github.com",
		AuthToken:    "test-token",
		Transport:    fake,
		LogIgnoreEnv: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, fake
}

func noProgress(int, int) {}

func TestFetchOpenPRsFollowsCursorsAcrossPages(t *testing.T) {
	client, fake := newFakeClient(t, fixture(t, "open_prs_page1.json"), fixture(t, "open_prs_page2.json"))

	prs, err := fetchOpenPRs(client, "acme", "widgets", noProgress)
	if err != nil {
		t.Fatal(err)
	}

	var got []int
	for _, pr := range prs {
		got = append(got, pr.Number)
	}
	want := []int{101, 140, 152, 160, 171}
	if len(got) != len(want) {
		t.Fatalf("numbers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("numbers = %v, want %v", got, want)
		}
	}

	if len(fake.variables) != 2 {
		t.Fatalf("made %d requests, want 2", len(fake.variables))
	}
	first, second := fake.variables[0], fake.variables[1]
	if first["owner"] != "acme" || first["name"] != "widgets" || first["after"] != nil {
		t.Errorf("first request variables = %v, want owner acme, name widgets, no cursor", first)
	}
	if second["after"] != "CURSOR1" {
		t.Errorf("second request cursor = %v, want CURSOR1", second["after"])
	}
}

func TestFetchOpenPRsMapsFields(t *testing.T) {
	client, _ := newFakeClient(t, fixture(t, "open_prs_page1.json"), fixture(t, "open_prs_page2.json"))

	prs, err := fetchOpenPRs(client, "acme", "widgets", noProgress)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("basic fields", func(t *testing.T) {
		pr := prs[0]
		if pr.Title != "Rework the import pipeline" ||
			pr.URL != "https://github.com/acme/widgets/pull/101" ||
			!pr.IsDraft ||
			pr.BaseRefName != "main" ||
			!pr.CreatedAt.Equal(time.Date(2024, 3, 10, 8, 0, 0, 0, time.UTC)) ||
			!pr.UpdatedAt.Equal(time.Date(2024, 11, 2, 16, 30, 0, 0, time.UTC)) {
			t.Errorf("PR 101 = %+v", pr)
		}
		if pr.Author == nil || pr.Author.Login != "alice" || pr.Author.Type != "User" {
			t.Errorf("PR 101 author = %+v, want alice (User)", pr.Author)
		}
	})

	t.Run("empty decision with reviews present", func(t *testing.T) {
		pr := prs[1]
		if pr.ReviewDecision != nil {
			t.Errorf("PR 140 decision = %q, want none", *pr.ReviewDecision)
		}
		if pr.Approvals != 1 || pr.ChangesRequested != 0 {
			t.Errorf("PR 140 approvals, changes requested = %d, %d, want 1, 0", pr.Approvals, pr.ChangesRequested)
		}
		if pr.PendingReviewRequests != 2 || pr.PendingCodeOwnerRequests != 1 {
			t.Errorf("PR 140 requests, code owner requests = %d, %d, want 2, 1", pr.PendingReviewRequests, pr.PendingCodeOwnerRequests)
		}
	})

	t.Run("bot author", func(t *testing.T) {
		pr := prs[2]
		if pr.Author == nil || pr.Author.Type != "Bot" || pr.Author.Login != "dependabot" {
			t.Errorf("PR 152 author = %+v, want dependabot (Bot)", pr.Author)
		}
		if pr.ReviewDecision == nil || *pr.ReviewDecision != "REVIEW_REQUIRED" {
			t.Errorf("PR 152 decision = %v, want REVIEW_REQUIRED", pr.ReviewDecision)
		}
	})

	t.Run("unavailable author and mixed reviews", func(t *testing.T) {
		pr := prs[3]
		if pr.Author != nil {
			t.Errorf("PR 160 author = %+v, want nil", pr.Author)
		}
		if pr.Approvals != 1 || pr.ChangesRequested != 1 {
			t.Errorf("PR 160 approvals, changes requested = %d, %d, want 1, 1", pr.Approvals, pr.ChangesRequested)
		}
		if pr.BaseRefName != "release/1.2" {
			t.Errorf("PR 160 base = %q, want release/1.2", pr.BaseRefName)
		}
	})
}

func TestFetchOpenPRsReportsProgressPerPage(t *testing.T) {
	client, _ := newFakeClient(t, fixture(t, "open_prs_page1.json"), fixture(t, "open_prs_page2.json"))

	var calls [][2]int
	_, err := fetchOpenPRs(client, "acme", "widgets", func(fetched, total int) {
		calls = append(calls, [2]int{fetched, total})
	})
	if err != nil {
		t.Fatal(err)
	}

	want := [][2]int{{3, 5}, {5, 5}}
	if len(calls) != len(want) || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("progress calls = %v, want %v", calls, want)
	}
}

func TestFetchOpenPRsSucceedsWithNoOpenPRs(t *testing.T) {
	client, _ := newFakeClient(t, fixture(t, "open_prs_empty.json"))

	prs, err := fetchOpenPRs(client, "acme", "widgets", noProgress)

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(prs) != 0 {
		t.Errorf("got %d PRs, want 0", len(prs))
	}
}

func TestFetchOpenPRsFailsWithoutPartialResultWhenLaterPageFails(t *testing.T) {
	client, _ := newFakeClient(t,
		fixture(t, "open_prs_page1.json"),
		cannedResponse{status: http.StatusBadGateway, body: `{"message":"Bad Gateway"}`},
	)

	prs, err := fetchOpenPRs(client, "acme", "widgets", noProgress)

	if err == nil {
		t.Fatal("err = nil, want an error for the failed second page")
	}
	if prs != nil {
		t.Errorf("got %d PRs alongside the error, want none", len(prs))
	}
	if !strings.Contains(err.Error(), "acme/widgets") || !strings.Contains(err.Error(), "502") {
		t.Errorf("err = %q, want it to name the repository and the HTTP status", err)
	}
}

func TestFetchOpenPRsExplainsRepositoryNotFound(t *testing.T) {
	client, _ := newFakeClient(t, fixture(t, "repository_not_found.json"))

	_, err := fetchOpenPRs(client, "acme", "widgets", noProgress)

	if err == nil {
		t.Fatal("err = nil, want an error")
	}
	for _, want := range []string{"acme/widgets", "gh auth status"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to mention %q", err, want)
		}
	}
}

func TestFetchOpenPRsFailsWhenReviewDataIsTruncated(t *testing.T) {
	body := `{"data":{"repository":{"pullRequests":{"totalCount":1,
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"t","url":"u","isDraft":false,"baseRefName":"main",
			"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z",
			"reviewDecision":null,"author":null,
			"reviewRequests":{"totalCount":51,"nodes":[{"asCodeOwner":false}]},
			"latestOpinionatedReviews":{"totalCount":0,"nodes":[]}}]}}}}`
	client, _ := newFakeClient(t, cannedResponse{status: http.StatusOK, body: body})

	prs, err := fetchOpenPRs(client, "acme", "widgets", noProgress)

	if err == nil {
		t.Fatalf("err = nil with %d PRs, want an error for truncated review requests", len(prs))
	}
	if !strings.Contains(err.Error(), "#7") {
		t.Errorf("err = %q, want it to name PR #7", err)
	}
}

func TestFetchFlowReadsCountsForWindow(t *testing.T) {
	client, fake := newFakeClient(t, fixture(t, "flow.json"))
	since := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	flow, err := fetchFlow(client, "acme", "widgets", since)
	if err != nil {
		t.Fatal(err)
	}

	want := Flow{Opened: 34, Merged: 25, ClosedUnmerged: 3}
	if flow != want {
		t.Errorf("flow = %+v, want %+v", flow, want)
	}

	if len(fake.variables) != 1 {
		t.Fatalf("made %d requests, want 1", len(fake.variables))
	}
	wantQueries := map[string]string{
		"opened":         "repo:acme/widgets is:pr created:>=2026-08-31T12:00:00Z",
		"merged":         "repo:acme/widgets is:pr is:merged merged:>=2026-08-31T12:00:00Z",
		"closedUnmerged": "repo:acme/widgets is:pr is:closed is:unmerged closed:>=2026-08-31T12:00:00Z",
	}
	for name, want := range wantQueries {
		if got := fake.variables[0][name]; got != want {
			t.Errorf("search query %s = %q, want %q", name, got, want)
		}
	}
}

func TestFetchFlowFailsOnHTTPError(t *testing.T) {
	client, _ := newFakeClient(t, cannedResponse{status: http.StatusBadGateway, body: `{"message":"Bad Gateway"}`})

	_, err := fetchFlow(client, "acme", "widgets", testNow)

	if err == nil {
		t.Fatal("err = nil, want an error")
	}
	if !strings.Contains(err.Error(), "acme/widgets") {
		t.Errorf("err = %q, want it to name the repository", err)
	}
}
