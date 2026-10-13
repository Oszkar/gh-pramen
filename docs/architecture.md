# Architecture: `gh pramen`

How the tool described in [product.md](product.md) is built. Keep this file in step with the code; the product decisions and definitions live in the product doc.

## Shape

A gh CLI extension written in Go. It uses `go-gh` for GitHub CLI integration (authentication, host and current-repository resolution, GraphQL client) and the standard library for everything else. Avoid libraries for anticipated future features.

```mermaid
flowchart LR
    CLI[Command + repo selection] --> Fetch[GitHub fetch + pagination]
    Fetch --> Snapshot[In-memory PR snapshot]
    Snapshot --> Report[Pure report calculations]
    Report --> Table[Terminal output]
    Report --> JSON[JSON output]
```

The useful boundary is **fetch → calculate → render**. Calculations are testable without the network; renderers do not decide metric semantics. This does not require a package for every box: the code is one `main` package.

| File | Responsibility |
|---|---|
| `main.go` | Flags, repository resolution, wiring, exit codes |
| `fetch.go` | GraphQL queries, pagination, mapping API nodes to `PullRequest` |
| `report.go` | Snapshot and report types, and `buildReport`, a pure function of the fetched facts and one reference time |
| `focus.go` | Validation and pure selection of focused non-draft row views |
| `render.go` | `writeJSON` and `writeSummary`, plus the row formatting helpers; they only lay out what the report already contains |

**Current state:** `--json` writes the full report. The terminal output is the summary block (repository and collection time, open backlog, recent flow, age distribution, review facts and head-commit check state for non-draft PRs) followed by two row sections: non-draft PRs with review facts and check state, and draft PRs without. `--focus` replaces those sections with one selected non-draft list, leaving the summary intact. Each section or focused list is capped by `--limit` (default 20) or uncapped with `--all`. A Links line gives the PR URL pattern once, and PR numbers are OSC 8 hyperlinks when stdout is an interactive terminal. GitHub's review decision is deliberately not in the terminal output. The formatting helpers (humanized durations, review-facts text, author and title cleaning, rune-aligned columns) live in `render.go`. All row flags change only the display: every PR is always fetched and the JSON always has all of them.

Redirected terminal output is plain text with no color or terminal-width logic. Hyperlink escape codes are only emitted for interactive stdout with a non-dumb terminal, and only for validated HTTPS PR URLs. Counts are right-aligned within each block, and times are shown in UTC. `testdata/report.golden.txt` is a complete plain-text example.

## Data collection

Start with a fresh fetch on each run and only the fields required for the report.

- **Open PRs** come from the `repository.pullRequests(states: OPEN)` connection, `openPRsPageSize` (50) per page, following cursors until the last page. This is not a search query, so the 1000-result search cap does not apply.
- **Per PR:** number, title, URL, draft state, base branch, created and updated timestamps, review decision, author login and account type, pending review requests (with the automatic-CODEOWNERS flag), each reviewer's latest approval or change request (`latestOpinionatedReviews`), and the head commit's check rollup state (`commits(last: 1) { … statusCheckRollup { state } }`).
- **Check state** is read only from a head commit that GitHub returns. A null rollup on that commit means no checks are reported; a missing commit connection, an empty one, or a null commit fails the run with a message naming the PR. A GraphQL error anywhere in the response, including partial data with errors, fails the run, so a check field the token cannot read never turns into "none reported".
- **Review requests and latest reviews** are read up to 50 per PR. A PR with more fails the run with a message naming it, rather than reporting counts that are too low.
- **Recent flow** is one request with three search queries, reading only `issueCount`. It never reads the search results themselves, so the search cap does not apply. The window starts at an exact timestamp, 30 days before the reference time.
- **One reference time** is taken before fetching, truncated to whole seconds in UTC. Every age and the flow window are measured against it. A timestamp later than the reference time counts as zero elapsed.

Avoid commit lists, files, deployments, workflow runs, and full timelines; the only commit read is the head commit, for its check rollup state. Avoid merge-conflict status in the bulk query: requesting it together with the check rollup for 100 PRs in one page returned HTTP 502 in the first spike. The rollup state alone also returned HTTP 502 at 100 per page on kubernetes in two of three runs; 50 per page passed every run (see [check state measurements](#check-state-measurements-2026-10-13)). A 502 or 504 on a page fails the run with a hint that GitHub may have timed out.

**No SQLite, incremental sync, persistent cache, offline mode, or migrations initially.** Measure collection time before adding caching. Measured so far: about 3 s per 100 PRs without check state, and about 6 s per 100 PRs with it (50 per page).

## Errors

The fetch returns either the complete list or an error, never a partial list. Nothing is written to stdout unless the whole report succeeded.

- A failed page or a failed flow query fails the run, with the repository and the underlying error on stderr.
- A repository that cannot be resolved produces a message pointing at `gh auth status`, because the usual cause is that the active gh account is not the one with access.
- A repository with no open PRs is a success: zero counts, all four age buckets, and an empty list.

Exit codes: 0 success, 1 runtime failure, 2 usage error. Progress (one line per page) and errors go to stderr.

## JSON output

`schemaVersion` is 1. Raise it when a field changes meaning or is removed.

| Field | Content |
|---|---|
| `repository`, `collectedAt` | `owner/name` and the reference time |
| `openBacklog` | `total`, `draft`, `nonDraft` |
| `recentFlow` | `windowDays`, `since`, `opened`, `merged`, `closedUnmerged` |
| `ageDistribution` | Always four buckets, each with `label`, `minDays`, `maxDays` (inclusive; `null` for the open-ended bucket), `total`, `draft`, `nonDraft` |
| `reviewState` | Over non-draft PRs only: their number as `nonDraft`, then counts of PRs `withApprovals`, `withChangesRequested`, `withPendingRequests`, `withOnlyCodeOwnerRequests` (a subset of the previous), and `withNoneOfThese`, plus `decision` counts. A PR can be in several of the `with*` counts |
| `reviewState.decision` | `approved`, `changesRequested`, `reviewRequired`, `notReported`, and `other` for a value GitHub adds later. These sum to `nonDraft` |
| `checks` | Over non-draft PRs only: `nonDraft`, then `success`, `failureOrError`, `pendingOrExpected`, `noneReported`, and `other` for a state GitHub adds later. Each PR is in exactly one, so these sum to `nonDraft` |
| `pullRequests` | Every open PR, oldest first: the fetched facts plus `ageSeconds` and `secondsSinceUpdate`. `checkState` is GitHub's rollup state for the head commit as reported, or `null` when none is reported |

Conventions: keys are camelCase; timestamps are UTC RFC 3339; durations are whole seconds; counts are always present, including zeros. `reviewDecision: null` means GitHub reported no decision. `author: null` means GitHub no longer has the account. `checkState: null` means GitHub reported no checks on the head commit; it never stands for data that could not be read. `testdata/report.golden.json` is a complete example.

Base branch and `pendingCodeOwnerRequests` are in the JSON without a decision yet on whether or how the terminal report shows them.

## Testing

Tests are written first. Fetch and command tests run the real `go-gh` GraphQL client over a fake HTTP transport that serves fixtures from `testdata/` and records request variables, so response decoding and error handling are the real ones. `buildReport` is tested directly with a fixed reference time. Two hand-written golden files check the full JSON output and the terminal summary.

Fixtures cover pagination, empty results, an old draft, a missing review decision, an empty review decision with reviews present, a bot author, an unavailable author, a non-default base branch, truncated review data, every check state (including none reported, an unknown state and an empty one), a missing head commit, partial data with a GraphQL error, a missing repository, and a failed page with and without the timeout hint. Check representative rows against GitHub manually when the queries change.

## Why Go

- `go-gh` provides gh's auth (keyring, GHES, `GH_ENTERPRISE_TOKEN`), current-repo resolution (`GH_REPO`, git remotes), REST/GraphQL clients, and gh's own `jq` / `template` / `tableprinter`. Rust has only community crates; Rust extensions shell out to `gh auth token`, since gh passes no token to extensions (it sets only `GH_EXTENSION` and `GH_PATH`).
- `cli/gh-extension-precompile` handles cross-compiles, attestations and gh's asset naming: raw binaries ending in `<goos>-<goarch>[.exe]`, no archives, and only darwin-arm64 falls back (to amd64, if Rosetta is installed).
- The workload is network-bound, so Rust's performance advantage doesn't matter.

Releases are built by `.github/workflows/release.yml` when a `v*` tag is pushed.

## Pitfalls and design flaws to avoid

From shufo/gh-pr-stats:

- Fetching `/issues` and filtering PRs client-side wastes 2–10× requests. Query pull requests directly.
- Page count taken from a separate search call; if that call fails, the tool silently reports zero PRs. Use cursor pagination and treat errors as errors.
- Merged and closed-unmerged both counted as "closed".
- Durations rounded to whole days, so medians show 0 or 1.
- Division by zero on empty repos produces NaN, which then breaks JSON output.
- Inconsistent output schema (mixed key casing, `%` only in some rows).

General:

- Search-based queries cap at 1000 results.
- Drafts inflate waiting-time metrics unless the clock starts at ready-for-review.
- Bots skew aggregates.
- Averages get dominated by outliers; use median/p75/p90 and show `n`.
- Force-pushes rewrite commit dates; prefer `authoredDate` and fall back to PR creation.
- Squash merges break commit-based deploy mapping; map via merge commit SHA.
- Per-person rankings invite Goodhart's law; aggregate at team level and compare against the team's own baseline.

## API spike (2026-09-30)

Read-only GraphQL queries against three public repositories, fetching all open PRs through the `pullRequests(states: OPEN)` connection with cursor pagination.

| Repository | Open PRs | `reviewDecision` empty | Other values |
|---|---|---|---|
| kubernetes/kubernetes | 1,246 | 1,235 (99%) | 5 `APPROVED`, 6 `CHANGES_REQUESTED` |
| astral-sh/ruff | 477 | 445 (93%) | 1 `APPROVED`, 31 `CHANGES_REQUESTED` |
| dlvhdr/gh-dash | 15 | 0 | 15 `REVIEW_REQUIRED` |

- The spike could not read these repositories' branch rules. The pattern suggested that `reviewDecision` is populated only where branch rules require reviews; the private repository below shows that a review requirement is not sufficient either.
- An empty decision does not mean no review activity. In ruff, 17 PRs with an empty decision had an approving review and 152 had pending review requests. In kubernetes, 1,226 PRs with an empty decision had pending review requests.
- `reviewRequests { totalCount }` and `latestOpinionatedReviews { nodes { state } }` were available on every PR and need no timeline query.
- Adding these nested fields raised the cost from 1 to 2 rate-limit points per 100-PR page and roughly doubled fetch time (ruff 15 s → 27 s, kubernetes 24 s → 71 s). The slower query also fetched each PR's last commit, so the review fields alone cost less than that. The built tool, without the last commit, fetched ruff in 14 s.
- Search `issueCount` returned opened, merged, and closed-unmerged totals for a date window in one request of about a second, at a cost of 1 point. Totals above 1,000 came back intact (kubernetes, since 2026-03-01: 3,940 opened, 2,269 merged, 1,014 closed-unmerged), so the 1000-result search cap does not affect counts.

**Private repository.** The same queries were run against one private repository, read with admin permission. A repository ruleset requires one approving review on the default branch; there are no classic branch protection rules.

- `reviewDecision` was empty for most PRs to the default branch even though an approval is required there, and reported `REVIEW_REQUIRED` on some PRs to branches that no rule covers. Draft state, age, merge conflicts, author association, review content, and pending requests did not separate empty from `REVIEW_REQUIRED`; the cause is unknown.
- Where a decision was reported, it agreed with the human reviews.
- Most PRs to the default branch had a pending review request that GitHub added from CODEOWNERS (`asCodeOwner`), so a pending request does not mean someone asked for a review.
- A review bot had left a comment-only review on nearly every PR, so "has a review" is not a usable fact; approvals and change requests are.
- Adding `mergeable` and `statusCheckRollup` for 100 PRs in one page returned HTTP 502; `mergeable` alone worked in pages of 25.
- A burst of merges just inside or outside the window edge changed the 30-day merged count by more than half, so the flow counts need the exact window start shown next to them.

Repeat the spike on each further dogfood repository to validate field availability and permissions there.

## Check state measurements (2026-10-13)

Full paginated runs of the open-PR query with `gh api graphql --paginate`, three times per variant. A run passes when `gh` exits 0 and no page has `errors`. The token was gh's default OAuth login (`repo` scope). Times are medians of the passing runs; cost is the summed `rateLimit.cost` of one run.

| Repository (open PRs) | Without check state, 100/page | With check state, 100/page | 50/page | 25/page |
|---|---|---|---|---|
| astral-sh/ruff (519) | 3/3, 13.4 s, cost 12 | 3/3, 25.7 s, cost 18 | 3/3, 30.0 s, cost 22 | 3/3, 36.3 s, cost 21 |
| kubernetes/kubernetes (1,354) | 3/3, 35.4 s, cost 28 | **1/3** (two HTTP 502), 86.5 s, cost 42 | 3/3, 83.6 s, cost 56 | 3/3, 104.5 s, cost 55 |
| One private repository (under 100) | 3/3, under 3 s | 3/3, under 6 s | 3/3, under 6 s | 3/3, under 6 s |

- `openPRsPageSize` is 50: the largest size that passed every run on every repository. Smaller pages were never faster.
- Check state roughly doubles collection time and cost, at any page size.
- The commit from `commits(last: 1)` matched `headRefOid` on every PR in every run (1,931 PRs). No PR came back without a head commit.
- Rollup states with no check state at all: ruff 97 of 519. Through REST, eight sampled PRs among them had zero check runs and zero commit statuses, and a `SUCCESS` control had 50 check runs.
- kubernetes reported no `SUCCESS`: 909 `PENDING` and 445 `FAILURE`. The pending state on sampled PRs came from Prow's `tide` merge-pool status, which stays pending until merge.
- Not measured: tokens with narrower permissions (fine-grained PATs, GitHub App or Actions tokens). Whether they return an error, a null rollup, or a partial rollup is unknown.
