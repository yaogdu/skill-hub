package shub

import (
	"path/filepath"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
)

func TestDefaultSourceInstaller_InstallsFromTarball(t *testing.T) {
	sourceDir := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo\n")
	archivePath := filepath.Join(t.TempDir(), "demo-skill.tar.gz")
	if _, err := shubskills.BuildPackage(sourceDir, archivePath); err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}

	targetDir := t.TempDir()
	skill := &models.SkillResponse{Skill: models.SkillJSON{
		Name:        "demo-skill",
		Description: "Demo skill",
		Version:     "1.0.0",
		Packages: []models.SkillPackageInfo{{
			RegistryType: "tarball",
			Identifier:   fileURL(archivePath),
			Version:      "1.0.0",
		}},
	}}
	installer := DefaultSourceInstaller{}
	if err := installer.Install(skill, targetDir); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	asset, err := shubskills.LoadAssetDir(targetDir)
	if err != nil {
		t.Fatalf("LoadAssetDir(installed) error = %v", err)
	}
	if asset.ID != "local/demo-skill" {
		t.Fatalf("Asset.ID = %q, want %q", asset.ID, "local/demo-skill")
	}
}
