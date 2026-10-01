package main

import (
	"bytes"
	"strings"
	"testing"
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
