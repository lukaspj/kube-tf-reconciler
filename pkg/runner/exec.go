package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfreconcilev1alpha1 "lukaspj.io/kube-tf-reconciler/api/v1alpha1"
)

type Exec struct {
	RootDir       string
	installDir    string
	WorkspacesDir string
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

	return &Exec{
		RootDir:       rootDir,
		installDir:    installDir,
		WorkspacesDir: workspacesDir,
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

func (e *Exec) GetTerraformForWorkspace(ctx context.Context, ws tfreconcilev1alpha1.Workspace) (*tfexec.Terraform, error) {
	path, err := e.SetupWorkspace(filepath.Join(ws.Namespace, ws.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to setup workspace: %w", err)
	}

	// Create detailed logger that writes to stdout
	consoleLogger := log.New(os.Stdout, "TF-Install: ", log.LstdFlags)
	consoleLogger.Printf("Setting up Terraform %s in dir %s", ws.Spec.TerraformVersion, e.installDir)

	// Check if install directory exists and is writable
	fileInfo, err := os.Stat(e.installDir)
	if err != nil {
		consoleLogger.Printf("Install dir stat error: %v", err)
	} else {
		consoleLogger.Printf("Install dir exists: %v, mode: %s", fileInfo.IsDir(), fileInfo.Mode().String())
	}

	// Try creating a test file to verify write permissions
	testFile := filepath.Join(e.installDir, "test_permissions.txt")
	tf, err := os.CreateTemp(e.installDir, "test_permissions_*.txt")
	if err != nil {
		consoleLogger.Printf("Failed to create test file in install dir: %v", err)
	} else {
		tf.Close()
		consoleLogger.Printf("Successfully created test file: %s", testFile)
		// Clean up
		os.Remove(testFile)
	}

	// List directory contents before installation
	files, err := os.ReadDir(e.installDir)
	if err != nil {
		consoleLogger.Printf("Failed to list install dir contents: %v", err)
	} else {
		consoleLogger.Printf("Install dir contains %d items", len(files))
		for _, file := range files {
			consoleLogger.Printf("- %s (dir: %v, size: %d)", file.Name(), file.IsDir(), file.Type())
		}
	}

	installer := &releases.ExactVersion{
		Product:    product.Terraform,
		InstallDir: e.installDir,
		Version:    version.Must(version.NewVersion(ws.Spec.TerraformVersion)),
	}

	installer.SetLogger(consoleLogger)

	ctx = context.Background()
	// Set a timeout for the installation
	execPath, err := installer.Install(ctx)
	if err != nil {
		// Add more context to the error
		return nil, fmt.Errorf("failed to install terraform (dir %s): %w", e.installDir, err)
	}

	// Verify the binary exists after installation
	if _, statErr := os.Stat(execPath); statErr != nil {
		consoleLogger.Printf("After installation, binary not found at %s: %v", execPath, statErr)

		// Check if it's somewhere else in the directory
		if dirFiles, readErr := os.ReadDir(e.installDir); readErr == nil {
			consoleLogger.Printf("Install directory contents after installation:")
			for _, f := range dirFiles {
				consoleLogger.Printf("- %s", f.Name())
			}
		}
	} else {
		consoleLogger.Printf("Successfully verified binary at %s", execPath)
	}

	terraform, err := tfexec.NewTerraform(path, execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create terraform instance: %w", err)
	}

	return terraform, nil
}
