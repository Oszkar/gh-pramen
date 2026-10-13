package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

type summaryOptions struct {
	rowLimit   int
	focus      focusKind
	hyperlinks bool
}

// writeSummary writes the terminal summary. A focus changes only the listed
// rows, not the summary counts or the complete report.
func writeSummary(w io.Writer, r Report, opts summaryOptions) error {
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

	cc := r.Checks
	checkRows := []countRow{
		{"Success", cc.Success},
		{"Failure or error", cc.FailureOrError},
		{"Pending/expected", cc.PendingOrExpected},
		{"None reported", cc.NoneReported},
	}
	if cc.Other > 0 {
		checkRows = append(checkRows, countRow{"Other state", cc.Other})
	}
	writeCounts(&b, fmt.Sprintf("Checks on the head commit of the %d non-draft pull requests", cc.NonDraft), checkRows)

	if opts.focus != focusNone {
		linksDone := false
		writeRowSection(&b, opts.focus.label(), focusedRows(r.PullRequests, opts.focus), opts.rowLimit, true, &linksDone, opts.hyperlinks)
		_, err := io.WriteString(w, b.String())
		return err
	}

	var nonDraft, draft []Row
	for _, row := range r.PullRequests {
		if row.IsDraft {
			draft = append(draft, row)
		} else {
			nonDraft = append(nonDraft, row)
		}
	}
	linksDone := false
	writeRowSection(&b, "non-draft", nonDraft, opts.rowLimit, true, &linksDone, opts.hyperlinks)
	writeRowSection(&b, "draft", draft, opts.rowLimit, false, &linksDone, opts.hyperlinks)

	_, err := io.WriteString(w, b.String())
	return err
}

const (
	maxAuthorRunes = 24
	maxTitleRunes  = 60
)

// humanDuration shows a duration coarsely, always rounded down: <1h, Nh
// (under a day), Nd (under 90 days, so the 7, 30 and 90 day bucket edges
// stay visible), Nmo (fixed 30-day months, under a year), then Ny.
func humanDuration(seconds int64) string {
	d := time.Duration(max(seconds, 0)) * time.Second
	days := int(d / day)
	switch {
	case d < time.Hour:
		return "<1h"
	case d < day:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case days < 90:
		return fmt.Sprintf("%dd", days)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		return fmt.Sprintf("%dy", days/365)
	}
}

// reviewFacts lists the observed review facts of a PR in one cell. Approvals
// are reviewers' latest approving reviews, not an approval to merge.
// checkLabel shows a check rollup state in lower case, "-" when no checks
// are reported, and "?" for an empty state.
func checkLabel(state *string) string {
	switch {
	case state == nil:
		return "-"
	case *state == "":
		return "?"
	default:
		return cleanTitle(strings.ToLower(*state))
	}
}

func reviewFacts(pr PullRequest) string {
	var parts []string
	if pr.Approvals > 0 {
		parts = append(parts, fmt.Sprintf("approvals %d", pr.Approvals))
	}
	if pr.ChangesRequested > 0 {
		parts = append(parts, fmt.Sprintf("changes requested %d", pr.ChangesRequested))
	}
	if pr.PendingReviewRequests > 0 {
		s := fmt.Sprintf("pending requests %d", pr.PendingReviewRequests)
		if pr.PendingCodeOwnerRequests == pr.PendingReviewRequests {
			s += " (CODEOWNERS only)"
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

// authorLabel names a PR author. A nil author means GitHub no longer has the
// account; "[bot]" marks bot accounts.
func authorLabel(a *Author) string {
	if a == nil {
		return "ghost"
	}
	label := a.Login
	if a.Type == "Bot" {
		label += "[bot]"
	}
	return truncateRunes(label, maxAuthorRunes)
}

// cleanTitle makes a title safe for a terminal: control characters, such as
// newlines and escape sequences, become spaces, and long titles are cut.
func cleanTitle(title string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
	return truncateRunes(clean, maxTitleRunes)
}

// truncateRunes cuts s to at most n runes, ending in an ellipsis when cut.
// n must be >= 1.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// defaultRowLimit is how many rows each section shows unless --limit or
// --all says otherwise.
const defaultRowLimit = 20

// writeTable writes rows as aligned columns. Links wrap only the PR number,
// after widths have been calculated from plain text.
func writeTable(b *strings.Builder, header []string, rows [][]string, links []string, hyperlinks bool) {
	widths := make([]int, len(header))
	for _, row := range append([][]string{header}, rows...) {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}
	line := func(row []string, link string) {
		cells := make([]string, len(row))
		for i, cell := range row {
			if i == 0 {
				cells[i] = fmt.Sprintf("%*s", widths[i], cell)
				if hyperlinks && link != "" {
					padding := strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell))
					cells[i] = padding + terminalLink(cell, link)
				}
			} else {
				cells[i] = fmt.Sprintf("%-*s", widths[i], cell)
			}
		}
		fmt.Fprintf(b, "  %s\n", strings.TrimRight(strings.Join(cells, "  "), " "))
	}
	line(header, "")
	for i, row := range rows {
		line(row, links[i])
	}
}

func terminalLink(label, target string) string {
	const start = "\x1b]8;;"
	const end = "\x1b\\"
	return start + target + end + label + start + end
}

// pullRequestURL accepts only a well-formed HTTPS URL ending in this PR's
// number before using it in a terminal escape sequence or link template.
func pullRequestURL(row Row) string {
	number := strconv.Itoa(row.Number)
	parsed, err := url.Parse(row.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasSuffix(parsed.Path, "/pull/"+number) || !strings.HasSuffix(row.URL, "/pull/"+number) {
		return ""
	}
	return row.URL
}

// linksLine derives the URL pattern for PR links from one PR's URL.
func linksLine(row Row) (string, bool) {
	target := pullRequestURL(row)
	if target == "" {
		return "", false
	}
	return "Links: " + strings.TrimSuffix(target, strconv.Itoa(row.Number)) + "<number>", true
}

// writeRowSection writes one titled table of rows, at most limit of them
// (0 means all). The header always says how many of how many are shown.
func writeRowSection(b *strings.Builder, kind string, rows []Row, limit int, withReview bool, linksDone *bool, hyperlinks bool) {
	if len(rows) == 0 {
		fmt.Fprintf(b, "\nOpen pull requests, %s (none)\n", kind)
		return
	}
	shown := rows
	count := strconv.Itoa(len(rows))
	if limit > 0 && len(rows) > limit {
		shown = rows[:limit]
		count = fmt.Sprintf("showing %d of %d", limit, len(rows))
	}
	fmt.Fprintf(b, "\nOpen pull requests, %s (%s, oldest first by creation date)\n", kind, count)
	if !*linksDone {
		if line, ok := linksLine(shown[0]); ok {
			fmt.Fprintln(b, line)
			*linksDone = true
		}
	}
	fmt.Fprintln(b)

	header := []string{"#", "Age", "Updated", "Author", "Title"}
	if withReview {
		header = []string{"#", "Age", "Updated", "Review facts", "Checks", "Author", "Title"}
	}
	cells := make([][]string, 0, len(shown))
	links := make([]string, 0, len(shown))
	for _, row := range shown {
		cell := []string{strconv.Itoa(row.Number), humanDuration(row.AgeSeconds), humanDuration(row.SecondsSinceUpdate)}
		if withReview {
			cell = append(cell, reviewFacts(row.PullRequest), checkLabel(row.CheckState))
		}
		cell = append(cell, authorLabel(row.Author), cleanTitle(row.Title))
		cells = append(cells, cell)
		links = append(links, pullRequestURL(row))
	}
	writeTable(b, header, cells, links, hyperlinks)
}
