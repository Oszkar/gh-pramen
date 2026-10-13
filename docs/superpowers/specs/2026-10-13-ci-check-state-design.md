# CI check state on open PRs — design

Date: 2026-10-13. Status: approved design, implementation pending.

## Goal

Show each open PR's CI state as an observed fact, and summarize it for non-draft PRs, without making the run fail more often or hiding missing data.

## Scope

- Per PR in the JSON (every PR, drafts included): the head commit's check rollup state.
- Summary block in the terminal: counts for non-draft PRs only.
- Row column in the non-draft section and in focused lists. Draft rows are unchanged.
- Out of scope: per-check names or conclusions, a `--focus` view for failing checks, how long a check has been pending, required versus optional checks.

## Definitions

Source: `statusCheckRollup.state` on the PR's head commit (`commits(last: 1)`), GitHub's combined state of all check runs and commit statuses on that commit.

| Term | Definition | Limitation |
|---|---|---|
| Check state (JSON `checkState`) | GitHub's rollup state, as reported: `SUCCESS`, `FAILURE`, `ERROR`, `PENDING`, `EXPECTED`, or `null` when no checks are reported | Head commit only. Includes optional checks. `SUCCESS` does not mean the PR can be merged |
| Success | Rollup state `SUCCESS` | As above |
| Failure or error | Rollup state `FAILURE` or `ERROR` | One failing optional check is enough |
| Pending/expected | Rollup state `PENDING` or `EXPECTED` | Says nothing about how long the state has lasted |
| None reported | No rollup on the head commit | Not "no CI": checks may not have been triggered, for example on fork PRs that wait for approval |

## Output

Terminal summary, after the review facts block:

```
Checks on head commit (non-draft)
  Success            12
  Failure or error    4
  Pending/expected    2
  None reported       3
```

The four lines are mutually exclusive and add up to the non-draft total. Counts are right-aligned like the other blocks.

Rows: a `Checks` column after `Review facts`, showing the lowercase state (`success`, `failure`, `error`, `pending`, `expected`) or `-` when none is reported.

JSON:

- `pullRequests[].checkState`: string or `null`.
- A new summary object `checkState` with `nonDraft`, `success`, `failureOrError`, `pendingOrExpected` and `noneReported`, all always present.
- `schemaVersion` stays 1, because no existing field changes meaning.

## Step 0: spike (gates implementation)

Run with `gh api graphql` before changing code:

1. Use the current open-PRs query plus `commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }`, together with `rateLimit { cost }`.
2. Try page sizes 100, 50 and 25 on ruff, kubernetes and the private work repo (with the work account's token). Record success or failure, wall time, and cost.
3. Check the permission case: with a token that cannot read checks, record whether GitHub returns an error or a `null` rollup.

Decisions from the spike:

- Page size: the largest size that passed on all three repositories, one step smaller for margin. If only sizes below 25 work, stop and revisit the design (a separate batched query by node ID).
- If missing permission produces a silent `null`, stop. Either find a field that tells "no access" apart from "no checks", or the feature does not ship. "None reported" must never stand in for unavailable data.

Record the public results in the "Spike findings" section of `docs/architecture.md`. Work-repo names and numbers go only in `NOTES.local.md`.

## Implementation

Follow the existing fetch → calculate → render boundary. No new packages or dependencies.

- `fetch.go`: add the nested field to `openPRsQuery`, and set the page size from a named constant chosen by the spike. Map the rollup to `PullRequest.CheckState` (`*string`). A failed page, including HTTP 502, still fails the run with the repository and the underlying error. A 502 message adds a hint that GitHub may have timed out on a large page.
- `report.go`: `buildReport` fills the `checkState` summary from non-draft PRs.
- `render.go`: the summary block and the `Checks` row column. It lays out only what the report contains.

## Tests (written first)

- Fixtures in `testdata/` that cover every rollup state, including `null`.
- `buildReport` table test: bucket counts per state, drafts excluded, buckets add up to `nonDraft`.
- Fetch test: the query requests the rollup, and the page size matches the constant.
- Render tests: the summary block, and the row column including `-`.
- Golden files `testdata/report.golden.json` and `testdata/report.golden.txt` updated by hand.

## Docs

- `docs/product.md`: the definitions above, the outputs table row, and a dated decision (2026-10-13).
- `docs/architecture.md`: the schema table, the data-collection description, the page size, and replacing "Avoid … check status in the bulk query" with the measured result.
- `README.md`: the new summary block and row column.
