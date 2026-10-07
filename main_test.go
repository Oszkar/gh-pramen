package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/repository"
)

// testRun runs the command with the given arguments against canned GitHub
// responses, with testNow as the clock and no current repository.
type testRun struct {
	stdout, stderr bytes.Buffer
	fake           *fakeGitHub
	hosts          []string
	env            env
}

func newTestRun(t *testing.T, responses ...cannedResponse) *testRun {
	t.Helper()
	t.Setenv("GH_HOST", "")
	tr := &testRun{}
	client, fake := newFakeClient(t, responses...)
	tr.fake = fake
	tr.env = env{
		stdout: &tr.stdout,
		stderr: &tr.stderr,
		now:    func() time.Time { return testNow },
		currentRepo: func() (repository.Repository, error) {
			return repository.Repository{}, errors.New("not in a repository")
		},
		newClient: func(host string) (graphQLClient, error) {
			tr.hosts = append(tr.hosts, host)
			return client, nil
		},
	}
	return tr
}

func TestRunJSONWritesFullReportToStdout(t *testing.T) {
	tr := newTestRun(t,
		fixture(t, "open_prs_page1.json"),
		fixture(t, "open_prs_page2.json"),
		fixture(t, "flow.json"),
	)

	code := run([]string{"-R", "acme/widgets", "--json"}, tr.env)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, tr.stderr.String())
	}
	want, err := os.ReadFile("testdata/report.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := tr.stdout.String(); got != string(want) {
		t.Errorf("stdout differs from testdata/report.golden.json:\n%s", got)
	}
	if len(tr.hosts) != 1 || tr.hosts[0] != "github.com" {
		t.Errorf("client hosts = %v, want [github.com]", tr.hosts)
	}
}

func TestRunReportsProgressOnStderr(t *testing.T) {
	tr := newTestRun(t,
		fixture(t, "open_prs_page1.json"),
		fixture(t, "open_prs_page2.json"),
		fixture(t, "flow.json"),
	)

	run([]string{"-R", "acme/widgets", "--json"}, tr.env)

	for _, want := range []string{"3 of 5", "5 of 5"} {
		if !strings.Contains(tr.stderr.String(), want) {
			t.Errorf("stderr = %q, want it to contain %q", tr.stderr.String(), want)
		}
	}
}

func TestRunUsesFlowWindowEndingAtReferenceTime(t *testing.T) {
	tr := newTestRun(t, fixture(t, "open_prs_empty.json"), fixture(t, "flow.json"))

	run([]string{"-R", "acme/widgets", "--json"}, tr.env)

	if len(tr.fake.variables) != 2 {
		t.Fatalf("made %d requests, want 2", len(tr.fake.variables))
	}
	want := "repo:acme/widgets is:pr created:>=2026-08-31T12:00:00Z"
	if got := tr.fake.variables[1]["opened"]; got != want {
		t.Errorf("opened search = %q, want %q", got, want)
	}
}

func TestRunJSONForEmptyBacklogHasEmptyList(t *testing.T) {
	tr := newTestRun(t, fixture(t, "open_prs_empty.json"), fixture(t, "flow.json"))

	code := run([]string{"-R", "acme/widgets", "--json"}, tr.env)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, tr.stderr.String())
	}
	if !strings.Contains(tr.stdout.String(), `"pullRequests": []`) {
		t.Errorf("stdout = %s, want an empty pullRequests list", tr.stdout.String())
	}
}

func TestRunJSONKeepsTitlesReadable(t *testing.T) {
	body := `{"data":{"repository":{"pullRequests":{"totalCount":1,
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"Use <T> & drop \"legacy\" mode","url":"u","isDraft":false,"baseRefName":"main",
			"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z",
			"reviewDecision":null,"author":null,
			"reviewRequests":{"totalCount":0,"nodes":[]},
			"latestOpinionatedReviews":{"totalCount":0,"nodes":[]}}]}}}}`
	tr := newTestRun(t, cannedResponse{status: http.StatusOK, body: body}, fixture(t, "flow.json"))

	run([]string{"-R", "acme/widgets", "--json"}, tr.env)

	want := `"title": "Use <T> & drop \"legacy\" mode"`
	if !strings.Contains(tr.stdout.String(), want) {
		t.Errorf("stdout = %s, want it to contain %s", tr.stdout.String(), want)
	}
}

func TestRunUsesCurrentRepositoryWithoutRepoFlag(t *testing.T) {
	tr := newTestRun(t, fixture(t, "open_prs_empty.json"), fixture(t, "flow.json"))
	tr.env.currentRepo = func() (repository.Repository, error) {
		return repository.Repository{Host: "ghe.example.com", Owner: "acme", Name: "widgets"}, nil
	}

	code := run([]string{"--json"}, tr.env)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, tr.stderr.String())
	}
	if len(tr.hosts) != 1 || tr.hosts[0] != "ghe.example.com" {
		t.Errorf("client hosts = %v, want [ghe.example.com]", tr.hosts)
	}
	first := tr.fake.variables[0]
	if first["owner"] != "acme" || first["name"] != "widgets" {
		t.Errorf("request variables = %v, want owner acme, name widgets", first)
	}
}

func TestRunAcceptsLongRepoFlag(t *testing.T) {
	tr := newTestRun(t, fixture(t, "open_prs_empty.json"), fixture(t, "flow.json"))

	code := run([]string{"--repo", "acme/widgets", "--json"}, tr.env)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, tr.stderr.String())
	}
	if !strings.Contains(tr.stdout.String(), `"repository": "acme/widgets"`) {
		t.Errorf("stdout = %s, want repository acme/widgets", tr.stdout.String())
	}
}

func TestRunFailsWhenNoRepositoryCanBeDetermined(t *testing.T) {
	tr := newTestRun(t)

	code := run([]string{"--json"}, tr.env)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(tr.stderr.String(), "-R") {
		t.Errorf("stderr = %q, want it to suggest -R", tr.stderr.String())
	}
	if tr.stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", tr.stdout.String())
	}
}

func TestRunRejectsMalformedRepoFlag(t *testing.T) {
	tr := newTestRun(t)

	code := run([]string{"-R", "not-a-repository", "--json"}, tr.env)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(tr.stderr.String(), "not-a-repository") {
		t.Errorf("stderr = %q, want it to quote the bad value", tr.stderr.String())
	}
	if len(tr.fake.variables) != 0 {
		t.Errorf("made %d requests, want 0", len(tr.fake.variables))
	}
}

func TestRunWithoutJSONWritesSummaryToStdout(t *testing.T) {
	tr := newTestRun(t,
		fixture(t, "open_prs_page1.json"),
		fixture(t, "open_prs_page2.json"),
		fixture(t, "flow.json"),
	)

	code := run([]string{"-R", "acme/widgets"}, tr.env)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, tr.stderr.String())
	}
	want, err := os.ReadFile("testdata/report.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := tr.stdout.String(); got != string(want) {
		t.Errorf("stdout differs from testdata/report.golden.txt:\n%s", got)
	}
}

func TestRunWithoutJSONWritesNothingToStdoutWhenFetchFails(t *testing.T) {
	tr := newTestRun(t, cannedResponse{status: http.StatusBadGateway, body: `{"message":"Bad Gateway"}`})

	code := run([]string{"-R", "acme/widgets"}, tr.env)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if tr.stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", tr.stdout.String())
	}
}

func TestRunRejectsUnknownFlagsAndPositionalArguments(t *testing.T) {
	for _, args := range [][]string{{"--nope"}, {"--json", "extra"}} {
		tr := newTestRun(t)

		code := run(args, tr.env)

		if code != 2 {
			t.Errorf("run(%v) exit code = %d, want 2", args, code)
		}
		if !strings.Contains(tr.stderr.String(), "Usage") {
			t.Errorf("run(%v) stderr = %q, want usage", args, tr.stderr.String())
		}
	}
}

func TestRunWritesNothingToStdoutWhenPullRequestFetchFails(t *testing.T) {
	tr := newTestRun(t,
		fixture(t, "open_prs_page1.json"),
		cannedResponse{status: http.StatusBadGateway, body: `{"message":"Bad Gateway"}`},
	)

	code := run([]string{"-R", "acme/widgets", "--json"}, tr.env)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if tr.stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", tr.stdout.String())
	}
	if !strings.Contains(tr.stderr.String(), "502") {
		t.Errorf("stderr = %q, want the HTTP status", tr.stderr.String())
	}
}

func TestRunWritesNothingToStdoutWhenFlowFetchFails(t *testing.T) {
	tr := newTestRun(t,
		fixture(t, "open_prs_empty.json"),
		cannedResponse{status: http.StatusBadGateway, body: `{"message":"Bad Gateway"}`},
	)

	code := run([]string{"-R", "acme/widgets", "--json"}, tr.env)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if tr.stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", tr.stdout.String())
	}
}

// manyOpenPRs is a first page of n non-draft PRs numbered 1..n.
func manyOpenPRs(n int) cannedResponse {
	var nodes []string
	for i := 1; i <= n; i++ {
		nodes = append(nodes, fmt.Sprintf(`{"number":%d,"title":"PR %d","url":"https://github.com/acme/widgets/pull/%d","isDraft":false,"baseRefName":"main",
			"createdAt":"2026-01-%02dT00:00:00Z","updatedAt":"2026-01-%02dT00:00:00Z","reviewDecision":null,"author":null,
			"reviewRequests":{"totalCount":0,"nodes":[]},"latestOpinionatedReviews":{"totalCount":0,"nodes":[]}}`, i, i, i, i, i))
	}
	body := fmt.Sprintf(`{"data":{"repository":{"pullRequests":{"totalCount":%d,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[%s]}}}}`, n, strings.Join(nodes, ","))
	return cannedResponse{status: http.StatusOK, body: body}
}

func TestRunShowsTwentyRowsPerSectionByDefault(t *testing.T) {
	tr := newTestRun(t, manyOpenPRs(25), fixture(t, "flow.json"))

	code := run([]string{"-R", "acme/widgets"}, tr.env)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, tr.stderr.String())
	}
	if !strings.Contains(tr.stdout.String(), "non-draft (showing 20 of 25, oldest first") {
		t.Errorf("stdout = %s, want a cap of 20", tr.stdout.String())
	}
}

func TestRunLimitChangesRowsPerSection(t *testing.T) {
	tr := newTestRun(t, manyOpenPRs(25), fixture(t, "flow.json"))

	run([]string{"-R", "acme/widgets", "--limit", "3"}, tr.env)

	if !strings.Contains(tr.stdout.String(), "non-draft (showing 3 of 25, oldest first") {
		t.Errorf("stdout = %s, want a cap of 3", tr.stdout.String())
	}
}

func TestRunAllShowsEveryRow(t *testing.T) {
	tr := newTestRun(t, manyOpenPRs(25), fixture(t, "flow.json"))

	run([]string{"-R", "acme/widgets", "--all"}, tr.env)

	if !strings.Contains(tr.stdout.String(), "non-draft (25, oldest first") {
		t.Errorf("stdout = %s, want all 25 rows", tr.stdout.String())
	}
}

func TestRunRejectsBadRowFlagsBeforeAnyRequest(t *testing.T) {
	for _, args := range [][]string{
		{"-R", "acme/widgets", "--limit", "0"},
		{"-R", "acme/widgets", "--limit", "-1"},
		{"-R", "acme/widgets", "--limit", "5", "--all"},
		{"-R", "acme/widgets", "--limit", "20", "--all"},
		{"-R", "acme/widgets", "--json", "--limit", "0"},
		{"-R", "acme/widgets", "--json", "--limit", "5", "--all"},
	} {
		tr := newTestRun(t)

		code := run(args, tr.env)

		if code != 2 {
			t.Errorf("run(%v) exit code = %d, want 2", args, code)
		}
		if !strings.Contains(tr.stderr.String(), "Usage") {
			t.Errorf("run(%v) stderr = %q, want usage", args, tr.stderr.String())
		}
		if len(tr.fake.variables) != 0 || tr.stdout.Len() != 0 {
			t.Errorf("run(%v) made %d requests and wrote %q; want neither", args, len(tr.fake.variables), tr.stdout.String())
		}
	}
}

func TestRunJSONIgnoresValidRowFlags(t *testing.T) {
	for _, extra := range [][]string{{"--limit", "1"}, {"--all"}} {
		tr := newTestRun(t, fixture(t, "open_prs_page1.json"), fixture(t, "open_prs_page2.json"), fixture(t, "flow.json"))

		code := run(append([]string{"-R", "acme/widgets", "--json"}, extra...), tr.env)

		want, err := os.ReadFile("testdata/report.golden.json")
		if err != nil {
			t.Fatal(err)
		}
		if code != 0 || tr.stdout.String() != string(want) {
			t.Errorf("run with %v: exit %d, stdout differs from the golden JSON", extra, code)
		}
	}
}
