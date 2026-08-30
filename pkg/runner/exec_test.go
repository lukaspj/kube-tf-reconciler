package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	tofuexec "github.com/opentofu/tofu-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tfreconcilev1alpha1 "github.com/LEGO/kube-tf-reconciler/api/v1alpha1"
)

func TestChecksum(t *testing.T) {
	dir := t.TempDir()
	e := New(dir)
	ws := &tfreconcilev1alpha1.Workspace{
		ObjectMeta: v1.ObjectMeta{
			Name:      "workspace1",
			Namespace: "test-ns1",
		},
	}
	path := filepath.Join(dir, "workspaces", ws.Namespace, ws.Name, ".terraform", "f1.txt")
	err := os.MkdirAll(filepath.Dir(path), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(path, []byte("file1"), 0644)

	check, err := e.CalculateChecksum(ws)
	assert.NoError(t, err)
	assert.Equal(t, "c147efcfc2d7ea666a9e4f5187b115c90903f0fc896a56df9a6ef5d8f3fc9f31", check)
}

func TestWithOutputStream(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name string
		tool tfreconcilev1alpha1.Tool
		ver  string
	}{
		{"terraform", tfreconcilev1alpha1.ToolTerraform, "1.11.2"},
		{"tofu", tfreconcilev1alpha1.ToolOpenTofu, "1.9.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			e := New(dir)

			binary, err := e.getBinary(ctx, tt.tool, tt.ver)
			require.NoError(t, err)

			var tool IaCTool
			var runVersion func() error
			switch tt.tool {
			case tfreconcilev1alpha1.ToolTerraform:
				tf, err := tfexec.NewTerraform(dir, binary)
				require.NoError(t, err)
				tool = NewTerraformTool(tf)
				runVersion = func() error {
					_, _, err := tf.Version(ctx, false)
					return err
				}
			case tfreconcilev1alpha1.ToolOpenTofu:
				tofu, err := tofuexec.NewTofu(dir, binary)
				require.NoError(t, err)
				tool = NewTofuTool(tofu)
				runVersion = func() error {
					_, _, err := tofu.Version(ctx, false)
					return err
				}
			}

			var body string
			WithOutputStream(ctx, tool, func() {
				err := runVersion()
				require.NoError(t, err)
				err = runVersion()
				require.NoError(t, err)
			}, func(stdout, stderr string) {
				assert.NotEqual(t, body, stdout)
				assert.Truef(t, strings.HasPrefix(stdout, body), "received a different stdout than what we've had so far")
			})
		})
	}
}

func TestMultipleVersions(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		tool      tfreconcilev1alpha1.Tool
		versions  []string
	}{
		{"terraform", tfreconcilev1alpha1.ToolTerraform, []string{"1.11.2", "1.14.3"}},
		{"tofu", tfreconcilev1alpha1.ToolOpenTofu, []string{"1.8.3", "1.9.0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			e := New(dir)

			for _, wantVersion := range tt.versions {
				binary, err := e.getBinary(ctx, tt.tool, wantVersion)
				require.NoError(t, err)

				var got string
				switch tt.tool {
				case tfreconcilev1alpha1.ToolTerraform:
					tf, err := tfexec.NewTerraform(dir, binary)
					require.NoError(t, err)
					version, _, err := tf.Version(ctx, false)
					require.NoError(t, err)
					got = version.String()
				case tfreconcilev1alpha1.ToolOpenTofu:
					tofu, err := tofuexec.NewTofu(dir, binary)
					require.NoError(t, err)
					version, _, err := tofu.Version(ctx, false)
					require.NoError(t, err)
					got = version.String()
				}
				assert.Equal(t, wantVersion, got)
			}
		})
	}
}

func TestGetIaCToolForWorkspaceToolSelection(t *testing.T) {
	ctx := t.Context()
	e := New(t.TempDir())

	tests := []struct {
		name         string
		ws           *tfreconcilev1alpha1.Workspace
		expectedTool any
	}{
		{
			name: "defaults to terraform",
			ws: &tfreconcilev1alpha1.Workspace{
				ObjectMeta: v1.ObjectMeta{Name: "ws", Namespace: "ns"},
				Spec: tfreconcilev1alpha1.WorkspaceSpec{
					TerraformVersion: "1.11.2",
				},
			},
			expectedTool: (*TerraformTool)(nil),
		},
		{
			name: "selects tofu",
			ws: &tfreconcilev1alpha1.Workspace{
				ObjectMeta: v1.ObjectMeta{Name: "ws", Namespace: "ns"},
				Spec: tfreconcilev1alpha1.WorkspaceSpec{
					Tool:             tfreconcilev1alpha1.ToolOpenTofu,
					TerraformVersion: "1.11.2",
					TofuVersion:      "1.9.0",
				},
			},
			expectedTool: (*TofuTool)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, _, err := e.GetIaCToolForWorkspace(ctx, tt.ws)
			require.NoError(t, err)
			assert.IsType(t, tt.expectedTool, tool)
		})
	}
}

func TestGetIaCToolForWorkspaceValidation(t *testing.T) {
	ctx := t.Context()
	e := New(t.TempDir())

	ws := &tfreconcilev1alpha1.Workspace{
		ObjectMeta: v1.ObjectMeta{Name: "ws", Namespace: "ns"},
		Spec: tfreconcilev1alpha1.WorkspaceSpec{
			Tool:            tfreconcilev1alpha1.ToolOpenTofu,
			TerraformVersion: "1.11.2",
		},
	}

	_, _, err := e.GetIaCToolForWorkspace(ctx, ws)
	assert.ErrorContains(t, err, "tofuVersion is required when tool is opentofu")

	ws.Spec.Tool = tfreconcilev1alpha1.Tool("unknown")
	ws.Spec.TofuVersion = "1.9.0"
	_, _, err = e.GetIaCToolForWorkspace(ctx, ws)
	assert.ErrorContains(t, err, "unsupported IaC tool")
}
