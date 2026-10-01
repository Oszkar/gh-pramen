package main

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// openPR returns a non-draft PR created the given duration before testNow.
func openPR(number int, age time.Duration) PullRequest {
	created := testNow.Add(-age)
	return PullRequest{Number: number, CreatedAt: created, UpdatedAt: created}
}

func draftPR(number int, age time.Duration) PullRequest {
	p := openPR(number, age)
	p.IsDraft = true
	return p
}

func strPtr(s string) *string { return &s }

func TestBuildReportHeader(t *testing.T) {
	local := time.FixedZone("JST", 9*60*60)
	now := time.Date(2026, 9, 30, 21, 0, 0, 123456789, local)

	r := buildReport("owner/repo", nil, Flow{}, now)

	if r.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", r.SchemaVersion)
	}
	if r.Repository != "owner/repo" {
		t.Errorf("Repository = %q, want owner/repo", r.Repository)
	}
	want := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if !r.CollectedAt.Equal(want) || r.CollectedAt.Location() != time.UTC || r.CollectedAt.Nanosecond() != 0 {
		t.Errorf("CollectedAt = %v, want %v in UTC with whole seconds", r.CollectedAt, want)
	}
}

func TestBuildReportCountsDraftAndNonDraft(t *testing.T) {
	prs := []PullRequest{openPR(1, day), draftPR(2, day), openPR(3, day)}

	r := buildReport("o/r", prs, Flow{}, testNow)

	want := Split{Total: 3, Draft: 1, NonDraft: 2}
	if r.OpenBacklog != want {
		t.Errorf("OpenBacklog = %+v, want %+v", r.OpenBacklog, want)
	}
}

func TestBuildReportAgeBucketBoundaries(t *testing.T) {
	prs := []PullRequest{
		openPR(1, 0),
		openPR(2, 7*day-time.Second),
		draftPR(3, 7*day),
		openPR(4, 30*day-time.Second),
		openPR(5, 30*day),
		draftPR(6, 90*day-time.Second),
		openPR(7, 90*day),
		draftPR(8, 900*day),
	}

	r := buildReport("o/r", prs, Flow{}, testNow)

	max := func(n int) *int { return &n }
	want := []AgeBucket{
		{Label: "under 7 days", MinDays: 0, MaxDays: max(6), Split: Split{Total: 2, Draft: 0, NonDraft: 2}},
		{Label: "7-29 days", MinDays: 7, MaxDays: max(29), Split: Split{Total: 2, Draft: 1, NonDraft: 1}},
		{Label: "30-89 days", MinDays: 30, MaxDays: max(89), Split: Split{Total: 2, Draft: 1, NonDraft: 1}},
		{Label: "90+ days", MinDays: 90, MaxDays: nil, Split: Split{Total: 2, Draft: 1, NonDraft: 1}},
	}
	if len(r.AgeDistribution) != len(want) {
		t.Fatalf("got %d buckets, want %d", len(r.AgeDistribution), len(want))
	}
	for i, w := range want {
		g := r.AgeDistribution[i]
		if g.Label != w.Label || g.MinDays != w.MinDays || g.Split != w.Split {
			t.Errorf("bucket %d = %+v, want %+v", i, g, w)
		}
		if (g.MaxDays == nil) != (w.MaxDays == nil) || (g.MaxDays != nil && *g.MaxDays != *w.MaxDays) {
			t.Errorf("bucket %d MaxDays = %v, want %v", i, g.MaxDays, w.MaxDays)
		}
	}
}

func TestBuildReportRowDurationsAreSeconds(t *testing.T) {
	p := openPR(1, 10*day)
	p.UpdatedAt = testNow.Add(-90 * time.Minute)

	r := buildReport("o/r", []PullRequest{p}, Flow{}, testNow)

	row := r.PullRequests[0]
	if row.AgeSeconds != 864000 {
		t.Errorf("AgeSeconds = %d, want 864000", row.AgeSeconds)
	}
	if row.SecondsSinceUpdate != 5400 {
		t.Errorf("SecondsSinceUpdate = %d, want 5400", row.SecondsSinceUpdate)
	}
}

func TestBuildReportClampsTimestampsAfterReferenceTime(t *testing.T) {
	p := openPR(1, -5*time.Minute)

	r := buildReport("o/r", []PullRequest{p}, Flow{}, testNow)

	row := r.PullRequests[0]
	if row.AgeSeconds != 0 || row.SecondsSinceUpdate != 0 {
		t.Errorf("durations = %d, %d, want 0, 0", row.AgeSeconds, row.SecondsSinceUpdate)
	}
	if r.AgeDistribution[0].Total != 1 {
		t.Errorf("under-7-days bucket total = %d, want 1", r.AgeDistribution[0].Total)
	}
}

func TestBuildReportListsOldestFirstWithoutReorderingInput(t *testing.T) {
	prs := []PullRequest{openPR(3, day), openPR(1, 100*day), openPR(5, 10*day), openPR(4, 10*day)}

	r := buildReport("o/r", prs, Flow{}, testNow)

	var got []int
	for _, row := range r.PullRequests {
		got = append(got, row.Number)
	}
	want := []int{1, 4, 5, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if prs[0].Number != 3 {
		t.Errorf("input slice was reordered: first is #%d", prs[0].Number)
	}
}

func TestBuildReportReviewFactsCoverNonDraftPRsAndMayOverlap(t *testing.T) {
	approved := openPR(1, day)
	approved.Approvals = 2
	approvedAndRequested := openPR(2, day)
	approvedAndRequested.Approvals = 1
	approvedAndRequested.PendingReviewRequests = 3
	approvedAndRequested.PendingCodeOwnerRequests = 2
	changes := openPR(3, day)
	changes.ChangesRequested = 1
	nothing := openPR(4, day)
	onlyCodeOwners := openPR(5, day)
	onlyCodeOwners.PendingReviewRequests = 2
	onlyCodeOwners.PendingCodeOwnerRequests = 2
	draftWithApproval := draftPR(6, day)
	draftWithApproval.Approvals = 1
	draftWithNothing := draftPR(7, day)

	r := buildReport("o/r", []PullRequest{approved, approvedAndRequested, changes, nothing, onlyCodeOwners, draftWithApproval, draftWithNothing}, Flow{}, testNow)

	got := r.ReviewState
	want := ReviewState{
		NonDraft:                  5,
		WithApprovals:             2,
		WithChangesRequested:      1,
		WithPendingRequests:       2,
		WithOnlyCodeOwnerRequests: 1,
		WithNoneOfThese:           1,
		Decision:                  DecisionCounts{NotReported: 5},
	}
	if got != want {
		t.Errorf("ReviewState = %+v, want %+v", got, want)
	}
}

func TestBuildReportDecisionCountsCoverNonDraftPRsAndKeepNotReportedSeparate(t *testing.T) {
	withDecision := func(n int, d *string) PullRequest {
		p := openPR(n, day)
		p.ReviewDecision = d
		return p
	}
	draft := draftPR(7, day)
	draft.ReviewDecision = strPtr("REVIEW_REQUIRED")
	prs := []PullRequest{
		withDecision(1, strPtr("APPROVED")),
		withDecision(2, strPtr("CHANGES_REQUESTED")),
		withDecision(3, strPtr("REVIEW_REQUIRED")),
		withDecision(4, strPtr("REVIEW_REQUIRED")),
		withDecision(5, nil),
		withDecision(6, strPtr("SOMETHING_NEW")),
		draft,
	}

	r := buildReport("o/r", prs, Flow{}, testNow)

	want := DecisionCounts{Approved: 1, ChangesRequested: 1, ReviewRequired: 2, NotReported: 1, Other: 1}
	if r.ReviewState.Decision != want {
		t.Errorf("Decision = %+v, want %+v", r.ReviewState.Decision, want)
	}
}

func TestBuildReportRecentFlowCoversThirtyDaysBeforeReferenceTime(t *testing.T) {
	r := buildReport("o/r", nil, Flow{Opened: 34, Merged: 25, ClosedUnmerged: 3}, testNow)

	want := RecentFlow{
		WindowDays:     30,
		Since:          time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
		Opened:         34,
		Merged:         25,
		ClosedUnmerged: 3,
	}
	if r.RecentFlow != want {
		t.Errorf("RecentFlow = %+v, want %+v", r.RecentFlow, want)
	}
}

func TestBuildReportEmptyRepositoryHasZeroCountsAndAllBuckets(t *testing.T) {
	r := buildReport("o/r", nil, Flow{}, testNow)

	if r.OpenBacklog != (Split{}) {
		t.Errorf("OpenBacklog = %+v, want zeros", r.OpenBacklog)
	}
	if len(r.AgeDistribution) != 4 {
		t.Errorf("got %d age buckets, want 4", len(r.AgeDistribution))
	}
	if r.PullRequests == nil || len(r.PullRequests) != 0 {
		t.Errorf("PullRequests = %#v, want an empty non-nil slice", r.PullRequests)
	}
}
