#!/bin/sh
# Checks that a commit message subject follows Conventional Commits:
#   <type>(<optional scope>)!: <description>
# The same types are configured for the PR title check in
# .github/workflows/pr-title.yml.

set -eu

subject=$(head -n 1 "$1")

# Messages git writes itself are not subject to the format.
case "$subject" in
  "Merge "* | "Revert "* | "fixup! "* | "squash! "*) exit 0 ;;
esac

types='build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test'

if printf '%s\n' "$subject" | grep -Eq "^($types)(\([^()]+\))?!?: [^ ]"; then
  exit 0
fi

cat >&2 <<EOF
Commit subject does not follow Conventional Commits:

  $subject

Expected: <type>(<optional scope>)!: <description>
Types:    $(printf '%s' "$types" | tr '|' ' ')
Example:  feat(report): list the oldest non-draft pull requests
EOF
exit 1
