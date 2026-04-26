package shub

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common/docker"
	"github.com/agentregistry-dev/agentregistry/internal/cli/common/gitutil"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
)

type DefaultSourceInstaller struct{}

func (DefaultSourceInstaller) Install(skill *models.SkillResponse, targetDir string) error {
	if skill == nil {
		return fmt.Errorf("skill metadata is nil")
	}

	var tarballRef string
	var dockerImage string
	for _, pkg := range skill.Skill.Packages {
		switch strings.ToLower(strings.TrimSpace(pkg.RegistryType)) {
		case "tarball", "archive":
			if tarballRef == "" {
				tarballRef = pkg.Identifier
			}
		case "docker", "oci":
			if dockerImage == "" {
				dockerImage = pkg.Identifier
			}
		}
	}
	if tarballRef != "" {
		return installFromTarball(tarballRef, targetDir)
	}
	if dockerImage != "" {
		return installFromDocker(dockerImage, targetDir)
	}
	if skill.Skill.Repository != nil && skill.Skill.Repository.Source == "git" {
		return installFromGit(skill.Skill.Repository.URL, targetDir)
	}
	return fmt.Errorf("skill has no supported package source")
}

func installFromGit(repoURL, targetDir string) error {
	return gitutil.CloneAndCopy(repoURL, targetDir, false)
}

func installFromTarball(ref, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	if localPath, ok, err := resolveLocalArchivePath(ref); err != nil {
		return err
	} else if ok {
		if err := shubskills.ExtractPackage(localPath, targetDir); err != nil {
			return fmt.Errorf("extract local SHUB package: %w", err)
		}
		return nil
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(ref) //nolint:gosec // package URL comes from trusted registry metadata configured by the user
	if err != nil {
		return fmt.Errorf("download SHUB package: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download SHUB package: unexpected status %d", resp.StatusCode)
	}
	if err := shubskills.ExtractPackageReader(resp.Body, targetDir); err != nil {
		return fmt.Errorf("extract downloaded SHUB package: %w", err)
	}
	return nil
}

func resolveLocalArchivePath(ref string) (string, bool, error) {
	if !strings.Contains(ref, "://") {
		absPath, err := filepath.Abs(ref)
		if err != nil {
			return "", false, fmt.Errorf("resolve local package path %q: %w", ref, err)
		}
		return absPath, true, nil
	}

	parsed, err := url.Parse(ref)
	if err != nil {
		return "", false, fmt.Errorf("parse package URL %q: %w", ref, err)
	}
	if parsed.Scheme != "file" {
		return "", false, nil
	}

	filePath := parsed.Path
	if filePath == "" {
		filePath = parsed.Opaque
	}
	if filePath == "" {
		return "", false, fmt.Errorf("invalid file package URL: %s", ref)
	}
	return filepath.FromSlash(filePath), true, nil
}

func installFromDocker(dockerImage, targetDir string) error {
	pullCmd := exec.Command("docker", "pull", dockerImage)
	if output, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pull Docker image: %w: %s", err, string(output))
	}

	createCmd := exec.Command("docker", "create", "--entrypoint", "/bin/sh", dockerImage, "-c", "echo")
	createOutput, err := createCmd.CombinedOutput()
	if err != nil {
		createCmd = exec.Command("docker", "create", dockerImage)
		createOutput, err = createCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("create container from image: %w: %s", err, string(createOutput))
		}
	}
	containerID := strings.TrimSpace(string(createOutput))
	defer func() {
		_ = exec.Command("docker", "rm", containerID).Run()
	}()

	tempDir, err := os.MkdirTemp("", "shub-extract-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	cpCmd := exec.Command("docker", "cp", containerID+":"+"/.", tempDir)
	if output, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract contents from container: %w: %s", err, string(output))
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}
	if err := docker.CopyNonEmptyContents(tempDir, targetDir); err != nil {
		return fmt.Errorf("copy extracted contents: %w", err)
	}
	return nil
}
