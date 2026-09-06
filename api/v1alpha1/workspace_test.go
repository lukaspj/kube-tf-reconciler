package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestModuleRevisions(t *testing.T) {
	ws := &Workspace{}

	if got := ws.ModuleRevisions(); got != nil {
		t.Fatalf("expected nil revisions for missing annotation, got %v", got)
	}

	ws.Annotations = map[string]string{
		ModuleRevisionAnnotation: `{"github.com/org/repo?ref=main":"abc123"}`,
	}
	got := ws.ModuleRevisions()
	if len(got) != 1 || got["github.com/org/repo?ref=main"] != "abc123" {
		t.Fatalf("unexpected revisions: %v", got)
	}

	ws.Annotations[ModuleRevisionAnnotation] = "not json"
	if got := ws.ModuleRevisions(); got != nil {
		t.Fatalf("expected nil revisions for invalid annotation, got %v", got)
	}

	ws.Annotations[ModuleRevisionAnnotation] = "{}"
	if got := ws.ModuleRevisions(); got != nil {
		t.Fatalf("expected nil revisions for empty annotation, got %v", got)
	}
}

func TestModuleRevisionChanged(t *testing.T) {
	ws := &Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				ModuleRevisionAnnotation: `{"github.com/org/repo":"abc123"}`,
			},
		},
	}

	// No observed revisions yet -> annotation counts as changed.
	if !ws.ModuleRevisionChanged() {
		t.Fatal("expected changed when no revisions observed yet")
	}

	ws.Status.ObservedModuleRevisions = map[string]string{"github.com/org/repo": "abc123"}
	if ws.ModuleRevisionChanged() {
		t.Fatal("expected unchanged when observed revisions match")
	}

	ws.Status.ObservedModuleRevisions = map[string]string{"github.com/org/repo": "old"}
	if !ws.ModuleRevisionChanged() {
		t.Fatal("expected changed when observed revision differs")
	}
}
