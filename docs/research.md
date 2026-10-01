# Research background

Reference material from the first round of research (2026-09-30). Not a roadmap: it supports the deferred possibilities in [product.md](product.md#5-deferred-until-justified), so picking one up does not start from zero.

## Landscape: what already exists

| Already covered elsewhere | Where |
|---|---|
| Raw PR fields + jq (`mergedAt`, `reviews`, `reviewDecision`, `isDraft`, `additions`, …) | `gh pr list --json … --jq` |
| PR inbox / dashboard | `gh-dash` |
| One metrics row per PR | `hectcastro/gh-metrics` |
| DORA and cycle-time stages | Heavy self-hosted platforms (Apache DevLake, Middleware); Google Four Keys is archived |
| Review-stage medians (ready → first review → final review → merge) | GitHub Copilot usage metrics API (2026). Org/enterprise level, Copilot-gated, API-only, not team-scoped |
| Vendor dashboards | LinearB, Swarmia, Jellyfish, DX, Sleuth, Graphite Insights |

What `gh` can't do on its own: PR timeline events, aggregation/percentiles, anything past the 1000-result cap of search-based queries (`gh search prs`, `gh pr list --search`), caching.

## Terminology

Use these names if and when the related features are built.

**DORA (dora.dev, 2024+ naming)**

| Metric | Group | Definition | GitHub data | Needs convention |
|---|---|---|---|---|
| Deployment frequency | Throughput | Successful production deployments per period | Deployments API, releases, or workflow runs; fallback merges to default branch, labelled *(merge proxy)* | What counts as a deploy |
| Change lead time | Throughput | First commit → running in production | PR commits `authoredDate` → deployment SHA mapped to merged PRs | Deploy source |
| Failed deployment recovery time | Throughput | Failed deployment → recovery | Deployment statuses, revert PRs | Failure definition |
| Change fail rate | Instability | Share of deployments needing immediate intervention | Failed deployment statuses, reverts, hotfix labels | Failure definition |
| Deployment rework rate | Instability | Share of deployments that are unplanned, caused by a production incident | `hotfix`/`incident` labels, branch patterns | Labelling convention |

Renames: MTTR became *failed deployment recovery time* (2023); *rework rate* added and "lead time for changes" became *change lead time* (2024). DORA dropped performance tiers in 2025. DORA advises measuring per application/service and not comparing teams.

**PR cycle time (LinearB-style breakdown, the most common vocabulary)**

| Stage | Start | End |
|---|---|---|
| Coding time | First commit | PR opened |
| Pickup time | Ready for review (or created, if never a draft) | First non-author, non-bot review activity |
| Review time | First review activity | Merge |
| Deploy time | Merge | Production deploy |

Vendors disagree: Swarmia ends review at final approval and starts at review request; Sleuth calls pickup "review lag"; GitLab's DORA lead time starts at merge. Always state start and end events.

**Other PR terms:** *PR size* (additions + deletions, excluding lockfiles/generated), *merge frequency*, *merged without review*, *review depth* (comments per reviewed PR), *abandoned* (closed without merge).

**Disambiguation:** "rework" means DORA deployment rework rate; LinearB's "rework ratio" is *code churn*. Never say "lead time" alone.

**Frameworks leads cite:** SPACE (only its Activity and some collaboration/flow parts are computable from GitHub) and DX Core 4 (Speed = PRs per engineer, which DX says never to use at individual level).

## References

**Original and related tools**
- shufo/gh-pr-stats (original inspiration) — https://github.com/shufo/gh-pr-stats
- dlvhdr/gh-dash — https://github.com/dlvhdr/gh-dash
- hectcastro/gh-metrics — https://github.com/hectcastro/gh-metrics
- rvalessandro/gh-pr-metrics — https://github.com/rvalessandro/gh-pr-metrics
- github-community-projects/issue-metrics — https://github.com/github-community-projects/issue-metrics
- apache/devlake — https://github.com/apache/devlake
- middlewarehq/middleware — https://github.com/middlewarehq/middleware
- dora-team/fourkeys (archived) — https://github.com/dora-team/fourkeys
- ParthibanRajasekaran/delivery-intel — https://github.com/ParthibanRajasekaran/delivery-intel

**DORA and frameworks**
- DORA metrics guide — https://dora.dev/guides/dora-metrics/
- DORA metrics history (renames) — https://dora.dev/insights/dora-metrics-history/
- DORA 2025 report commentary — https://redmonk.com/rstephens/2025/12/18/dora2025/
- 2024 performance clusters — https://octopus.com/blog/2024-devops-performance-clusters
- DX Core 4 — https://getdx.com/dx-core-4/ · https://docs.getdx.com/dx-core-4/
- SPACE framework — Forsgren et al., ACM Queue, 2021

**Vendor definitions**
- LinearB cycle time — https://linearb.helpdocs.io/article/0vif1ihmgc-how-is-cycle-time-calculated
- LinearB glossary — https://linearb.helpdocs.io/article/1s38stsugw-glossary-metrics
- Swarmia PR cycle time — https://help.swarmia.com/metrics-and-definitions/pull-request-cycle-time
- Swarmia change lead time — https://help.swarmia.com/definitions/dora-metrics/change-lead-time
- Swarmia draft time (2026-09) — https://www.swarmia.com/changelog/2026-09-11-pull-request-draft-time/
- Sleuth change lead time — https://help.sleuth.io/sleuth-dora/accelerate-metrics/change-lead-time.md
- Middleware calculations — https://middlewarehq.com/docs/product/oss/calculations
- Graphite Insights definitions — https://graphite.dev/docs/insights-stats-definitions
- GitLab Value Stream Analytics — https://docs.gitlab.co.jp/ee/user/analytics/value_stream_analytics.html

**GitHub platform**
- go-gh — https://github.com/cli/go-gh
- gh-extension-precompile — https://github.com/cli/gh-extension-precompile
- gh extension binary selection — `pkg/cmd/extension/manager.go` in https://github.com/cli/cli
- GraphQL rate & query limits — https://docs.github.com/en/graphql/overview/rate-limits-and-query-limits-for-the-graphql-api
- REST rate limits — https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
- Deployments REST API — https://docs.github.com/en/rest/deployments/deployments
- Copilot usage metrics: PR throughput & time to merge (2026-02) — https://github.blog/changelog/2026-02-19-pull-request-throughput-and-time-to-merge-available-in-copilot-usage-metrics-api/
- Copilot usage metrics: PR review stages (2026-09) — https://github.blog/changelog/2026-09-25-usage-metrics-api-adds-pull-request-review-stages/
