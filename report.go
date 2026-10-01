package main

import (
	"sort"
	"time"
)

// Author is the account that opened a pull request.
type Author struct {
	Login string `json:"login"`
	// Type is GitHub's account type, such as "User" or "Bot".
	Type string `json:"type"`
}

// PullRequest holds the facts fetched for one open pull request.
type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	// Author is nil when GitHub no longer has the account.
	Author      *Author   `json:"author"`
	IsDraft     bool      `json:"isDraft"`
	BaseRefName string    `json:"baseRefName"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// ReviewDecision is nil when GitHub reports no decision.
	ReviewDecision           *string `json:"reviewDecision"`
	PendingReviewRequests    int     `json:"pendingReviewRequests"`
	PendingCodeOwnerRequests int     `json:"pendingCodeOwnerRequests"`
	Approvals                int     `json:"approvals"`
	ChangesRequested         int     `json:"changesRequested"`
}

// Flow holds counts of pull requests opened, merged, and closed without
// merge since the start of the recent-flow window.
type Flow struct {
	Opened         int
	Merged         int
	ClosedUnmerged int
}

type Report struct {
	SchemaVersion   int         `json:"schemaVersion"`
	Repository      string      `json:"repository"`
	CollectedAt     time.Time   `json:"collectedAt"`
	OpenBacklog     Split       `json:"openBacklog"`
	RecentFlow      RecentFlow  `json:"recentFlow"`
	AgeDistribution []AgeBucket `json:"ageDistribution"`
	ReviewState     ReviewState `json:"reviewState"`
	PullRequests    []Row       `json:"pullRequests"`
}

type Split struct {
	Total    int `json:"total"`
	Draft    int `json:"draft"`
	NonDraft int `json:"nonDraft"`
}

type RecentFlow struct {
	WindowDays     int       `json:"windowDays"`
	Since          time.Time `json:"since"`
	Opened         int       `json:"opened"`
	Merged         int       `json:"merged"`
	ClosedUnmerged int       `json:"closedUnmerged"`
}

type AgeBucket struct {
	Label   string `json:"label"`
	MinDays int    `json:"minDays"`
	// MaxDays is inclusive; nil means the bucket has no upper bound.
	MaxDays *int `json:"maxDays"`
	Split
}

// ReviewState counts review facts over the non-draft open PRs. A PR can be
// counted in several of the With* fields; WithNoneOfThese is exclusive.
type ReviewState struct {
	NonDraft                  int            `json:"nonDraft"`
	WithApprovals             int            `json:"withApprovals"`
	WithChangesRequested      int            `json:"withChangesRequested"`
	WithPendingRequests       int            `json:"withPendingRequests"`
	WithOnlyCodeOwnerRequests int            `json:"withOnlyCodeOwnerRequests"`
	WithNoneOfThese           int            `json:"withNoneOfThese"`
	Decision                  DecisionCounts `json:"decision"`
}

type DecisionCounts struct {
	Approved         int `json:"approved"`
	ChangesRequested int `json:"changesRequested"`
	ReviewRequired   int `json:"reviewRequired"`
	NotReported      int `json:"notReported"`
	Other            int `json:"other"`
}

type Row struct {
	PullRequest
	AgeSeconds         int64 `json:"ageSeconds"`
	SecondsSinceUpdate int64 `json:"secondsSinceUpdate"`
}

const (
	schemaVersion  = 1
	flowWindowDays = 30
	day            = 24 * time.Hour
)

// flowSince returns the start of the recent-flow window ending at now.
func flowSince(now time.Time) time.Time {
	return now.Add(-flowWindowDays * day)
}

// referenceTime normalizes the time all ages are measured against.
func referenceTime(now time.Time) time.Time {
	return now.UTC().Truncate(time.Second)
}

func newAgeBuckets() []AgeBucket {
	max := func(n int) *int { return &n }
	return []AgeBucket{
		{Label: "under 7 days", MinDays: 0, MaxDays: max(6)},
		{Label: "7-29 days", MinDays: 7, MaxDays: max(29)},
		{Label: "30-89 days", MinDays: 30, MaxDays: max(89)},
		{Label: "90+ days", MinDays: 90},
	}
}

func (s *Split) add(isDraft bool) {
	s.Total++
	if isDraft {
		s.Draft++
	} else {
		s.NonDraft++
	}
}

func (s *ReviewState) add(pr PullRequest) {
	s.NonDraft++
	if pr.Approvals > 0 {
		s.WithApprovals++
	}
	if pr.ChangesRequested > 0 {
		s.WithChangesRequested++
	}
	if pr.PendingReviewRequests > 0 {
		s.WithPendingRequests++
		if pr.PendingCodeOwnerRequests == pr.PendingReviewRequests {
			s.WithOnlyCodeOwnerRequests++
		}
	}
	if pr.Approvals == 0 && pr.ChangesRequested == 0 && pr.PendingReviewRequests == 0 {
		s.WithNoneOfThese++
	}

	switch {
	case pr.ReviewDecision == nil:
		s.Decision.NotReported++
	case *pr.ReviewDecision == "APPROVED":
		s.Decision.Approved++
	case *pr.ReviewDecision == "CHANGES_REQUESTED":
		s.Decision.ChangesRequested++
	case *pr.ReviewDecision == "REVIEW_REQUIRED":
		s.Decision.ReviewRequired++
	default:
		s.Decision.Other++
	}
}

// elapsed returns the time from t to now, or zero if t is after now.
func elapsed(now, t time.Time) time.Duration {
	if t.After(now) {
		return 0
	}
	return now.Sub(t)
}

// buildReport calculates the report from fetched facts. now is the single
// reference time for every age in the report.
func buildReport(repo string, prs []PullRequest, flow Flow, now time.Time) Report {
	now = referenceTime(now)
	r := Report{
		SchemaVersion: schemaVersion,
		Repository:    repo,
		CollectedAt:   now,
		RecentFlow: RecentFlow{
			WindowDays:     flowWindowDays,
			Since:          flowSince(now),
			Opened:         flow.Opened,
			Merged:         flow.Merged,
			ClosedUnmerged: flow.ClosedUnmerged,
		},
		AgeDistribution: newAgeBuckets(),
		PullRequests:    make([]Row, 0, len(prs)),
	}

	for _, pr := range prs {
		age := elapsed(now, pr.CreatedAt)
		r.OpenBacklog.add(pr.IsDraft)

		ageDays := int(age / day)
		for i := len(r.AgeDistribution) - 1; i >= 0; i-- {
			if ageDays >= r.AgeDistribution[i].MinDays {
				r.AgeDistribution[i].add(pr.IsDraft)
				break
			}
		}

		if !pr.IsDraft {
			r.ReviewState.add(pr)
		}

		r.PullRequests = append(r.PullRequests, Row{
			PullRequest:        pr,
			AgeSeconds:         int64(age / time.Second),
			SecondsSinceUpdate: int64(elapsed(now, pr.UpdatedAt) / time.Second),
		})
	}

	sort.SliceStable(r.PullRequests, func(i, j int) bool {
		a, b := r.PullRequests[i], r.PullRequests[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.Number < b.Number
	})

	return r
}
