package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// graphQLClient is the part of go-gh's GraphQL client that fetching uses.
type graphQLClient interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

// nestedLimit bounds the review requests and latest reviews read per PR. It
// must match the literal in openPRsQuery.
const nestedLimit = 50

// openPRsPageSize is the number of pull requests read per request. Asking
// for the head commit's check rollup makes each page slower; the size was
// chosen from measured full runs (see docs/architecture.md).
const openPRsPageSize = 50

// Pull requests come from the repository connection, not search, so the
// 1000-result search cap does not apply. Merge-conflict status is left out on
// purpose: asking for it in bulk makes the API time out.
const openPRsQuery = `
query($owner: String!, $name: String!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: $first, after: $after, orderBy: {field: CREATED_AT, direction: ASC}) {
      totalCount
      pageInfo { hasNextPage endCursor }
      nodes {
        number
        title
        url
        isDraft
        baseRefName
        createdAt
        updatedAt
        reviewDecision
        author { __typename login }
        commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
        reviewRequests(first: 50) { totalCount nodes { asCodeOwner } }
        latestOpinionatedReviews(first: 50) { totalCount nodes { state } }
      }
    }
  }
}`

type openPRsResponse struct {
	Repository *struct {
		PullRequests struct {
			TotalCount int
			PageInfo   struct {
				HasNextPage bool
				EndCursor   *string
			}
			Nodes []prNode
		}
	}
}

type prNode struct {
	Number         int
	Title          string
	URL            string
	IsDraft        bool
	BaseRefName    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ReviewDecision *string
	Author         *struct {
		Typename string `json:"__typename"`
		Login    string
	}
	// Commits holds the head commit; a nil rollup means no checks reported.
	Commits *struct {
		Nodes []struct {
			Commit *struct {
				StatusCheckRollup *struct{ State string }
			}
		}
	}
	ReviewRequests struct {
		TotalCount int
		Nodes      []struct{ AsCodeOwner bool }
	}
	LatestOpinionatedReviews struct {
		TotalCount int
		Nodes      []struct{ State string }
	}
}

func (n prNode) toPullRequest() (PullRequest, error) {
	if n.ReviewRequests.TotalCount > len(n.ReviewRequests.Nodes) ||
		n.LatestOpinionatedReviews.TotalCount > len(n.LatestOpinionatedReviews.Nodes) {
		return PullRequest{}, fmt.Errorf("pull request #%d has more than %d review requests or reviews, which is not supported", n.Number, nestedLimit)
	}
	// Only an explicit null rollup on a known head commit means no checks
	// were reported; anything less is missing data, not an absence.
	if n.Commits == nil || len(n.Commits.Nodes) != 1 || n.Commits.Nodes[0].Commit == nil {
		return PullRequest{}, fmt.Errorf("could not read the head commit of pull request #%d", n.Number)
	}

	pr := PullRequest{
		Number:                n.Number,
		Title:                 n.Title,
		URL:                   n.URL,
		IsDraft:               n.IsDraft,
		BaseRefName:           n.BaseRefName,
		CreatedAt:             n.CreatedAt,
		UpdatedAt:             n.UpdatedAt,
		ReviewDecision:        n.ReviewDecision,
		PendingReviewRequests: n.ReviewRequests.TotalCount,
	}
	if rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup; rollup != nil {
		pr.CheckState = &rollup.State
	}
	if n.Author != nil {
		pr.Author = &Author{Login: n.Author.Login, Type: n.Author.Typename}
	}
	for _, r := range n.ReviewRequests.Nodes {
		if r.AsCodeOwner {
			pr.PendingCodeOwnerRequests++
		}
	}
	for _, r := range n.LatestOpinionatedReviews.Nodes {
		switch r.State {
		case "APPROVED":
			pr.Approvals++
		case "CHANGES_REQUESTED":
			pr.ChangesRequested++
		}
	}
	return pr, nil
}

// fetchOpenPRs reads every open pull request in the repository. It returns
// either the complete list or an error, never a partial list. progress is
// called after each page.
func fetchOpenPRs(client graphQLClient, owner, name string, progress func(fetched, total int)) ([]PullRequest, error) {
	repo := owner + "/" + name
	prs := []PullRequest{}
	var after *string

	for {
		var resp openPRsResponse
		variables := map[string]interface{}{"owner": owner, "name": name, "first": openPRsPageSize, "after": after}
		err := client.Do(openPRsQuery, variables, &resp)

		var gqlErr *api.GraphQLError
		if (errors.As(err, &gqlErr) && gqlErr.Match("NOT_FOUND", "repository")) || (err == nil && resp.Repository == nil) {
			return nil, fmt.Errorf("repository %s was not found or the active gh account cannot access it; check the account with `gh auth status`", repo)
		}
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusBadGateway || httpErr.StatusCode == http.StatusGatewayTimeout) {
			return nil, fmt.Errorf("fetching open pull requests for %s: %w (GitHub may have timed out on a large page; try again)", repo, err)
		}
		if err != nil {
			return nil, fmt.Errorf("fetching open pull requests for %s: %w", repo, err)
		}

		page := resp.Repository.PullRequests
		for _, node := range page.Nodes {
			pr, err := node.toPullRequest()
			if err != nil {
				return nil, fmt.Errorf("fetching open pull requests for %s: %w", repo, err)
			}
			prs = append(prs, pr)
		}
		progress(len(prs), page.TotalCount)

		if !page.PageInfo.HasNextPage {
			return prs, nil
		}
		after = page.PageInfo.EndCursor
	}
}

// Only the result counts are read, so the 1000-result search cap does not
// apply.
const flowQuery = `
query($opened: String!, $merged: String!, $closedUnmerged: String!) {
  opened: search(query: $opened, type: ISSUE, first: 1) { issueCount }
  merged: search(query: $merged, type: ISSUE, first: 1) { issueCount }
  closedUnmerged: search(query: $closedUnmerged, type: ISSUE, first: 1) { issueCount }
}`

// fetchFlow counts the pull requests opened, merged, and closed without
// merge since the given time.
func fetchFlow(client graphQLClient, owner, name string, since time.Time) (Flow, error) {
	repo := owner + "/" + name
	ts := since.UTC().Format(time.RFC3339)
	variables := map[string]interface{}{
		"opened":         fmt.Sprintf("repo:%s is:pr created:>=%s", repo, ts),
		"merged":         fmt.Sprintf("repo:%s is:pr is:merged merged:>=%s", repo, ts),
		"closedUnmerged": fmt.Sprintf("repo:%s is:pr is:closed is:unmerged closed:>=%s", repo, ts),
	}

	var resp struct {
		Opened         struct{ IssueCount int }
		Merged         struct{ IssueCount int }
		ClosedUnmerged struct{ IssueCount int }
	}
	if err := client.Do(flowQuery, variables, &resp); err != nil {
		return Flow{}, fmt.Errorf("fetching recent pull request counts for %s: %w", repo, err)
	}
	return Flow{
		Opened:         resp.Opened.IssueCount,
		Merged:         resp.Merged.IssueCount,
		ClosedUnmerged: resp.ClosedUnmerged.IssueCount,
	}, nil
}
