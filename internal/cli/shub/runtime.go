package shub

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

func createPythonVenv(venvDir, installDir string, runtime models.AssetRuntime) error {
	python, err := exec.LookPath("python3")
	if err != nil {
		return fmt.Errorf("python runtime requested but python3 is not available: %w", err)
	}
	cmd := exec.Command(python, "-m", "venv", venvDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create python venv: %w: %s", err, string(output))
	}

	if runtime.Install == nil {
		return nil
	}

	pipPath := filepath.Join(venvDir, "bin", "pip")
	if _, err := os.Stat(pipPath); err != nil {
		return nil
	}
	if runtime.Install.Path == "requirements.txt" {
		installCmd := exec.Command(pipPath, "install", "-r", filepath.Join(installDir, runtime.Install.Path))
		if output, err := installCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("install python requirements: %w: %s", err, string(output))
		}
	}
	return nil
}

func pythonRuntimeHealthy(venvDir string) bool {
	pythonPath := filepath.Join(venvDir, "bin", "python3")
	if _, err := os.Stat(pythonPath); err == nil {
		return true
	}
	fallbackPath := filepath.Join(venvDir, "bin", "python")
	_, err := os.Stat(fallbackPath)
	return err == nil
}
