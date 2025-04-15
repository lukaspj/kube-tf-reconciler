package controller

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	tfreconcilev1alpha1 "lukaspj.io/kube-tf-reconciler/api/v1alpha1"
	"lukaspj.io/kube-tf-reconciler/internal/testutils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/support/kind"
)

func TestWorkspace(t *testing.T) {
	kindClusterName := "krec-cluster" //envconf.RandomName("my-cluster", 16)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	k := kind.NewCluster(kindClusterName)
	_, err := k.Create(ctx)
	require.NoError(t, err)

	client, err := klient.New(k.KubernetesRestConfig())
	require.NoError(t, err)
	err = k.WaitForControlPlane(ctx, client)
	require.NoError(t, err)
	//defer k.Destroy(ctx)

	err = testutils.SetupCRDs(client, ctx, testutils.CRDFolder(), "*")
	assert.NoError(t, err)
	t.Cleanup(func() {
		err = testutils.TeardownCRDs(client, context.Background(), testutils.CRDFolder(), "*")
		assert.NoError(t, err)
	})

	t.Run("test 1", func(t *testing.T) {

		s := scheme.Scheme
		utilruntime.Must(clientgoscheme.AddToScheme(s))
		utilruntime.Must(tfreconcilev1alpha1.AddToScheme(s))

		slog.SetLogLoggerLevel(slog.LevelInfo)
		ctrl.SetLogger(logr.FromSlogHandler(slog.Default().Handler()))

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                  s,
			HealthProbeBindAddress:  "0",
			LeaderElectionNamespace: "default",
			LeaderElection:          false,
			LeaderElectionID:        "krec-leader",
		})
		assert.NoError(t, err)

		reconciler := &WorkspaceReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		kl, err := klient.New(mgr.GetConfig())
		assert.NoError(t, err)

		err = reconciler.SetupWithManager(mgr)
		assert.NoError(t, err)

		go func() {
			err = mgr.Start(ctrl.SetupSignalHandler())
			assert.NoError(t, err)
		}()

		<-mgr.Elected()

		err = kl.Resources().Create(ctx, newWorkspace())
		assert.NoError(t, err)

		err = testutils.WaitPod(kl, ctx, "default", "worker")
		assert.NoError(t, err)
	})
}

func newWorkspace() *tfreconcilev1alpha1.Workspace {
	return &tfreconcilev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "default",
		},
		Spec: tfreconcilev1alpha1.WorkspaceSpec{
			Backend: tfreconcilev1alpha1.BackendSpec{
				Type: "s3",
				Inputs: &apiextensionsv1.JSON{
					Raw: []byte(`{"bucket": "my-bucket"}`),
				},
			},
			ProviderRefs: []tfreconcilev1alpha1.ProviderRef{
				{
					Name:      "aws",
					Namespace: "default",
				},
			},
			Module: &tfreconcilev1alpha1.ModuleSpec{
				Source:  "terraform-aws-modules/vpc/aws",
				Version: "5.19.0",
			},
			WorkerSpec: &v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:       "worker",
						Image:      "hashicorp/terraform:1.11",
						Args:       []string{"version"},
						WorkingDir: "/workspace",
						EnvFrom:    nil,
						Env:        nil,
						Resources:  v1.ResourceRequirements{},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      "workspace",
								MountPath: "/workspace",
							},
						},
						LivenessProbe:   nil,
						ReadinessProbe:  nil,
						StartupProbe:    nil,
						Lifecycle:       nil,
						ImagePullPolicy: v1.PullIfNotPresent,
						SecurityContext: nil,
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "workspace",
						VolumeSource: v1.VolumeSource{
							ConfigMap: &v1.ConfigMapVolumeSource{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "workspace-config",
								},
							},
						},
					},
				},
			},
		},
	}
}
