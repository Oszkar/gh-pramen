package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
)

func summaryOf(t *testing.T, prs []PullRequest, flow Flow) string {
	t.Helper()
	return summaryWithLimit(t, prs, flow, defaultRowLimit)
}

func summaryWithLimit(t *testing.T, prs []PullRequest, flow Flow, limit int) string {
	t.Helper()
	var out bytes.Buffer
	if err := writeSummary(&out, buildReport("acme/widgets", prs, flow, testNow), summaryOptions{rowLimit: limit}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func numberedPR(number int, age time.Duration, isDraft bool) PullRequest {
	pr := openPR(number, age)
	pr.IsDraft = isDraft
	pr.Title = "Title " + strconv.Itoa(number)
	pr.URL = "https://github.com/acme/widgets/pull/" + strconv.Itoa(number)
	return pr
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

func TestWriteSummaryListsNonDraftsBeforeDraftsOldestFirst(t *testing.T) {
	prs := []PullRequest{
		numberedPR(2, 10*day, true),
		numberedPR(3, 5*day, false),
		numberedPR(4, 20*day, false),
	}

	got := summaryOf(t, prs, Flow{})

	nonDraft := strings.Index(got, "Open pull requests, non-draft (2, oldest first by creation date)")
	draft := strings.Index(got, "Open pull requests, draft (1, oldest first by creation date)")
	if nonDraft < 0 || draft < nonDraft {
		t.Fatalf("want non-draft section before draft section; got:\n%s", got)
	}
	idx4, idx3 := strings.Index(got, "Title 4"), strings.Index(got, "Title 3")
	if idx4 < 0 || idx3 < 0 {
		t.Fatalf("want both Title 4 and Title 3 listed; got:\n%s", got)
	}
	if idx4 > idx3 {
		t.Errorf("want #4 (20 days old) listed before #3 (5 days old); got:\n%s", got)
	}
	if strings.Contains(got, "not in this report yet") {
		t.Errorf("summary still says rows are missing:\n%s", got)
	}
}

func TestWriteSummaryCapsEachSectionAndSaysSo(t *testing.T) {
	var prs []PullRequest
	for i := 1; i <= 5; i++ {
		prs = append(prs, numberedPR(i, time.Duration(10-i)*day, false))
	}
	for i := 6; i <= 8; i++ {
		prs = append(prs, numberedPR(i, time.Duration(10-i)*day, true))
	}

	got := summaryWithLimit(t, prs, Flow{}, 2)

	for _, want := range []string{
		"Open pull requests, non-draft (showing 2 of 5, oldest first by creation date)",
		"Open pull requests, draft (showing 2 of 3, oldest first by creation date)",
		"Title 1", "Title 2", "Title 6", "Title 7",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary does not contain %q; got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Title 3", "Title 8"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("summary contains %q beyond the limit:\n%s", unwanted, got)
		}
	}
}

func TestWriteSummaryRowsEqualToLimitDoNotSayShowing(t *testing.T) {
	prs := []PullRequest{
		numberedPR(1, 3*day, false),
		numberedPR(2, 2*day, false),
		numberedPR(3, 1*day, false),
	}

	got := summaryWithLimit(t, prs, Flow{}, 3)

	if !strings.Contains(got, "non-draft (3, oldest first by creation date)") {
		t.Errorf("want plain count header; got:\n%s", got)
	}
	if strings.Contains(got, "showing") {
		t.Errorf("summary says showing although every row is shown:\n%s", got)
	}
}

func TestWriteSummaryLimitZeroShowsEveryRow(t *testing.T) {
	var prs []PullRequest
	for i := 1; i <= 30; i++ {
		prs = append(prs, numberedPR(i, time.Duration(40-i)*day, false))
	}

	got := summaryWithLimit(t, prs, Flow{}, 0)

	if !strings.Contains(got, "non-draft (30, oldest first") || !strings.Contains(got, "Title 30") {
		t.Errorf("want all 30 rows listed; got:\n%s", got)
	}
}

func TestWriteSummaryEmptySectionsSaySoWithoutTableOrLinks(t *testing.T) {
	got := summaryOf(t, nil, Flow{})

	for _, want := range []string{
		"Open pull requests, non-draft (none)\n",
		"Open pull requests, draft (none)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary does not contain %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Links:") || strings.Contains(got, "Title") {
		t.Errorf("empty report should have no links line or table header:\n%s", got)
	}
}

func TestWriteSummaryShowsLinksPatternOnceUnderFirstSectionWithRows(t *testing.T) {
	prs := []PullRequest{numberedPR(7, 3*day, true), numberedPR(8, 2*day, true)}

	got := summaryOf(t, prs, Flow{})

	if n := strings.Count(got, "Links:"); n != 1 {
		t.Fatalf("Links line appears %d times, want 1; got:\n%s", n, got)
	}
	want := "Open pull requests, draft (2, oldest first by creation date)\nLinks: https://github.com/acme/widgets/pull/<number>\n"
	if !strings.Contains(got, want) {
		t.Errorf("summary does not contain %q; got:\n%s", want, got)
	}
}

func TestWriteSummaryOmitsLinksWhenURLDoesNotEndInNumber(t *testing.T) {
	pr := numberedPR(7, day, false)
	pr.URL = "https://example.com/somewhere/else"

	got := summaryOf(t, []PullRequest{pr}, Flow{})

	if strings.Contains(got, "Links:") {
		t.Errorf("want no Links line for an unrecognised URL; got:\n%s", got)
	}
}

func TestWriteSummaryAlignsRowsByRuneCount(t *testing.T) {
	a := numberedPR(1, 3*day, false)
	a.Title = "x"
	a.Author = &Author{Login: "élodie", Type: "User"}
	b := numberedPR(10, 2*day, false)
	b.Title = "y"
	b.Author = &Author{Login: "bob", Type: "User"}

	got := summaryOf(t, []PullRequest{a, b}, Flow{})

	if !strings.Contains(got, "   1  3d   3d       -             -       élodie  x\n") ||
		!strings.Contains(got, "  10  2d   2d       -             -       bob     y\n") {
		t.Errorf("rows are not aligned by rune count; got:\n%s", got)
	}
}

func TestFocusedRowsUseExistingFactsAndKeepOldestFirst(t *testing.T) {
	prs := []PullRequest{
		numberedPR(1, 100*day, true),
		numberedPR(2, 90*day-time.Second, false),
		numberedPR(3, 90*day, false),
		numberedPR(4, 100*day, false),
	}
	prs[2].PendingReviewRequests = 1
	prs[3].ChangesRequested = 1
	prs[3].PendingReviewRequests = 1
	rows := buildReport("acme/widgets", prs, Flow{}, testNow).PullRequests
	tests := []struct {
		focus focusKind
		want  []int
	}{
		{focusOldNonDraft, []int{4, 3}},
		{focusPendingRequests, []int{4, 3}},
		{focusChangesRequested, []int{4}},
		{focusNoReviewFacts, []int{2}},
	}
	for _, tt := range tests {
		selected := focusedRows(rows, tt.focus)
		if len(selected) != len(tt.want) {
			t.Errorf("%s: got %d rows, want %d", tt.focus, len(selected), len(tt.want))
			continue
		}
		for i, row := range selected {
			if row.Number != tt.want[i] {
				t.Errorf("%s: row %d = #%d, want #%d", tt.focus, i, row.Number, tt.want[i])
			}
		}
	}
}

func TestWriteSummaryFocusWithNoMatchesShowsNone(t *testing.T) {
	var out bytes.Buffer
	r := buildReport("acme/widgets", []PullRequest{numberedPR(1, day, false)}, Flow{}, testNow)
	if err := writeSummary(&out, r, summaryOptions{rowLimit: 20, focus: focusChangesRequested}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Open pull requests, non-draft, with changes requested (none)\n") {
		t.Errorf("missing empty focus section:\n%s", got)
	}
	if strings.Contains(got, "Open pull requests, draft (") || strings.Contains(got, "Links:") {
		t.Errorf("unexpected default section or links:\n%s", got)
	}
}

func TestTerminalHyperlinksWrapNumbersWithoutChangingColumnSpacing(t *testing.T) {
	pr := numberedPR(7, day, false)
	var out bytes.Buffer
	r := buildReport("acme/widgets", []PullRequest{pr}, Flow{}, testNow)
	if err := writeSummary(&out, r, summaryOptions{rowLimit: 20, hyperlinks: true}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	link := "\x1b]8;;https://github.com/acme/widgets/pull/7\x1b\\7\x1b]8;;\x1b\\"
	if !strings.Contains(got, "  "+link+"  1d") {
		t.Errorf("number is not linked in its aligned cell:\n%q", got)
	}
	if strings.Count(got, "\x1b]8;;") != 2 {
		t.Errorf("expected one opening and one closing hyperlink: %q", got)
	}
}

func TestTerminalHyperlinksRejectUnsafeURL(t *testing.T) {
	pr := numberedPR(7, day, false)
	pr.URL = "https://github.com/acme/widgets/pull/7\x1b]8;;https://bad.example"
	var out bytes.Buffer
	r := buildReport("acme/widgets", []PullRequest{pr}, Flow{}, testNow)
	if err := writeSummary(&out, r, summaryOptions{rowLimit: 20, hyperlinks: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "Links:") {
		t.Errorf("unsafe URL reached terminal output: %q", out.String())
	}
}

func TestWriteSummaryShowsCheckCountsForNonDraftPRs(t *testing.T) {
	prs := []PullRequest{numberedPR(1, day, false), numberedPR(2, day, false), numberedPR(3, day, true)}
	prs[0].CheckState = strPtr("FAILURE")
	prs[2].CheckState = strPtr("FAILURE")

	got := summaryOf(t, prs, Flow{})

	want := "\nChecks on the head commit of the 2 non-draft pull requests\n" +
		"  Success           0\n" +
		"  Failure or error  1\n" +
		"  Pending/expected  0\n" +
		"  None reported     1\n"
	if !strings.Contains(got, want) {
		t.Errorf("summary missing check block %q:\n%s", want, got)
	}
	if strings.Contains(got, "  Other") {
		t.Errorf("summary shows an Other line with nothing in it:\n%s", got)
	}
}

func TestWriteSummaryShowsOtherCheckStatesOnlyWhenPresent(t *testing.T) {
	pr := numberedPR(1, day, false)
	pr.CheckState = strPtr("SOMETHING_NEW")

	got := summaryOf(t, []PullRequest{pr}, Flow{})

	if !strings.Contains(got, "  Other state       1\n") {
		t.Errorf("summary missing Other state line:\n%s", got)
	}
}

func TestCheckLabel(t *testing.T) {
	for state, want := range map[string]string{
		"SUCCESS": "success", "FAILURE": "failure", "ERROR": "error",
		"PENDING": "pending", "EXPECTED": "expected", "SOMETHING_NEW": "something_new", "": "?",
	} {
		if got := checkLabel(&state); got != want {
			t.Errorf("checkLabel(%q) = %q, want %q", state, got, want)
		}
	}
	if got := checkLabel(nil); got != "-" {
		t.Errorf("checkLabel(nil) = %q, want -", got)
	}
}

func TestWriteSummaryShowsChecksColumnOnNonDraftRowsOnly(t *testing.T) {
	nonDraft := numberedPR(1, day, false)
	nonDraft.CheckState = strPtr("PENDING")
	draft := numberedPR(2, day, true)
	draft.CheckState = strPtr("SUCCESS")

	got := summaryOf(t, []PullRequest{nonDraft, draft}, Flow{})

	if !strings.Contains(got, "#  Age  Updated  Review facts  Checks   Author  Title\n") ||
		!strings.Contains(got, "1  1d   1d       -             pending  ghost   Title 1\n") {
		t.Errorf("non-draft rows missing Checks column:\n%s", got)
	}
	if !strings.Contains(got, "#  Age  Updated  Author  Title\n") || strings.Contains(got, "success") {
		t.Errorf("draft rows changed:\n%s", got)
	}
}

func TestWriteSummaryFocusKeepsCheckBlockAndColumn(t *testing.T) {
	pr := numberedPR(1, 100*day, false)
	pr.CheckState = strPtr("SUCCESS")
	var out bytes.Buffer
	r := buildReport("acme/widgets", []PullRequest{pr}, Flow{}, testNow)
	if err := writeSummary(&out, r, summaryOptions{rowLimit: 20, focus: focusOldNonDraft}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Checks on the head commit of the 1 non-draft pull requests") ||
		!strings.Contains(got, "Checks") || !strings.Contains(got, "success") {
		t.Errorf("focused view missing check block or column:\n%s", got)
	}
}
