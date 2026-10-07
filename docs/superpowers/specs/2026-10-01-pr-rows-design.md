# Per-pull-request rows in the terminal report

Status: approved design, 2026-10-01. Closes the open decision in `docs/product.md` section 6 ("A readable terminal layout for the per-PR rows").

## Goal

After the summary block, list a sample of the open PRs in the terminal so a lead has concrete PRs to inspect. Today the report ends with "Per-pull-request rows are not in this report yet; use --json for them."

This is a sample, not a trace. By default each section shows the 20 oldest PRs, so rows will not explain every summary count (newer PRs behind "under 7 days" or a review-facts count may not appear), and there is no way to select a summary category. The JSON stays the complete trace. Oldest first is one useful view, not the best view for every conversation. If dogfooding shows leads asking for a slice (for example approved PRs, or recent PRs), a filter on top of the same renderer is the follow-up, and it is out of scope here.

## Layout

Two sections follow the summary: non-draft PRs first, then draft PRs. Each is ordered oldest first by creation date, ties broken by PR number (the order `buildReport` already produces).

```
Open pull requests, non-draft (showing 20 of 93, oldest first by creation date)
Links: https://github.com/owner/repo/pull/<number>

      #  Age   Updated  Review facts                         Author         Title
   1311  2y    2y       approvals 1                          bob            Add retry to uploader
   1590  1y    3d       pending requests 2 (CODEOWNERS only) carol          Bump lint config

Open pull requests, draft (showing 20 of 219, oldest first by creation date)

      #  Age   Updated  Author         Title
   1204  2y    11mo     alice          Refactor billing module
```

An empty section prints only its header: `Open pull requests, draft (none)`.

## Rules

- **Sections:** non-draft first, because review facts apply to it. The draft section has no review-facts column, matching how the summary excludes drafts from review facts. An empty section prints `Open pull requests, <kind> (none)` and no table, so it never looks like a failed fetch.
- **Cap:** `--limit N` rows per section, default 20. `--all` removes the cap. Every section header says "showing N of total" (or "N" when nothing is cut), so truncation is never silent.
- **Flag validation** happens before any fetch, whether or not `--json` is set: `--limit` below 1 is a usage error (exit 2), and an explicit `--limit` together with `--all` is a usage error (exit 2; explicit is detected with `flag.Visit`, so the default does not count). With valid flags, `--json` ignores both and always contains every PR. Help text says both flags only change what the terminal shows; the fetch always reads every PR, so they do not speed up collection.
- **Review facts cell:** words only, in this order: `approvals N`, `changes requested N`, `pending requests N`, joined with ", ". The cell counts reviewers' latest approving reviews, not an approval of the PR for merge. When every pending request came from CODEOWNERS, `pending requests N` is followed by `(CODEOWNERS only)`. `-` means none of these were observed. Never "unreviewed", "approved" or "ready to merge". A cell can hold approvals and changes requested together; that is accurate and shown as is.
- **Links:** when at least one row is shown, the URL pattern is printed once, under the first section header that has rows, derived from the first listed PR's URL by replacing its number with `<number>`. With no rows there is no line. Full URLs stay in the JSON. Whether the template is too much friction is a dogfood question (how often does a lead open a row?), and a URL column or terminal hyperlinks are the options if it is.
- **Times:** coarse humanized values measured against the single reference time, always rounded down: under 1 hour `<1h`; 1-23 hours `Nh`; 1-89 days `Nd`; 90-364 days `Nmo` (N = whole days / 30); 365 days or more `Ny` (N = whole days / 365). Days run to 89 so the 7, 30 and 90 day bucket edges stay visible. Months are fixed 30-day months, not calendar months. A timestamp after the reference time counts as zero. The JSON keeps exact seconds. Updated is a duration since the last update, not a date, for consistency with Age.
- **Author:** the login, with a `[bot]` suffix when the account type is `Bot`. A missing account (`author: null`) shows `ghost`. Truncated to 24 runes with `…`.
- **Titles:** control characters (including newlines and escape sequences) are replaced with a space, then the title is truncated to 60 runes (not bytes) with `…`. Titles are untrusted text.
- **Width:** columns are aligned by rune count, which is right for ASCII and most Latin text but can misalign wide characters (CJK, emoji). Title is the last column so wrapping hits it first. A row can exceed 120 columns; there is no terminal-width logic in this first version. Check real output with long titles and logins, and mixed Unicode, while dogfooding.
- **Plain text:** aligned columns, no color; reads the same when redirected.
- **Wording:** headers state the sort ("oldest first by creation date") and nothing that implies priority or judgement.

## Architecture

- `report.go`: unchanged. Humanized durations and the review-facts text are presentation rules, so they live in `render.go` (pure functions there, tested directly), keeping fetch -> calculate -> render intact. No new report fields.
- `render.go`: duration, review-facts, author and title formatting, and `writeSummary` taking the row limit to lay out the two sections.
- `main.go`: `--limit` and `--all` flags, validated in `run` before any fetch (usage errors exit 2), passed to the summary writer. Usage text updated.
- JSON: unchanged, so `schemaVersion` stays 1.
- The "not in this report yet" line is removed.

## Testing (written first)

- Unit tests: duration formatting at each boundary (59m, 1h, 23h, 24h, 89d, 90d, 364d, 365d, and a future timestamp), review-facts text for each combination, author rendering (user, bot, missing, long), title sanitizing and truncation by runes, cap header wording, empty sections (`(none)`).
- `testdata/report.golden.txt` extended by hand with both sections (not generated).
- Command tests: default limit, `--limit`, `--all`, `--limit 0` and explicit `--limit` with `--all` rejected with exit 2 including together with `--json` (before any request is made), `--json` with valid flags unaffected by them.

## Docs (same change)

- `docs/product.md`: close the open decision in section 6; add definitions for the row columns (humanized time rule, `approvals` wording, sample-not-trace limitation) to the definitions table.
- `docs/architecture.md`: update the "Current state" paragraph and the file responsibilities.
- `README.md`: the status line (terminal rows are only in the JSON), the example output, and the `--json` usage comment.

## Out of scope

Filters, per-section sorts, clickable terminal links, color, width-aware wrapping, showing base branch.
