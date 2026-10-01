package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func writeJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

const timeLayout = "2006-01-02 15:04 MST"

type countRow struct {
	label string
	n     int
}

// writeCounts writes a titled block of labelled counts, with the labels
// left-aligned and the counts right-aligned.
func writeCounts(b *strings.Builder, title string, rows []countRow) {
	labelWidth, countWidth := 0, 0
	for _, row := range rows {
		labelWidth = max(labelWidth, len(row.label))
		countWidth = max(countWidth, len(strconv.Itoa(row.n)))
	}
	fmt.Fprintf(b, "\n%s\n", title)
	for _, row := range rows {
		fmt.Fprintf(b, "  %-*s  %*d\n", labelWidth, row.label, countWidth, row.n)
	}
}

func writeAgeDistribution(b *strings.Builder, buckets []AgeBucket) {
	headers := [3]string{"Total", "Non-draft", "Draft"}
	labelWidth := 0
	widths := [3]int{len(headers[0]), len(headers[1]), len(headers[2])}
	for _, bucket := range buckets {
		labelWidth = max(labelWidth, len(bucket.Label))
		for i, n := range [3]int{bucket.Total, bucket.NonDraft, bucket.Draft} {
			widths[i] = max(widths[i], len(strconv.Itoa(n)))
		}
	}

	fmt.Fprint(b, "\nAge of open pull requests\n")
	fmt.Fprintf(b, "  %*s  %*s  %*s  %*s\n", labelWidth, "", widths[0], headers[0], widths[1], headers[1], widths[2], headers[2])
	for _, bucket := range buckets {
		fmt.Fprintf(b, "  %-*s  %*d  %*d  %*d\n", labelWidth, bucket.Label, widths[0], bucket.Total, widths[1], bucket.NonDraft, widths[2], bucket.Draft)
	}
}

// writeSummary writes the plain-text summary of the report. It only lays
// out what the report already contains.
func writeSummary(w io.Writer, r Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository  %s\n", r.Repository)
	fmt.Fprintf(&b, "Collected   %s\n", r.CollectedAt.Format(timeLayout))

	writeCounts(&b, "Open pull requests", []countRow{
		{"Total", r.OpenBacklog.Total},
		{"Non-draft", r.OpenBacklog.NonDraft},
		{"Draft", r.OpenBacklog.Draft},
	})

	flowTitle := fmt.Sprintf("Last %d days (since %s)", r.RecentFlow.WindowDays, r.RecentFlow.Since.Format(timeLayout))
	writeCounts(&b, flowTitle, []countRow{
		{"Opened", r.RecentFlow.Opened},
		{"Merged", r.RecentFlow.Merged},
		{"Closed without merge", r.RecentFlow.ClosedUnmerged},
	})

	writeAgeDistribution(&b, r.AgeDistribution)

	// GitHub's review decision is left out on purpose: it is empty on most
	// PRs even where reviews are required, so it is only in the JSON.
	rs := r.ReviewState
	reviewTitle := fmt.Sprintf("Review facts for the %d non-draft pull requests (one PR can count in several rows)", rs.NonDraft)
	writeCounts(&b, reviewTitle, []countRow{
		{"With approvals", rs.WithApprovals},
		{"With changes requested", rs.WithChangesRequested},
		{"With pending review requests", rs.WithPendingRequests},
		{"  only from CODEOWNERS", rs.WithOnlyCodeOwnerRequests},
		{"With none of these", rs.WithNoneOfThese},
	})

	fmt.Fprint(&b, "\nPer-pull-request rows are not in this report yet; use --json for them.\n")

	_, err := io.WriteString(w, b.String())
	return err
}
