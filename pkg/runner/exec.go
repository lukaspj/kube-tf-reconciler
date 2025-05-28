package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfreconcilev1alpha1 "lukaspj.io/kube-tf-reconciler/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Exec struct {
	RootDir        string
	installDir     string
	WorkspacesDir  string
	PluginCacheDir string

	terraformInstalledVersions map[string]string
	terraformInstallMutex      sync.RWMutex
	providerInitMutex          sync.Mutex
}

func New(rootDir string) *Exec {
	var err error
	rootDir, err = filepath.Abs(rootDir)
	if err != nil {
		panic(fmt.Errorf("failed to get absolute path for root dir: %w", err))
	}

	err = os.MkdirAll(rootDir, 0755)
	if err != nil {
		panic(err)
	}

	installDir := filepath.Join(rootDir, "installs")
	workspacesDir := filepath.Join(rootDir, "workspaces")
	err = os.MkdirAll(installDir, 0755)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(workspacesDir, 0755)
	if err != nil {
		panic(err)
	}
	pluginCacheDir := filepath.Join(rootDir, "plugin-cache")
	err = os.MkdirAll(pluginCacheDir, 0755)
	if err != nil {
		panic(err)
	}

	return &Exec{
		RootDir:                    rootDir,
		installDir:                 installDir,
		WorkspacesDir:              workspacesDir,
		PluginCacheDir:             pluginCacheDir,
		terraformInstalledVersions: make(map[string]string),
	}
}

func (e *Exec) SetupWorkspace(ws string) (string, error) {
	fullPath := filepath.Join(e.WorkspacesDir, ws)
	err := os.MkdirAll(fullPath, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create workspace dir: %w", err)
	}

	return fullPath, nil
}

// SetupTerraformRC creates a .terraformrc file in the workspace directory if content is provided
func (e *Exec) SetupTerraformRC(workspacePath string, terraformRCContent string) (string, error) {
	if terraformRCContent == "" {
		return "", nil // No custom .terraformrc provided
	}

	// Create the config file in the workspace's directory to isolate configuration
	terraformRCPath := filepath.Join(workspacePath, ".terraformrc")

	err := os.WriteFile(terraformRCPath, []byte(terraformRCContent), 0600)
	if err != nil {
		return "", fmt.Errorf("failed to write .terraformrc file: %w", err)
	}

	return terraformRCPath, nil
}

func (e *Exec) TerraformInit(ctx context.Context, tf *tfexec.Terraform, opts ...tfexec.InitOption) error {
	log := logf.FromContext(ctx)

	// Check cache before init
	if entries, err := os.ReadDir(e.PluginCacheDir); err == nil {
		log.Info("plugin cache before init", "cached_items", len(entries))
		for _, entry := range entries[:min(5, len(entries))] { // Log first 5
			if info, err := entry.Info(); err == nil {
				log.Info("cached provider", "name", entry.Name(), "size", info.Size(), "modified", info.ModTime())
			}
		}
	}

	log.Info("Initializing Terraform", "workspace", tf.WorkingDir(), "cache_dir", e.PluginCacheDir, "time", time.Now().Format(time.RFC3339))

	e.providerInitMutex.Lock()
	defer e.providerInitMutex.Unlock()

	start := time.Now()
	err := tf.Init(ctx, opts...)
	duration := time.Since(start)

	log.Info("Terraform initialization completed", "workspace", tf.WorkingDir(), "duration", duration, "time", time.Now().Format(time.RFC3339))

	// Check if provider was re-downloaded
	awsProviderPath := filepath.Join(e.PluginCacheDir, "registry.terraform.io/hashicorp/aws/5.98.0/linux_amd64/terraform-provider-aws_v5.98.0_x5")
	if info, err := os.Stat(awsProviderPath); err == nil {
		log.Info("AWS provider after init", "modified", info.ModTime(), "size", info.Size())
	}

	return err
}
func (e *Exec) getTerraformBinary(ctx context.Context, terraformVersion string) (string, error) {
	log := logf.FromContext(ctx)
	log.Info("Checking for existing Terraform binary", "version", terraformVersion, "time", time.Now().Format(time.RFC3339))
	e.terraformInstallMutex.Lock()
	defer e.terraformInstallMutex.Unlock()
	if execPath, exists := e.terraformInstalledVersions[terraformVersion]; exists {
		// Verify the binary still exists
		if _, err := os.Stat(execPath); err == nil {
			log.Info("Found existing Terraform binary", "version", terraformVersion, "path", time.Now().Format(time.RFC3339))
			return execPath, nil
		}
		// If it doesn't exist or was deleted, remove from cache
		delete(e.terraformInstalledVersions, terraformVersion)
	}

	// Not installed or missing — do the install
	installer := &releases.ExactVersion{
		Product:    product.Terraform,
		InstallDir: e.installDir,
		Version:    version.Must(version.NewVersion(terraformVersion)),
	}
	installer.Timeout = 2 * time.Minute

	execPath, err := installer.Install(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to install terraform: %w", err)
	}

	e.terraformInstalledVersions[terraformVersion] = execPath
	log.Info("Installed Terraform binary", "version", terraformVersion, "time", time.Now().Format(time.RFC3339))
	return execPath, nil
}

func (e *Exec) GetTerraformForWorkspace(ctx context.Context, ws tfreconcilev1alpha1.Workspace) (*tfexec.Terraform, string, error) {
	path, err := e.SetupWorkspace(filepath.Join(ws.Namespace, ws.Name))
	if err != nil {
		return nil, "", fmt.Errorf("failed to setup workspace: %w", err)
	}

	var terraformRCPath string
	if ws.Spec.TerraformRC != "" {
		terraformRCPath, err = e.SetupTerraformRC(path, ws.Spec.TerraformRC)
		if err != nil {
			return nil, "", fmt.Errorf("failed to setup .terraformrc: %w", err)
		}
	}

	execPath, err := e.getTerraformBinary(ctx, ws.Spec.TerraformVersion)
	if err != nil {
		return nil, "", err
	}

	tf, err := tfexec.NewTerraform(path, execPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create terraform instance: %w", err)
	}

	return tf, terraformRCPath, nil
}
