package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func summaryOf(t *testing.T, prs []PullRequest, flow Flow) string {
	t.Helper()
	var out bytes.Buffer
	if err := writeSummary(&out, buildReport("acme/widgets", prs, flow, testNow)); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestWriteSummaryRightAlignsCountsOfDifferentWidths(t *testing.T) {
	var prs []PullRequest
	for i := 1; i <= 1176; i++ {
		prs = append(prs, openPR(i, day))
	}
	for i := 1177; i <= 1246; i++ {
		prs = append(prs, draftPR(i, day))
	}

	got := summaryOf(t, prs, Flow{})

	want := "Open pull requests\n" +
		"  Total      1246\n" +
		"  Non-draft  1176\n" +
		"  Draft        70\n"
	if !strings.Contains(got, want) {
		t.Errorf("summary does not contain:\n%s\ngot:\n%s", want, got)
	}
}

func TestWriteSummaryWidensAgeColumnsForLargeCounts(t *testing.T) {
	var prs []PullRequest
	for i := 1; i <= 123456; i++ {
		prs = append(prs, openPR(i, day))
	}

	got := summaryOf(t, prs, Flow{})

	want := "                 Total  Non-draft  Draft\n" +
		"  under 7 days  123456     123456      0\n" +
		"  7-29 days          0          0      0\n"
	if !strings.Contains(got, want) {
		t.Errorf("summary does not contain:\n%s\ngot:\n%s", want, got)
	}
}

func TestWriteSummaryShowsZerosForEmptyBacklog(t *testing.T) {
	got := summaryOf(t, nil, Flow{})

	for _, want := range []string{
		"  Total      0\n",
		"  Closed without merge  0\n",
		"  90+ days          0          0      0\n",
		"Review facts for the 0 non-draft pull requests",
		"  With none of these            0\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary does not contain %q; got:\n%s", want, got)
		}
	}
}

func TestWriteSummaryLeavesOutReviewDecisions(t *testing.T) {
	pr := openPR(1, day)
	pr.ReviewDecision = strPtr("APPROVED")

	got := summaryOf(t, []PullRequest{pr}, Flow{})

	for _, unwanted := range []string{"decision", "Approved", "Not reported"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("summary mentions %q, but review decisions are only in the JSON:\n%s", unwanted, got)
		}
	}
}

func TestWriteSummaryHasNoTrailingWhitespace(t *testing.T) {
	got := summaryOf(t, []PullRequest{openPR(1, day)}, Flow{Opened: 1})

	for i, line := range strings.Split(got, "\n") {
		if strings.TrimRight(line, " \t") != line {
			t.Errorf("line %d has trailing whitespace: %q", i+1, line)
		}
	}
}

func TestHumanDurationRoundsDownAtEachBoundary(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "<1h"},
		{0, "<1h"},
		{59 * time.Minute, "<1h"},
		{time.Hour, "1h"},
		{23*time.Hour + 59*time.Minute, "23h"},
		{24 * time.Hour, "1d"},
		{89*day + 23*time.Hour, "89d"},
		{90 * day, "3mo"},
		{364 * day, "12mo"},
		{365 * day, "1y"},
		{729 * day, "1y"},
		{730 * day, "2y"},
	}
	for _, tt := range tests {
		if got := humanDuration(int64(tt.d / time.Second)); got != tt.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestReviewFacts(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want string
	}{
		{"nothing observed", PullRequest{}, "-"},
		{"approvals only", PullRequest{Approvals: 2}, "approvals 2"},
		{"changes requested only", PullRequest{ChangesRequested: 1}, "changes requested 1"},
		{"pending only", PullRequest{PendingReviewRequests: 3}, "pending requests 3"},
		{"all three in order", PullRequest{Approvals: 1, ChangesRequested: 1, PendingReviewRequests: 2}, "approvals 1, changes requested 1, pending requests 2"},
		{"pending all from CODEOWNERS", PullRequest{PendingReviewRequests: 1, PendingCodeOwnerRequests: 1}, "pending requests 1 (CODEOWNERS only)"},
		{"pending partly from CODEOWNERS", PullRequest{PendingReviewRequests: 2, PendingCodeOwnerRequests: 1}, "pending requests 2"},
	}
	for _, tt := range tests {
		if got := reviewFacts(tt.pr); got != tt.want {
			t.Errorf("%s: reviewFacts = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestAuthorLabel(t *testing.T) {
	tests := []struct {
		name string
		a    *Author
		want string
	}{
		{"user", &Author{Login: "bob", Type: "User"}, "bob"},
		{"bot", &Author{Login: "dependabot", Type: "Bot"}, "dependabot[bot]"},
		{"missing account", nil, "ghost"},
		{"long login", &Author{Login: strings.Repeat("a", 30), Type: "User"}, strings.Repeat("a", 23) + "…"},
	}
	for _, tt := range tests {
		if got := authorLabel(tt.a); got != tt.want {
			t.Errorf("%s: authorLabel = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "Fix the thing", "Fix the thing"},
		{"newline and tab", "Fix\nthe\tthing", "Fix the thing"},
		{"escape sequence", "Fix \x1b[31mred\x1b[0m", "Fix  [31mred [0m"},
		{"exactly 60 runes", strings.Repeat("é", 60), strings.Repeat("é", 60)},
		{"61 runes truncated by runes not bytes", strings.Repeat("é", 61), strings.Repeat("é", 59) + "…"},
	}
	for _, tt := range tests {
		if got := cleanTitle(tt.in); got != tt.want {
			t.Errorf("%s: cleanTitle = %q, want %q", tt.name, got, tt.want)
		}
	}
}
