package sources

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Resolver resolves a module source to a concrete upstream revision (SHA).
type Resolver interface {
	// Matches reports whether the resolver can handle the given source.
	Matches(source string) bool
	// Resolve returns the current upstream revision for the source.
	Resolve(ctx context.Context, source string, version string) (string, error)
}

// GitResolver resolves git module sources via `git ls-remote`.
type GitResolver struct{}

// NewGitResolver returns a Resolver for git module sources.
func NewGitResolver() *GitResolver {
	return &GitResolver{}
}

func (g *GitResolver) Matches(source string) bool {
	_, ok := ParseSource(source)
	return ok
}

// Resolve runs `git ls-remote` against the source repository and returns the
// HEAD commit of the tracked ref. The ref is taken from the source query
// string (?ref=), falling back to the module version and then to HEAD.
//
// Authentication relies on the ambient git configuration of the process
// (e.g. mounted SSH keys, netrc or credential helpers).
func (g *GitResolver) Resolve(ctx context.Context, source string, version string) (string, error) {
	ref, ok := ParseSource(source)
	if !ok {
		return "", fmt.Errorf("source %q is not a git source", source)
	}

	pattern := ref.Ref
	if pattern == "" {
		pattern = version
	}
	if pattern == "" {
		pattern = "HEAD"
	}

	out, err := exec.CommandContext(ctx, "git", "ls-remote", ref.RepoURL, pattern).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git ls-remote %s %s: %s: %s", ref.RepoURL, pattern, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git ls-remote %s %s: %w", ref.RepoURL, pattern, err)
	}

	sha := firstSHA(out)
	if sha == "" {
		return "", fmt.Errorf("git ls-remote %s %s: no matching refs", ref.RepoURL, pattern)
	}
	return sha, nil
}

func firstSHA(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		sha, _, found := strings.Cut(strings.TrimSpace(line), "\t")
		if found && len(sha) == 40 {
			return sha
		}
	}
	return ""
}

var _ Resolver = (*GitResolver)(nil)
