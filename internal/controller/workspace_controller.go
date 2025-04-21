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

	envs, err := r.getEnvsForExecution(ctx, ws)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get envs for execution: %w", err)
	}

	tf, err := r.Tf.GetTerraformForWorkspace(ctx, ws)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get terraform executable %s: %w", req.String(), err)
	}

	envs["HOME"] = os.Getenv("HOME")
	envs["PATH"] = os.Getenv("PATH")

	err = tf.SetEnv(envs)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set terraform env: %w", err)
	}

	result, err := r.renderHcl(tf.WorkingDir(), ws)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render workspace %s: %w", req.String(), err)
	}

	ws.Status.CurrentRender = string(result)
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

func (r *WorkspaceReconciler) renderHcl(workspaceDir string, ws tfreconcilev1alpha1.Workspace) ([]byte, error) {
	f := hclwrite.NewEmptyFile()
	err := render.Workspace(f.Body(), ws)
	renderErr := fmt.Errorf("failed to render workspace %s/%s", ws.Namespace, ws.Name)
	if err != nil {
		return f.Bytes(), fmt.Errorf("%w: %w", renderErr, err)
	}

	err = render.Providers(f.Body(), ws.Spec.ProviderSpecs)
	if err != nil {
		return f.Bytes(), fmt.Errorf("%w: failed to render providers: %w", renderErr, err)
	}

	err = render.Module(f.Body(), ws.Spec.Module)
	if err != nil {
		return f.Bytes(), fmt.Errorf("%w: failed to render module: %w", renderErr, err)
	}

	err = os.WriteFile(filepath.Join(workspaceDir, "main.tf"), f.Bytes(), 0644)
	if err != nil {
		return f.Bytes(), fmt.Errorf("%w: failed to write workspace: %w", renderErr, err)
	}

	return f.Bytes(), nil
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

func (r *WorkspaceReconciler) getEnvsForExecution(ctx context.Context, ws tfreconcilev1alpha1.Workspace) (map[string]string, error) {
	if ws.Spec.TFExec == nil {
		return map[string]string{}, nil
	}
	if ws.Spec.TFExec.Env == nil {
		return map[string]string{}, nil
	}
	envs := make(map[string]string)
	for _, env := range ws.Spec.TFExec.Env {
		if env.Name == "" {
			continue
		}
		if env.Value != "" {
			envs[env.Name] = env.Value
			continue
		}
		if env.ConfigMapKeyRef != nil {
			var cm v1.ConfigMap
			err := r.Client.Get(ctx, client.ObjectKey{Namespace: ws.Namespace, Name: env.ConfigMapKeyRef.Name}, &cm)
			if err != nil {
				return nil, fmt.Errorf("failed to get configmap %s: %w", env.ConfigMapKeyRef.Name, err)
			}
			if val, ok := cm.Data[env.ConfigMapKeyRef.Key]; ok {
				envs[env.Name] = val
				continue
			}
		}
		if env.SecretKeyRef != nil {
			var secret v1.Secret
			err := r.Client.Get(ctx, client.ObjectKey{Namespace: ws.Namespace, Name: env.SecretKeyRef.Name}, &secret)
			if err != nil {
				return nil, fmt.Errorf("failed to get secret %s: %w", env.SecretKeyRef.Name, err)
			}
			if val, ok := secret.Data[env.SecretKeyRef.Key]; ok {
				envs[env.Name] = string(val)
				continue
			}
		}
	}

	return envs, nil
}
