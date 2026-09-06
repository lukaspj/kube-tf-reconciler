package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	tfreconcilev1alpha1 "github.com/LEGO/kube-tf-reconciler/api/v1alpha1"
	"github.com/LEGO/kube-tf-reconciler/pkg/sources/webhook"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Watcher keeps the tf-reconcile.lego.com/module-revision annotation of each
// workspace in sync with the upstream module revision. Patching the
// annotation makes the operator reconcile the workspace immediately,
// bypassing the periodic refresh gate.
type Watcher struct {
	Client   client.Client
	Resolver Resolver
	// Namespace limits the watch to a single namespace (empty = all).
	Namespace string
}

// Poll resolves the upstream revision for every matching workspace module
// and patches the workspace annotation when the revision changed.
func (w *Watcher) Poll(ctx context.Context) error {
	workspaces, err := w.listWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("listing workspaces: %w", err)
	}
	for i := range workspaces {
		ws := &workspaces[i]
		w.reconcileRevision(ctx, ws)
	}
	return nil
}

// HandlePush resolves the upstream revision for every workspace whose module
// source matches the push and patches changed ones.
func (w *Watcher) HandlePush(ctx context.Context, push *webhook.Push) error {
	workspaces, err := w.listWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("listing workspaces: %w", err)
	}
	matched := 0
	for i := range workspaces {
		ws := &workspaces[i]
		if ws.Spec.Module == nil {
			continue
		}
		ref, ok := ParseSource(ws.Spec.Module.Source)
		if !ok || !ref.MatchesPush(push.RepoURL, push.Branch) {
			continue
		}
		matched++
		w.reconcileRevision(ctx, ws)
	}
	if matched == 0 {
		slog.DebugContext(ctx, "push matches no workspace",
			"repo", push.RepoURL, "branch", push.Branch)
	}
	return nil
}

// reconcileRevision resolves the current upstream revision for the workspace
// module and patches the annotation when it differs from the known one.
func (w *Watcher) reconcileRevision(ctx context.Context, ws *tfreconcilev1alpha1.Workspace) {
	if ws.Spec.Module == nil || !w.Resolver.Matches(ws.Spec.Module.Source) {
		return
	}

	sha, err := w.Resolver.Resolve(ctx, ws.Spec.Module.Source, ws.Spec.Module.Version)
	if err != nil {
		slog.ErrorContext(ctx, "resolving module revision failed",
			"namespace", ws.Namespace, "workspace", ws.Name,
			"source", ws.Spec.Module.Source, "error", err)
		return
	}

	current := ws.ModuleRevisions()
	if current[ws.Spec.Module.Source] == sha {
		return
	}

	old := ws.DeepCopy()
	if ws.Annotations == nil {
		ws.Annotations = make(map[string]string)
	}
	ws.Annotations[tfreconcilev1alpha1.ModuleRevisionAnnotation] = moduleRevisionsAnnotation(map[string]string{
		ws.Spec.Module.Source: sha,
	})
	if err := w.Client.Patch(ctx, ws, client.MergeFrom(old)); err != nil {
		slog.ErrorContext(ctx, "patching module revision annotation failed",
			"namespace", ws.Namespace, "workspace", ws.Name, "error", err)
		return
	}
	slog.InfoContext(ctx, "module revision changed, requested workspace refresh",
		"namespace", ws.Namespace, "workspace", ws.Name,
		"source", ws.Spec.Module.Source, "revision", sha)
}

func (w *Watcher) listWorkspaces(ctx context.Context) ([]tfreconcilev1alpha1.Workspace, error) {
	list := &tfreconcilev1alpha1.WorkspaceList{}
	if err := w.Client.List(ctx, list, client.InNamespace(w.Namespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// moduleRevisionsAnnotation renders the annotation value: a JSON map from
// module source to upstream revision.
func moduleRevisionsAnnotation(revisions map[string]string) string {
	data, err := json.Marshal(revisions)
	if err != nil {
		return ""
	}
	return string(data)
}

// parseModuleRevisionsAnnotation parses the annotation value.
func parseModuleRevisionsAnnotation(value string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var revisions map[string]string
	if err := json.Unmarshal([]byte(value), &revisions); err != nil {
		return nil, fmt.Errorf("parsing module revision annotation: %w", err)
	}
	if len(revisions) == 0 {
		return nil, nil
	}
	return revisions, nil
}

// WebhookHandler builds an HTTP handler dispatching webhook requests to the
// given providers. When a provider's secret is configured, requests are
// signature-verified before being processed.
func WebhookHandler(w *Watcher, providers ...webhook.Provider) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "reading body: "+err.Error(), http.StatusBadRequest)
			return
		}

		provider := findProvider(providers, r, body)
		if provider == nil {
			http.Error(rw, "no matching webhook provider", http.StatusBadRequest)
			return
		}
		if err := provider.Verify(r, body); err != nil {
			slog.WarnContext(r.Context(), "webhook signature verification failed",
				"provider", provider.Name(), "error", err)
			http.Error(rw, "signature verification failed", http.StatusUnauthorized)
			return
		}
		push, err := provider.Parse(r, body)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		if push == nil {
			rw.WriteHeader(http.StatusOK)
			return
		}
		if err := w.HandlePush(r.Context(), push); err != nil {
			slog.ErrorContext(r.Context(), "handling webhook push failed",
				"provider", provider.Name(), "error", err)
			http.Error(rw, "handling push failed", http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusOK)
	}
}

func findProvider(providers []webhook.Provider, r *http.Request, body []byte) webhook.Provider {
	for _, p := range providers {
		if p.Match(r, body) {
			return p
		}
	}
	return nil
}
