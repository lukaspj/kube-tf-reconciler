// Package sources resolves upstream module sources to concrete revisions and
// translates webhook payloads into workspace-affecting push events.
package sources

import (
	"net/url"
	"strings"
)

// SourceRef is the parsed representation of a terraform git module source.
type SourceRef struct {
	// RepoURL is the repository URL usable with git ls-remote.
	RepoURL string
	// Ref is the tracked branch or tag (empty = HEAD).
	Ref string
	// Subdir is the module subdirectory inside the repository.
	Subdir string
}

// RepoName returns the normalized repository identity, used to match a
// webhook push against a workspace module source.
func (s SourceRef) RepoName() string {
	u := s.RepoURL
	u = strings.TrimPrefix(u, "ssh://")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "git@")
	// Drop the module subdirectory separator.
	if i := strings.Index(u, "//"); i >= 0 {
		u = u[:i]
	}
	// Normalize scp-style host:path to host/path so webhook clone URLs match.
	if i := strings.IndexByte(u, ':'); i >= 0 && !strings.Contains(u, "://") && !strings.Contains(u[:i], "/") {
		u = u[:i] + "/" + u[i+1:]
	}
	u = strings.TrimSuffix(u, ".git")
	return strings.ToLower(u)
}

// ParseSource parses a terraform module source and returns false when the
// source is not a git source (e.g. a registry reference or local path).
//
// Supported forms:
//   - git::https://example.com/org/repo.git//subdir?ref=branch
//   - git::git@github.com:org/repo.git//subdir?ref=branch
//   - git::ssh://git@example.com/org/repo//subdir
//   - github.com/org/repo//subdir?ref=branch
//   - https://github.com/org/repo//subdir?ref=branch
func ParseSource(source string) (SourceRef, bool) {
	ref := SourceRef{}
	s := source

	switch {
	case strings.HasPrefix(s, "git::"):
		s = strings.TrimPrefix(s, "git::")
	case strings.HasPrefix(s, "github::"):
		s = strings.TrimPrefix(s, "github::")
	}

	if s == "" {
		return ref, false
	}

	// Split off the query string.
	if i := strings.IndexByte(s, '?'); i >= 0 {
		query, err := url.ParseQuery(s[i+1:])
		if err == nil {
			ref.Ref = query.Get("ref")
		}
		s = s[:i]
	}

	// Split off the subdirectory (a "//" separator after the host part).
	if i := strings.Index(s, "://"); i >= 0 {
		if j := strings.Index(s[i+3:], "//"); j >= 0 {
			ref.Subdir = s[i+3+j+2:]
			s = s[:i+3+j]
		}
	} else if j := strings.Index(s, "//"); j >= 0 {
		ref.Subdir = s[j+2:]
		s = s[:j]
	}

	switch {
	case strings.Contains(s, "://"), strings.HasPrefix(s, "git@"):
		// Direct git URL, keep as-is.
	case strings.Contains(s, "github.com/"):
		s = "https://" + s
	default:
		// Registry or local source, not git.
		return ref, false
	}

	ref.RepoURL = s
	return ref, true
}

// SourceMatches reports whether a webhook push (repo, branch) targets the
// parsed module source.
func (s SourceRef) MatchesPush(pushRepo, pushBranch string) bool {
	if s.RepoName() != normalizeRepo(pushRepo) {
		return false
	}
	if s.Ref == "" {
		return pushBranch == "" || pushBranch == "HEAD"
	}
	return s.Ref == pushBranch
}

func normalizeRepo(repo string) string {
	return SourceRef{RepoURL: repo}.RepoName()
}
