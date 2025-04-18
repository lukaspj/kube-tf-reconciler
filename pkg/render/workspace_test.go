package render

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	tfreconcilev1alpha1 "lukaspj.io/kube-tf-reconciler/api/v1alpha1"
)

func TestRenderWorkspace_Success(t *testing.T) {
	f := hclwrite.NewEmptyFile()
	ws := tfreconcilev1alpha1.Workspace{
		Spec: tfreconcilev1alpha1.WorkspaceSpec{
			ProviderSpecs: []tfreconcilev1alpha1.ProviderSpec{
				{
					Name:    "aws",
					Source:  "hashicorp/aws",
					Version: ">= 5.40.0",
				},
			},
			Backend: tfreconcilev1alpha1.BackendSpec{
				Type: "s3",
				Inputs: &apiextensionsv1.JSON{
					Raw: []byte(`{"bucket": "my-bucket"}`),
				},
			},
		},
	}

	expectedWs := `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.40.0"
    }
  }
  backend "s3" {
    bucket = "my-bucket"
  }
}
`
	err := Workspace(f.Body(), ws)

	assert.NoError(t, err)
	assert.Equal(t, expectedWs, string(f.Bytes()))
}
