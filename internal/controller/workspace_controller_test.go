package controller

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	tfreconcilev1alpha1 "lukaspj.io/kube-tf-reconciler/api/v1alpha1"
	"lukaspj.io/kube-tf-reconciler/internal/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestWorkspaceController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "crds")},
		BinaryAssetsDirectory: testutils.GetFirstFoundEnvTestBinaryDir(),
		ErrorIfCRDPathMissing: true,
		Scheme:                k8sscheme.Scheme,
	}

	err := tfreconcilev1alpha1.AddToScheme(testEnv.Scheme)
	assert.NoError(t, err)

	cfg, err := testEnv.Start()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	k8sClient, err := client.New(cfg, client.Options{Scheme: testEnv.Scheme})

	const resourceName = "test-resource"
	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: "default",
	}
	workspace := &tfreconcilev1alpha1.Workspace{}

	t.Run("creating the custom resource for the Kind Workspace", func(t *testing.T) {
		err := k8sClient.Get(ctx, typeNamespacedName, workspace)
		if err != nil && errors.IsNotFound(err) {
			resource := &tfreconcilev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
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
			assert.NoError(t, k8sClient.Create(ctx, resource))
		}
	})
}
