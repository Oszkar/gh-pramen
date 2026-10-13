# CI check state on open PRs: design

Date: 2026-10-13. Status: approved design, revised after review, implemented on `feat/ci-check-state`. Spike result: 50 per page (100 per page returned HTTP 502 on kubernetes in two of three runs).

## Goal

Show each open PR's head-commit check state as an observed fact, and summarize it for non-draft PRs. Never let unreadable or missing check data look like "no checks". A run that cannot read the check state fails clearly instead.

This adds a requirement. A credential that can read pull requests but not checks used to succeed and now fails with GitHub's error. That is deliberate. The alternative would be degrading silently to "none reported", and the guardrails forbid that.

## Scope

- In the JSON, for every PR including drafts: the head commit's check rollup state.
- In the terminal summary: counts for non-draft PRs only.
- A row column in the non-draft section and in focused lists. Draft rows are unchanged.
- Out of scope: per-check names or conclusions, a `--focus` view for failing checks (added in a follow-up PR as `failing-checks`), how long a check has been pending, required versus optional checks, and a flag that skips check state.

## Definitions

Source: `commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }` on each PR. This is the same path `gh pr view --json statusCheckRollup` uses. In the spike, the returned commit matched `headRefOid` on every PR.

| Term | Definition | Limitation |
|---|---|---|
| Check state (JSON `checkState`) | GitHub's rollup state as reported (`SUCCESS`, `FAILURE`, `ERROR`, `PENDING`, `EXPECTED`), or `null` when no rollup is reported on a head commit GitHub returned | Head commit only: checks reported against another commit, such as a test merge commit, are not included. Includes optional checks. `SUCCESS` does not mean the PR can be merged |
| Success | `SUCCESS` | As above |
| Failure or error | `FAILURE` or `ERROR` | One failing optional check is enough |
| Pending/expected | `PENDING` or `EXPECTED` | Says nothing about how long the state has lasted. A merge-bot status (kubernetes' `tide`) can hold a PR at pending until it merges |
| None reported | Explicit `null` rollup on a head commit GitHub returned | Not "no CI": checks may not have been triggered, for example on fork PRs that wait for approval |
| Other state | Any other value, including an empty string | Shown in the terminal only when present |

### Response shapes (review finding 5)

| Shape | Behavior |
|---|---|
| `commits` with one node, non-null `commit`, non-null rollup | `checkState` = the state string, as reported |
| Same, with `statusCheckRollup: null` | `checkState: null` (none reported) |
| `commits` null, `nodes` empty, or `commit` null | The run fails: "could not read the head commit of pull request #N" |
| Any GraphQL `errors`, including partial data | The run fails with GitHub's message (`go-gh` returns a `GraphQLError` whenever `errors` is non-empty) |

### Unexpected values (review finding 6)

This follows the existing `DecisionCounts.Other` precedent. An unknown or empty state goes into `other`, so the buckets still add up to `nonDraft`. JSON always has `other`. The terminal shows an "Other state" line only when it is non-zero. Rows show the state in lower case, or `?` when it is empty, with control characters stripped.

## Output

The terminal summary gets a new block after the review facts block:

```
Checks on the head commit of the 4 non-draft pull requests
  Success           1
  Failure or error  1
  Pending/expected  1
  None reported     1
```

Rows get a `Checks` column after `Review facts`.

JSON:

- `pullRequests[].checkState`: a string or `null`.
- A summary object `checks` with `nonDraft`, `success`, `failureOrError`, `pendingOrExpected`, `noneReported` and `other`, always present.
- `schemaVersion` stays 1, because the fields are additive.

## Spike (review findings 1, 3, 4, 8)

Method:
- Each run fetches every page of open PRs with `gh api graphql --paginate`, with `rateLimit { cost }` added.
- A run passes only if `gh` exits 0 and no page has `errors`.
- The variants are the current query at 100 per page (baseline), and the query with the rollup at 100, 50 and 25 per page.
- Every variant runs three times on astral-sh/ruff, kubernetes/kubernetes and the private work repo.

Page-size rule: choose the largest size that passed every run on every repository. Repeated full runs are the margin, so the old "one step smaller" rule is dropped. If no size passes, stop and revisit the design (a separate query batched by node ID). Results and the chosen size are in `docs/architecture.md` under "Check state measurements".

Head commit (finding 8): for every PR in every passing run with the rollup, compare `commits(last: 1)` with `headRefOid`.

Permissions (finding 1):
- **Verified:** with gh's default OAuth login (`repo` scope, both accounts on this machine), a `null` rollup matched zero check runs and zero commit statuses through REST on sampled PRs, and a `SUCCESS` control PR had check runs.
- **Not verified:** fine-grained PATs, GitHub App and Actions tokens with narrower permissions. Neither available token can create that condition, so whether they error, return null, or return a partial rollup is unknown.
- This limitation is documented in `docs/product.md`. Before relying on such tokens, run the same commit through a full-access token and a restricted token, and compare.
- A partial rollup (statuses visible, check runs hidden) cannot be detected from the rollup alone. This is documented, not solved.

## Implementation

- `fetch.go`: `openPRsPageSize` is passed as `$first`. The rollup is added to the query and mapped per the response-shape table. HTTP 502 and 504 add a "GitHub may have timed out" hint, and other errors do not.
- `report.go`: `PullRequest.CheckState`, plus `CheckCounts` filled from non-draft PRs.
- `render.go`: the summary block, the `Checks` column, and `checkLabel`.

## Tests (review finding 7)

- Fetch tests:
  - The query requests the rollup, and `first` equals `openPRsPageSize`.
  - Every state maps to `CheckState`, including on a draft and an explicit null.
  - An unknown state is kept as reported.
  - A missing head commit fails in three shapes: `commits` null, `nodes` empty, `commit` null.
  - Partial data with a FORBIDDEN error on the rollup path fails without returning PRs.
  - A 502 carries the hint and keeps the repository and status; a 401 has no hint.
- `buildReport`: every bucket, drafts excluded, unknown and empty states in `other`, buckets add up to `nonDraft`.
- Render: the summary block, the Other line only when non-zero, `checkLabel`, the column on non-draft rows only, and a focused view that keeps the block and the column.
- Golden JSON and text are updated by hand. Zero-valued `checks.other` is present in the golden JSON.

## Docs (review finding 9)

- `docs/product.md`: definitions, the one-report table, and a decision dated 2026-10-13.
- `docs/architecture.md`:
  - The schema table, data collection, and response-shape behavior.
  - The "avoid commits / check status" guidance is replaced with the measured result.
  - Measurements and the fixture list.
- `fetch.go`: the query comment now leaves out only merge-conflict status.
- `README.md`: the example output and the parts table.
