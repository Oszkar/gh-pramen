package main

import (
	"fmt"
	"time"
)

type focusKind string

const (
	focusNone             focusKind = ""
	focusOldNonDraft      focusKind = "old-non-draft"
	focusPendingRequests  focusKind = "pending-requests"
	focusChangesRequested focusKind = "changes-requested"
	focusNoReviewFacts    focusKind = "no-review-facts"
)

func parseFocus(s string) (focusKind, error) {
	focus := focusKind(s)
	switch focus {
	case focusNone, focusOldNonDraft, focusPendingRequests, focusChangesRequested, focusNoReviewFacts:
		return focus, nil
	default:
		return focusNone, fmt.Errorf("invalid --focus %q; choose old-non-draft, pending-requests, changes-requested, or no-review-facts", s)
	}
}

// focusedRows selects a factual slice of the complete, oldest-first report.
// Review facts are defined only for non-draft pull requests.
func focusedRows(rows []Row, focus focusKind) []Row {
	selected := make([]Row, 0)
	for _, row := range rows {
		if row.IsDraft {
			continue
		}
		match := false
		switch focus {
		case focusOldNonDraft:
			match = row.AgeSeconds >= int64(90*day/time.Second)
		case focusPendingRequests:
			match = row.PendingReviewRequests > 0
		case focusChangesRequested:
			match = row.ChangesRequested > 0
		case focusNoReviewFacts:
			match = row.Approvals == 0 && row.ChangesRequested == 0 && row.PendingReviewRequests == 0
		}
		if match {
			selected = append(selected, row)
		}
	}
	return selected
}

func (f focusKind) label() string {
	switch f {
	case focusOldNonDraft:
		return "non-draft, 90+ days old"
	case focusPendingRequests:
		return "non-draft, with pending review requests"
	case focusChangesRequested:
		return "non-draft, with changes requested"
	case focusNoReviewFacts:
		return "non-draft, with none of the listed review facts"
	default:
		return ""
	}
}
