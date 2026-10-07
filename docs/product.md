# Product: `gh pramen` — understand a repository's open PR backlog

> *“Untangle your pull requests.”* This is an early direction document, not a finalized spec. How it is built is in [architecture.md](architecture.md); background research is in [research.md](research.md).

## 1. First user and question

The first user is an engineering leader who has just joined an organization, opens GitHub, and finds hundreds of open PRs across several repositories. The immediate opportunity is to dogfood a small tool with the repository leads to understand that backlog and decide where attention is useful.

The first question is:

> **What is sitting open in this repository, and what should we discuss with its lead?**

A large backlog alone does not establish a problem. It may contain active work, long-lived drafts, dependency updates, experiments, or abandoned work. The tool should make that mix understandable and give the lead concrete PRs to inspect.

Start with **one repository per invocation**. Running the same command separately for several repos is sufficient for initial dogfooding.

## 2. Principles

A **gh CLI extension, written in Go**, producing a readable, factual snapshot of a repository's open PR backlog using existing GitHub authentication.

1. **KISS:** one repo, one report, no server, database, setup wizard, or configuration file.
2. **Facts before interpretations:** show age and observed state; do not infer that old PRs are abandoned or approved PRs are ready to merge.
3. **Traceable summaries:** counts lead to PR numbers, titles, and URLs. No TUI is needed.
4. **No individual performance metrics:** authors identify work in PR rows; no author rankings, reviewer leaderboards, or productivity scores.
5. **Visible limitations:** missing permissions, failed pages, and unavailable fields must not look like zero activity.
6. **Simple definitions:** use calendar elapsed time and describe exactly what each age or status means.
7. **Dogfooding drives scope:** deferred ideas are possibilities, not a committed release roadmap.

## 3. POC / MVP

### One command

```sh
gh pramen                    # current repository
gh pramen -R owner/repo       # one explicit repository
gh pramen -R owner/repo --json
gh pramen -R owner/repo --limit 50   # rows per section (default 20)
gh pramen -R owner/repo --all        # every row
```

Produce a terminal report by default and structured JSON for inspecting or reusing results. Plain output remains readable when redirected. Progress/errors go to stderr.

No separate `sync`, `cycle-time`, `reviews`, `size`, or `dora` commands initially.

### One report

| Part | Purpose |
|---|---|
| Repository and collection time | Establish scope and freshness |
| Open backlog | Total PRs, split into draft and non-draft |
| Recent flow | PRs opened, merged, and closed without merge in the last 30 days; context for the backlog size |
| Age distribution | Age buckets split by draft/non-draft |
| Review state | Counts and PR lists of non-draft PRs by observed review facts (pending review requests, approvals, changes requested), with pending requests that come only from CODEOWNERS shown separately. GitHub's review decision is kept per PR in the JSON, not summarized |
| PR details | The terminal lists the oldest non-draft and draft PRs in separate sections, capped per section, with age, time since update, review facts (non-draft), author and title; the JSON has every PR with full detail |

Start with age buckets of **under 7 days, 7–29 days, 30–89 days, and 90+ days**. These help navigate the backlog; they are not performance targets.

Recent flow is three counts over a fixed 30-day window ending at the reference time. It separates a busy repository from a stuck one: 300 open PRs with 400 merged a month is a different conversation from 300 open with 10 merged. Always split **merged** from **closed-unmerged**; the original tool counted both as "closed". Counts only: no durations, trends, or per-PR lists for closed work.

Review state cannot rest on GitHub's review decision alone. In the [API spike](architecture.md#api-spike-2026-09-30) it was empty for 93–99% of open PRs in two public repositories, including PRs that had approvals or pending review requests, and for most PRs to the default branch of a private repository that requires an approval. Report what is observable on every PR (pending requests and each reviewer's latest approval or change request) and keep the decision as a per-PR fact in the JSON where GitHub reports one. Summarizing it invites the wrong reading, so the summary leaves it out.

Review facts are counted over non-draft PRs. Drafts dominate the "no review facts" count otherwise (in one public repository, 219 of 283), and a draft without reviews is the expected state, not a discussion point. Pending requests are split out when all of them were added from CODEOWNERS, since those say something about repository configuration rather than about anyone asking for a review.

Show full counts even if the terminal displays a bounded number of detail rows. Make truncation explicit and include all rows in JSON.

Keep bot-authored PRs visible and identify them where account metadata supports it. Dependency updates may explain much of the backlog. Avoid elaborate bot detection and allow/deny configuration.

Precise time waiting for review is deferred: it requires history and decisions about drafts, repeated requests, and what counts as a review.

### Definitions

| Label | Meaning | Limitation |
|---|---|---|
| Open PR age | Reference time minus creation time | Includes draft time; not active work time |
| Time since update | Reference time minus GitHub's PR update timestamp | Updates need not represent meaningful progress or human activity |
| Draft | Current draft state | Does not reconstruct previous draft periods |
| Pending review requests | Number of reviewers or teams currently requested | Includes requests GitHub adds automatically from CODEOWNERS; does not show how long a request has been open |
| Only from CODEOWNERS | All of a PR's pending requests were created for code owners | A code owner may also have been asked in person; GitHub records only how the request was created |
| Latest reviews | Approvals and change requests, counting each reviewer's latest such review | Comment-only reviews, including those left by review bots, are not counted. A review may predate later commits; does not establish merge readiness |
| Review decision | GitHub's current reported review decision | Often empty, even where branch rules require reviews. Does not establish wait duration or merge readiness |
| Author | Account that opened the PR | Not necessarily the current owner or all contributors |
| Row ages (Age, Updated) | Open PR age and time since update, shown rounded down: `<1h`; `Nh` for 1-23 hours; `Nd` for 1-89 days; `Nmo` for 90-364 days (whole days / 30, fixed 30-day months); `Ny` for 365 days or more (whole days / 365) | Coarser than the JSON's exact seconds. Days run to 89 so the 7, 30 and 90 day bucket edges stay visible. A timestamp after the reference time counts as zero |
| Review facts (rows) | `approvals N`, `changes requested N`, `pending requests N`, as defined above, with `(CODEOWNERS only)` when every pending request came from CODEOWNERS; `-` means none of these were observed | Approvals count reviewers' latest approving reviews; they do not establish that the PR is approved for merge. `-` is not "unreviewed": it also appears on PRs whose only reviews are comment-only, including review-bot comments |
| Listed rows | The oldest N open PRs of each section (non-draft, draft) by creation date, 20 by default (`--limit N`, `--all`) | A sample, not a trace: newer PRs behind a summary count may not be listed, and there is no way to select a summary category. The JSON has every PR |
| Recently opened | PRs created in the 30 days before the reference time | Includes PRs already merged or closed |
| Recently merged | PRs merged in the same window | Says nothing about how long they were open. A burst of merges near the window edge can move the count sharply from one day to the next |
| Recently closed-unmerged | PRs closed without merge in the same window | Does not distinguish abandoned, superseded, or rejected work |

Do not label an absent review decision “unreviewed.” An empty decision says nothing about whether the PR was reviewed or whether the repository requires reviews. Distinguish “no decision reported” from unavailable data. Approval does not prove that checks, conflicts, policies, or other prerequisites permit merging.

### Essential correctness

- Fetch **all currently open PRs**, even those created years ago. No historical cutoff.
- Paginate completely; do not rely on an arbitrary first page or search result limit.
- Use one reference time for age calculations. The paginated read is a best-effort snapshot, not an atomic view.
- Distinguish a successful empty result from a failed fetch.
- On a failed page, fail clearly rather than presenting a successful-looking partial report. Partial-report support is not required.
- Missing optional metadata can remain unknown if that limitation is visible.
- JSON contains raw durations in seconds; the terminal humanizes them.
- The tool is read-only. Decisions to close, comment on, or assign PRs remain with the engineering leader and the leads.

## 4. Dogfooding with the leads

1. Start with one repository with a substantial open backlog.
2. Inspect a sample of report rows with its lead, including old drafts and approved-but-open work.
3. Discuss which PRs are active, intentionally parked, waiting on someone, or candidates to close. The tool supplies evidence; the lead supplies context.
4. Record misleading states and missing facts that would materially change the conversation.
5. Repeat independently on another repository.

Success means **a useful, accurate backlog conversation with little manual preparation**. It does not require a lower open-PR count or agreement with an industry benchmark.

Before expanding scope, check whether the report revealed something worth acting on, whether its facts were trustworthy, whether it reduced manual browsing, and whether it was fast enough to repeat. Prefer improvements that solve repeated problems over adding every requested metric.

## 5. Deferred until justified

Each row keeps what the earlier research learned, so picking one up does not start from zero. None of this is committed. See [research.md](research.md) for terminology and [architecture.md](architecture.md#pitfalls-and-design-flaws-to-avoid) for pitfalls.

| Possibility | Evidence that would justify it | Prior research / notes |
|---|---|---|
| Flow trends beyond the 30-day counts | Leads need to see whether the backlog is growing over several periods | The MVP has one 30-day window (section 3). Industry name for merged-per-period: *merge frequency* / *PR throughput*. |
| PR-opened-to-merge durations and comparisons | Leads need completed-flow trends; define the cohort and show open-work age alongside them | Report median/p75/p90 with `n`, not averages. `gh pr list --json createdAt,mergedAt` already gets a basic median in one jq line, so the value is in cohorts and comparisons, not the raw number. |
| Review waiting time and timeline explanations | Current review states repeatedly fail to answer the next useful question | Needs GraphQL `timelineItems` (`READY_FOR_REVIEW_EVENT`, `REVIEW_REQUESTED_EVENT`, reviews); `gh pr --json` has no timeline field. Standard terms: *pickup time* (ready for review → first non-author review) and *time to first review* (review requested → first review). Start the clock at ready-for-review so drafts don't inflate it. Nested timeline queries hit GraphQL's 10s timeout: fetch 25–50 PRs per page. |
| Markdown output | Sharing outside the terminal becomes routine | Useful for retros and written updates. Rendering only; no new data. |
| Labels/base-branch filters | Mixed workflows obstruct discussion | Cheap: fields are already in the PR payload. |
| Bot filtering | Dependency PRs dominate and obstruct discussion | Keep visible by default, per section 3. If needed, `author.is_bot` / `[bot]` suffix covers most cases; Renovate and Dependabot are the usual volume. |
| Multi-repo collection | Separate invocations become a significant burden | Bounded concurrency across repos. GraphQL budget is 5,000 points/hour per user (roughly 1 point per 100-node page). |
| Team/CODEOWNERS mapping | Repo-level scope does not support the necessary conversations | GitHub team membership via REST, or CODEOWNERS parsing. Decide whether current membership is acceptable for historical windows. |
| Local cache | Measured collection latency obstructs repeated use | Pure-Go SQLite (`modernc.org/sqlite`) keeps `CGO_ENABLED=0` cross-compiles simple. Incremental sync by `updatedAt`. |

**Outside initial scope:** DORA, deployments and deployment proxies, coding-time estimates, cycle-time stage breakdowns, review depth/load analytics, size correlations, business-hours clocks, TUI, scheduled digests, dashboards, and org-wide discovery. Definitions and data sources for these are kept in [research.md](research.md) for reference only.

Do not pre-build abstractions for these. DORA and deployment work have no committed next-version slot.

**Not a product goal:** individual performance scoring or leaderboards.

## 6. Open decisions

None at the moment.

**Decided (2026-09-30):** recent flow counts are part of the MVP report, alongside the backlog snapshot (section 3).

**Decided (2026-10-01):** review facts cover non-draft PRs only; pending requests that come only from CODEOWNERS are shown separately; GitHub's review decision stays out of the summary.

**Decided (2026-10-06):** the terminal lists the oldest open PRs in two sections, non-draft and draft, 20 rows each by default (`--limit N`, `--all`). It is a sample, not a trace of every summary count. Questions left for dogfooding: whether the one-line links template is too much friction, whether wide rows wrap badly on real titles, and whether leads want a filtered slice (for example approved PRs) instead of oldest first.
