package sources

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	tfreconcilev1alpha1 "github.com/LEGO/kube-tf-reconciler/api/v1alpha1"
	"github.com/LEGO/kube-tf-reconciler/pkg/sources/webhook"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeResolver struct {
	sha string
}

func (f *fakeResolver) Matches(string) bool { return true }

func (f *fakeResolver) Resolve(context.Context, string, string) (string, error) {
	return f.sha, nil
}

func newWorkspace(name, source string) *tfreconcilev1alpha1.Workspace {
	return &tfreconcilev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: tfreconcilev1alpha1.WorkspaceSpec{
			Module: &tfreconcilev1alpha1.ModuleSpec{
				Name:   "mod",
				Source: source,
			},
		},
	}
}

func newWatcherClient(t *testing.T, ws ...*tfreconcilev1alpha1.Workspace) *fake.ClientBuilder {
	t.Helper()
	swScheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(swScheme); err != nil {
		t.Fatal(err)
	}
	if err := tfreconcilev1alpha1.AddToScheme(swScheme); err != nil {
		t.Fatal(err)
	}
	builder := fake.NewClientBuilder().WithScheme(swScheme).WithStatusSubresource(&tfreconcilev1alpha1.Workspace{})
	for _, w := range ws {
		builder = builder.WithObjects(w)
	}
	return builder
}

func TestPollPatchesChangedRevision(t *testing.T) {
	ws := newWorkspace("ws1", "github.com/org/repo?ref=main")
	c := newWatcherClient(t, ws).Build()
	w := &Watcher{Client: c, Resolver: &fakeResolver{sha: "sha-1"}}

	if err := w.Poll(context.Background()); err != nil {
		t.Fatalf("poll failed: %s", err)
	}

	updated := &tfreconcilev1alpha1.Workspace{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(ws), updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.ModuleRevisions()["github.com/org/repo?ref=main"]; got != "sha-1" {
		t.Fatalf("expected annotation revision sha-1, got %q", got)
	}

	// Second poll with the same sha must not touch the object.
	before := updated.ResourceVersion
	if err := w.Poll(context.Background()); err != nil {
		t.Fatalf("poll failed: %s", err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(ws), updated); err != nil {
		t.Fatal(err)
	}
	if updated.ResourceVersion != before {
		t.Fatal("expected no patch when revision unchanged")
	}
}

func TestHandlePushMatchesOnlyMatchingWorkspaces(t *testing.T) {
	matching := newWorkspace("ws-match", "github.com/org/repo?ref=main")
	otherRepo := newWorkspace("ws-other-repo", "github.com/org/other?ref=main")
	otherBranch := newWorkspace("ws-other-branch", "github.com/org/repo?ref=dev")
	c := newWatcherClient(t, matching, otherRepo, otherBranch).Build()
	w := &Watcher{Client: c, Resolver: &fakeResolver{sha: "sha-2"}}

	if err := w.HandlePush(context.Background(), &webhook.Push{
		RepoURL: "https://github.com/org/repo.git",
		Branch:  "main",
		SHA:     "abc",
	}); err != nil {
		t.Fatalf("handle push failed: %s", err)
	}

	for name, want := range map[string]bool{"ws-match": true, "ws-other-repo": false, "ws-other-branch": false} {
		ws := &tfreconcilev1alpha1.Workspace{}
		ws.SetName(name)
		ws.SetNamespace("default")
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(ws), ws); err != nil {
			t.Fatal(err)
		}
		_, has := ws.ModuleRevisions()["github.com/org/repo?ref=main"]
		if name == "ws-other-branch" {
			_, has = ws.ModuleRevisions()["github.com/org/repo?ref=dev"]
		}
		if has != want {
			t.Fatalf("workspace %s: annotation present = %v, want %v", name, has, want)
		}
	}
}

func TestWebhookHandler(t *testing.T) {
	ws := newWorkspace("ws1", "github.com/org/repo?ref=main")
	c := newWatcherClient(t, ws).Build()
	w := &Watcher{Client: c, Resolver: &fakeResolver{sha: "sha-3"}}
	provider := webhook.NewGitHub("secret")
	handler := WebhookHandler(w, provider)

	body := []byte(`{"ref":"refs/heads/main","after":"abc","repository":{"clone_url":"https://github.com/org/repo.git"}}`)
	r := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(body))
	r.Header.Set(webhook.GitHubEventHeader, "push")
	r.Header.Set(webhook.GitHubSignatureHeader, signFor(t, body, "secret"))
	rec := httptest.NewRecorder()
	handler(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	updated := &tfreconcilev1alpha1.Workspace{}
	updated.SetName("ws1")
	updated.SetNamespace("default")
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(updated), updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.ModuleRevisions()["github.com/org/repo?ref=main"]; got != "sha-3" {
		t.Fatalf("expected revision sha-3 after webhook, got %q", got)
	}

	// Bad signature -> 401.
	rec = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(body))
	r.Header.Set(webhook.GitHubEventHeader, "push")
	r.Header.Set(webhook.GitHubSignatureHeader, "sha256=deadbeef")
	handler(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad signature, got %d", rec.Code)
	}

	// No matching provider -> 400.
	rec = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(body))
	handler(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without provider headers, got %d", rec.Code)
	}
}

func signFor(t *testing.T, body []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
