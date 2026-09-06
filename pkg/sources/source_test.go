package sources

import (
	"testing"
)

func TestParseSource(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   SourceRef
		ok     bool
	}{
		{
			name:   "github shorthand with ref and subdir",
			source: "github.com/org/repo//modules/vpc?ref=main",
			want:   SourceRef{RepoURL: "https://github.com/org/repo", Ref: "main", Subdir: "modules/vpc"},
			ok:     true,
		},
		{
			name:   "git prefix https",
			source: "git::https://github.com/org/repo.git?ref=v1.2.3",
			want:   SourceRef{RepoURL: "https://github.com/org/repo.git", Ref: "v1.2.3"},
			ok:     true,
		},
		{
			name:   "git prefix ssh with subdir",
			source: "git::ssh://git@github.com/org/repo//modules/db",
			want:   SourceRef{RepoURL: "ssh://git@github.com/org/repo", Subdir: "modules/db"},
			ok:     true,
		},
		{
			name:   "scp style",
			source: "git@github.com:org/repo.git?ref=develop",
			want:   SourceRef{RepoURL: "git@github.com:org/repo.git", Ref: "develop"},
			ok:     true,
		},
		{
			name:   "plain https",
			source: "https://gitlab.example.com/org/repo",
			want:   SourceRef{RepoURL: "https://gitlab.example.com/org/repo"},
			ok:     true,
		},
		{
			name:   "registry source rejected",
			source: "hashicorp/consul/aws",
			ok:     false,
		},
		{
			name:   "local path rejected",
			source: "./modules/vpc",
			ok:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseSource(tc.source)
			if ok != tc.ok {
				t.Fatalf("ParseSource(%q) ok = %v, want %v", tc.source, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("ParseSource(%q) = %+v, want %+v", tc.source, got, tc.want)
			}
		})
	}
}

func TestSourceRefRepoName(t *testing.T) {
	cases := map[string]string{
		"https://github.com/Org/Repo.git":  "github.com/org/repo",
		"git@github.com:org/repo.git":      "github.com/org/repo",
		"ssh://git@github.com/org/repo":    "github.com/org/repo",
		"https://gitlab.example.com/a/b":   "gitlab.example.com/a/b",
		"https://github.com/org/repo//sub": "github.com/org/repo",
	}
	for url, want := range cases {
		if got := (SourceRef{RepoURL: url}).RepoName(); got != want {
			t.Errorf("RepoName(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestMatchesPush(t *testing.T) {
	ref := SourceRef{RepoURL: "https://github.com/org/repo", Ref: "main"}
	if !ref.MatchesPush("https://github.com/org/repo.git", "main") {
		t.Error("expected same repo and branch to match")
	}
	if !ref.MatchesPush("git@github.com:Org/Repo.git", "main") {
		t.Error("expected normalized repo to match regardless of scheme/case")
	}
	if ref.MatchesPush("https://github.com/org/other.git", "main") {
		t.Error("expected different repo not to match")
	}
	if ref.MatchesPush("https://github.com/org/repo.git", "dev") {
		t.Error("expected different branch not to match")
	}

	// Ref-less sources track HEAD only.
	defaultRef := SourceRef{RepoURL: "https://github.com/org/repo"}
	if !defaultRef.MatchesPush("https://github.com/org/repo.git", "HEAD") {
		t.Error("expected default ref to match HEAD push")
	}
	if !defaultRef.MatchesPush("https://github.com/org/repo.git", "") {
		t.Error("expected default ref to match empty-branch push")
	}
	if defaultRef.MatchesPush("https://github.com/org/repo.git", "main") {
		t.Error("expected default ref not to match a named-branch push")
	}
}
