package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-exec/tfexec"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	tfreconcilev1alpha1 "lukaspj.io/kube-tf-reconciler/api/v1alpha1"
	"lukaspj.io/kube-tf-reconciler/pkg/render"
	"lukaspj.io/kube-tf-reconciler/pkg/runner"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	Tf *runner.Exec
}

func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	reqStart := time.Now()
	var ws tfreconcilev1alpha1.Workspace
	if err := r.Client.Get(ctx, req.NamespacedName, &ws); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get workspace %s: %w", req.String(), err)
		}

		return ctrl.Result{}, nil
	}

	if r.dueForRefresh(reqStart, ws) {

	}

	if r.alreadyProcessedOnce(ws) {
		log.Info("already processed workspace, skipping")
		return ctrl.Result{}, nil
	}

	tf, err := r.Tf.GetTerraformForWorkspace(ctx, ws)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get terraform executable %s: %w", req.String(), err)
	}

	f := hclwrite.NewEmptyFile()
	err = render.Workspace(f.Body(), ws)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render workspace %s: %w", req.String(), err)
	}

	err = render.Module(f.Body(), ws.Spec.Module)

	err = os.WriteFile(filepath.Join(tf.WorkingDir(), "main.tf"), f.Bytes(), 0644)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to write workspace %s: %w", req.String(), err)
	}

	ws.Status.CurrentRender = string(f.Bytes())
	err = r.Client.Status().Update(ctx, &ws)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update workspace status %s: %w", req.String(), err)
	}

	err = tf.Init(ctx, tfexec.Upgrade(true))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init workspace: %w", err)
	}

	changed, err := tf.Plan(ctx, tfexec.Out("plan.out"))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to plan: %w", err)
	}
	plan, err := tf.ShowPlanFileRaw(ctx, "plan.out")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to show plan file: %w", err)
	}
	r.Recorder.Eventf(&ws, v1.EventTypeNormal, "Planned", "Workspace %s planned", req.String())
	ws.Status.LatestPlan = plan
	err = r.Client.Status().Update(ctx, &ws)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update workspace status %s: %w", req.String(), err)
	}
	log.WithValues("changed", changed).Info("planned workspace")

	ws.Status.ObservedGeneration = ws.Generation
	return ctrl.Result{}, r.Client.Status().Update(ctx, &ws)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tfreconcilev1alpha1.Workspace{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *WorkspaceReconciler) alreadyProcessedOnce(ws tfreconcilev1alpha1.Workspace) bool {
	return ws.Status.ObservedGeneration == ws.Generation
}

func (r *WorkspaceReconciler) dueForRefresh(t time.Time, ws tfreconcilev1alpha1.Workspace) bool {
	return t.After(ws.Status.NextRefreshTimestamp.Time)
}

func (r *WorkspaceReconciler) refreshState(ctx context.Context, ws tfreconcilev1alpha1.Workspace) error {
	_, err := r.Tf.GetTerraformForWorkspace(ctx, ws)
	if err != nil {
		return fmt.Errorf("refreshState: failed to get terraform executable %s: %w", ws.Name, err)
	}

	ws.Status.NextRefreshTimestamp = metav1.NewTime(time.Now().Add(time.Minute * 5))
	return r.Client.Status().Update(ctx, &ws)
}
