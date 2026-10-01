# gh pramen

*Untangle your pull requests.*

A [GitHub CLI](https://cli.github.com) extension that shows what is sitting open in a repository's pull request backlog: how much, how old, in what review state, and which PRs those are. It reports facts, not judgments, and has no per-person metrics.

**Status:** early. The terminal output is a summary; the per-PR rows are only in the JSON report so far.

## Install

The extension is not published yet. From a checkout:

```sh
go build -o gh-pramen .
gh extension install .
```

## Use

```sh
gh pramen                         # summary for the current repository
gh pramen -R owner/repo           # summary for one explicit repository
gh pramen -R owner/repo --json    # full report, including every open PR
```

```
Repository  acme/widgets
Collected   2026-09-30 12:00 UTC

Open pull requests
  Total      5
  Non-draft  4
  Draft      1

Last 30 days (since 2026-08-31 12:00 UTC)
  Opened                34
  Merged                25
  Closed without merge   3

Age of open pull requests
                Total  Non-draft  Draft
  under 7 days      1          1      0
  7-29 days         0          0      0
  30-89 days        2          2      0
  90+ days          2          1      1

Review facts for the 4 non-draft pull requests (one PR can count in several rows)
  With approvals                3
  With changes requested        1
  With pending review requests  2
    only from CODEOWNERS        1
  With none of these            0
```

The report goes to stdout; progress and errors go to stderr. The command only reads from GitHub, using your existing `gh` authentication. If the repository is not found, check which account is active with `gh auth status`.

## What the report contains

The JSON report has these parts; the summary shows all but the last.

| Part | Content |
|---|---|
| `openBacklog` | Open PRs, split into draft and non-draft |
| `recentFlow` | PRs opened, merged, and closed without merge in the last 30 days |
| `ageDistribution` | Open PRs aged under 7 days, 7–29 days, 30–89 days, and 90+ days |
| `reviewState` | Non-draft PRs with approvals, change requests, or pending review requests (noting those that come only from CODEOWNERS), plus GitHub's review decision where it reports one |
| `pullRequests` | Every open PR, oldest first, with its age, time since update, author, base branch, and review facts |

Each term has a precise meaning and known limitations; see [the definitions](docs/product.md#definitions). Two matter most: an old PR is not necessarily abandoned, and an approved PR is not necessarily ready to merge.

Until the terminal report lists individual PRs, `jq` can pull them from the JSON:

```sh
gh pramen -R owner/repo --json > backlog.json

# Non-draft PRs open for 90 days or more, oldest first
jq -r '.pullRequests[] | select(.isDraft | not) | select(.ageSeconds >= 90 * 86400)
  | [(.ageSeconds / 86400 | floor | tostring) + "d", "#\(.number)", .title, .url] | @tsv' backlog.json
```

## Documentation

- [docs/product.md](docs/product.md): who it is for, principles, the report, definitions, and what is deferred
- [docs/architecture.md](docs/architecture.md): how it is built, the JSON schema, and API findings
- [docs/research.md](docs/research.md): background on related tools and metric terminology

## Development

```sh
go test ./...
go vet ./...
```

Tests run against fixtures in `testdata/` and need no network or credentials.

## License

[GNU AGPL v3](LICENSE).
