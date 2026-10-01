# gh-pramen

A gh CLI extension in Go that reports a repository's open pull request backlog. Read `docs/product.md` for what it should do and `docs/architecture.md` for how it is built before changing behavior.

## Guardrails

- **Scope:** one repository, one command, one report. Do not add anything from "Deferred until justified" in `docs/product.md` (DORA, cycle time, caching, multi-repo, TUI, and so on) unless the user asks for it.
- **Facts before interpretations:** report observed state with its definition. Never label PRs as abandoned, stale, unreviewed, or ready to merge.
- **No individual metrics:** no author rankings, reviewer leaderboards, or per-person aggregates.
- **Visible limitations:** a failed or partial fetch must never look like zero activity. Fail clearly instead.
- **Read-only:** the tool never writes to GitHub.
- **Privacy:** `NOTES.local.md` holds personal and work context and is gitignored. Do not copy work-repository names, numbers, or PR details into committed files.

## Commands

```sh
go test ./...                                # no network needed
golangci-lint run && golangci-lint fmt --diff
go build -o gh-pramen . && ./gh-pramen -R owner/repo          # add --json for the full report
```

## Commits and pull requests

- `main` only takes squash-merged pull requests; the PR title becomes the commit subject.
- Commit subjects and PR titles follow Conventional Commits with the types in `scripts/commit-msg.sh`: `type(scope): description`, `!` for breaking changes. Scopes are free-form and optional (`fetch`, `report`, `render`, `docs`, `ci`, `deps`).
- Hooks run through lefthook (`lefthook.yml`): format and lint on commit, message check, tests on push.

## Conventions

- One `main` package: `main.go` (flags, wiring), `fetch.go` (GraphQL), `report.go` (pure calculations), `render.go` (JSON and terminal summary). Renderers only lay out what `buildReport` calculated. Keep fetch, calculate, and render separate; do not add packages or dependencies for future features.
- Write the test first. Fetch and command tests use the real `go-gh` client over the fake transport in `fetch_test.go`, with fixtures in `testdata/`.
- One reference time per run; every age is measured against it.
- The report goes to stdout; progress and errors go to stderr.
- JSON is a contract: camelCase keys, durations in seconds, counts always present. The golden files `testdata/report.golden.json` and `testdata/report.golden.txt` are written by hand, not generated. Update the JSON golden, the schema table in `docs/architecture.md`, and `schemaVersion` when it changes meaning.
- When a definition or decision changes, update `docs/product.md` in the same change.
