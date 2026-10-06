package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// env holds what run needs from the outside world, so tests can replace it.
type env struct {
	stdout, stderr io.Writer
	now            func() time.Time
	currentRepo    func() (repository.Repository, error)
	newClient      func(host string) (graphQLClient, error)
}

func main() {
	os.Exit(run(os.Args[1:], env{
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		now:         time.Now,
		currentRepo: repository.Current,
		newClient: func(host string) (graphQLClient, error) {
			return api.NewGraphQLClient(api.ClientOptions{Host: host})
		},
	}))
}

const usage = `Show what is sitting open in a repository's pull request backlog.

Usage:
  gh pramen [flags]

Flags:
  -R, --repo [HOST/]OWNER/REPO   Repository to report on (default: the current repository)
      --json                     Write the full report as JSON, including every pull request
`

// run executes the command and returns its exit code. The report goes to
// stdout; progress and errors go to stderr.
func run(args []string, e env) int {
	var repoFlag string
	var asJSON bool
	fs := flag.NewFlagSet("gh pramen", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	fs.Usage = func() { fmt.Fprint(e.stderr, usage) }
	fs.StringVar(&repoFlag, "R", "", "")
	fs.StringVar(&repoFlag, "repo", "", "")
	fs.BoolVar(&asJSON, "json", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(e.stderr, "unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return 2
	}

	if err := report(repoFlag, asJSON, e); err != nil {
		fmt.Fprintln(e.stderr, err)
		return 1
	}
	return 0
}

func report(repoFlag string, asJSON bool, e env) error {
	repo, err := resolveRepo(repoFlag, e)
	if err != nil {
		return err
	}
	client, err := e.newClient(repo.Host)
	if err != nil {
		return err
	}

	now := referenceTime(e.now())
	fullName := repo.Owner + "/" + repo.Name

	prs, err := fetchOpenPRs(client, repo.Owner, repo.Name, func(fetched, total int) {
		fmt.Fprintf(e.stderr, "Fetched %d of %d open pull requests from %s\n", fetched, total, fullName)
	})
	if err != nil {
		return err
	}
	flow, err := fetchFlow(client, repo.Owner, repo.Name, flowSince(now))
	if err != nil {
		return err
	}

	r := buildReport(fullName, prs, flow, now)
	if asJSON {
		return writeJSON(e.stdout, r)
	}
	return writeSummary(e.stdout, r, 0)
}

func resolveRepo(repoFlag string, e env) (repository.Repository, error) {
	if repoFlag != "" {
		repo, err := repository.Parse(repoFlag)
		if err != nil {
			return repository.Repository{}, fmt.Errorf("invalid repository %q: %w", repoFlag, err)
		}
		return repo, nil
	}
	repo, err := e.currentRepo()
	if err != nil {
		return repository.Repository{}, fmt.Errorf("could not determine the current repository; run inside a repository or pass -R OWNER/REPO\n%w", err)
	}
	return repo, nil
}
