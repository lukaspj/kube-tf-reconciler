package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Header constants for GitHub webhooks.
const (
	GitHubEventHeader     = "X-GitHub-Event"
	GitHubSignatureHeader = "X-Hub-Signature-256"
)

// GitHub parses GitHub push webhook payloads.
type GitHub struct {
	secret string
}

// NewGitHub returns a GitHub webhook provider. When secret is non-empty,
// incoming payloads are verified against X-Hub-Signature-256.
func NewGitHub(secret string) *GitHub {
	return &GitHub{secret: secret}
}

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) Match(r *http.Request, _ []byte) bool {
	return r.Header.Get(GitHubEventHeader) != ""
}

func (g *GitHub) Verify(r *http.Request, body []byte) error {
	return VerifyHMACSHA256(r.Header.Get(GitHubSignatureHeader), body, g.secret)
}

type githubRepository struct {
	CloneURL string `json:"clone_url"`
}

type githubPushEvent struct {
	Ref        string           `json:"ref"`
	After      string           `json:"after"`
	Deleted    bool             `json:"deleted"`
	Repository githubRepository `json:"repository"`
}

// Parse extracts branch pushes. Non-push events and branch deletions yield a
// nil Push without error (nothing to reconcile).
func (g *GitHub) Parse(r *http.Request, body []byte) (*Push, error) {
	if r.Header.Get(GitHubEventHeader) != "push" {
		return nil, nil
	}
	var push githubPushEvent
	if err := json.Unmarshal(body, &push); err != nil {
		return nil, fmt.Errorf("parsing github payload: %w", err)
	}
	if push.Repository.CloneURL == "" {
		return nil, fmt.Errorf("github payload missing repository.clone_url")
	}
	if !strings.HasPrefix(push.Ref, "refs/heads/") {
		// Tags, PR refs etc. are not tracked by git module sources.
		return nil, nil
	}
	if push.Deleted {
		// Branch deleted, nothing to reconcile.
		return nil, nil
	}
	return &Push{
		RepoURL: push.Repository.CloneURL,
		Branch:  strings.TrimPrefix(push.Ref, "refs/heads/"),
		SHA:     push.After,
	}, nil
}
