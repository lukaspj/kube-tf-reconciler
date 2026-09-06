package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func signBody(t *testing.T, body []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func pushPayload(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"ref":        "refs/heads/main",
		"after":      "abc123",
		"deleted":    false,
		"repository": map[string]string{"clone_url": "https://github.com/org/repo.git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func githubRequest(event string, body []byte, signature string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(body))
	r.Header.Set(GitHubEventHeader, event)
	if signature != "" {
		r.Header.Set(GitHubSignatureHeader, signature)
	}
	return r
}

func TestGitHubParse(t *testing.T) {
	g := NewGitHub("")

	push, err := g.Parse(githubRequest("push", pushPayload(t), ""), pushPayload(t))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if push == nil {
		t.Fatal("expected push")
	}
	if push.RepoURL != "https://github.com/org/repo.git" || push.Branch != "main" || push.SHA != "abc123" {
		t.Fatalf("unexpected push: %+v", push)
	}

	// Non-push events and branch deletions are valid but produce no push.
	for _, event := range []string{"ping", "pull_request"} {
		push, err = g.Parse(githubRequest(event, pushPayload(t), ""), pushPayload(t))
		if err != nil || push != nil {
			t.Fatalf("event %q: expected nil push without error, got %+v, %v", event, push, err)
		}
	}

	deleted, err := json.Marshal(map[string]any{
		"ref":        "refs/heads/main",
		"deleted":    true,
		"repository": map[string]string{"clone_url": "https://github.com/org/repo.git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	push, err = g.Parse(githubRequest("push", deleted, ""), deleted)
	if err != nil || push != nil {
		t.Fatalf("expected deleted branch to yield nil push, got %+v, %v", push, err)
	}

	// Tags are ignored.
	tagged, err := json.Marshal(map[string]any{
		"ref":        "refs/tags/v1.0.0",
		"after":      "abc123",
		"repository": map[string]string{"clone_url": "https://github.com/org/repo.git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	push, err = g.Parse(githubRequest("push", tagged, ""), tagged)
	if err != nil || push != nil {
		t.Fatalf("expected tag push to yield nil push, got %+v, %v", push, err)
	}
}

func TestGitHubVerify(t *testing.T) {
	const secret = "hunter2"
	body := pushPayload(t)
	g := NewGitHub(secret)

	if err := g.Verify(githubRequest("push", body, signBody(t, body, secret)), body); err != nil {
		t.Fatalf("expected valid signature to verify, got %s", err)
	}
	if err := g.Verify(githubRequest("push", body, signBody(t, body, "wrong")), body); err == nil {
		t.Fatal("expected wrong secret to fail verification")
	}
	if err := g.Verify(githubRequest("push", body, ""), body); err == nil {
		t.Fatal("expected missing signature to fail verification when secret configured")
	}

	// No secret configured -> unsigned payloads accepted.
	unsigned := NewGitHub("")
	if err := unsigned.Verify(githubRequest("push", body, ""), body); err != nil {
		t.Fatalf("expected unsigned payload to be accepted without secret, got %s", err)
	}
}
