# Per-pull-request rows in the terminal report

Status: approved design, 2026-10-01. Closes the open decision in `docs/product.md` section 6 ("A readable terminal layout for the per-PR rows").

## Goal

Make the summary's counts traceable in the terminal: after the summary block, list the open PRs so a lead can see which PRs sit behind the numbers. Today the report ends with "Per-pull-request rows are not in this report yet; use --json for them."

## Layout

Two sections follow the summary: non-draft PRs first, then draft PRs. Each is ordered oldest first by creation date, ties broken by PR number (the order `buildReport` already produces).

```
Open pull requests, non-draft (showing 20 of 93, oldest first by creation date)
Links: https://github.com/owner/repo/pull/<number>

      #  Age     Updated   Review facts                  Author         Title
   1311  2y 1mo  2y        approved 1                    bob            Add retry to uploader
   1590  1y 8mo  3d        pending 2 (CODEOWNERS only)   carol          Bump lint config

Open pull requests, draft (showing 20 of 219, oldest first by creation date)

      #  Age     Updated   Author         Title
   1204  2y 3mo  14mo      alice          Refactor billing module
```

## Rules

- **Sections:** non-draft first, because review facts apply to it. The draft section has no review-facts column, matching how the summary excludes drafts from review facts. An empty section prints its header with "none" instead of rows, so it never looks like a failed fetch.
- **Cap:** `--limit N` rows per section, default 20, N >= 1. `--all` removes the cap. Passing both is a usage error (exit 2). Every section header says "showing N of total" (or "N" when nothing is cut), so truncation is never silent. `--json` ignores both flags and always contains every PR.
- **Review facts cell:** words only, in this order: `approved N`, `changes requested N`, `pending N`, joined with ", ". When every pending request came from CODEOWNERS, `pending N` is followed by `(CODEOWNERS only)`. `-` means none of these were observed. Never "unreviewed" or "ready to merge".
- **Links:** the URL pattern is printed once, under the first section header, derived from the PR URLs (host and repository as GitHub reported them). Full URLs stay in the JSON.
- **Times:** coarse humanized values measured against the single reference time: `<1h`, hours (`5h`), days (`3d`), months (`14mo`), years and months (`2y 1mo`). A timestamp after the reference time counts as zero. The JSON keeps exact seconds. Updated is a duration since the last update, not a date, for consistency with Age.
- **Author:** the login, with a `[bot]` suffix when the account type is `Bot`. A missing account (`author: null`) shows `ghost`.
- **Titles:** control characters (including newlines and escape sequences) are replaced with a space, then the title is truncated to 60 characters with `…`. Titles are untrusted text.
- **Plain text:** aligned columns, no color, no terminal-width logic; reads the same when redirected.
- **Wording:** headers state the sort ("oldest first by creation date") and nothing that implies priority or judgement.

## Architecture

- `report.go`: pure helpers for human durations and the review-facts text, functions of `Row` facts. No new fields in the report.
- `render.go`: `writeSummary` takes the row limit and lays out the two sections. It decides nothing about metrics.
- `main.go`: `--limit` and `--all` flags, validated in `run` (usage errors exit 2), passed to the summary writer. Usage text updated.
- JSON: unchanged, so `schemaVersion` stays 1.
- The "not in this report yet" line is removed.

## Testing (written first)

- Unit tests: duration formatting at each boundary (including negative and zero), review-facts text for each combination, author rendering (user, bot, missing), title sanitizing and truncation, cap header wording, empty sections.
- `testdata/report.golden.txt` extended by hand with both sections (not generated).
- Command tests: default limit, `--limit`, `--all`, `--limit 0` and `--limit` with `--all` rejected with exit 2, `--json` unaffected by both flags.

## Docs (same change)

- `docs/product.md`: close the open decision in section 6; add definitions for the row columns (Age, Updated, Review facts as already defined; humanized time rounding) to the definitions table.
- `docs/architecture.md`: update the "Current state" paragraph and the file responsibilities.

## Out of scope

Filters, per-section sorts, clickable terminal links, color, width-aware wrapping, showing base branch.
