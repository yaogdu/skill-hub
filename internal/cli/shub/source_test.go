package shub

import (
	"net/http"
	"net/http/httptest"
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

func TestDefaultSourceInstallerAddsBearerForRegistryTarball(t *testing.T) {
	sourceDir := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo\n")
	archivePath := filepath.Join(t.TempDir(), "demo-skill.tar.gz")
	if _, err := shubskills.BuildPackage(sourceDir, archivePath); err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v0/assets/local%2Fdemo-skill/versions/1.0.0/package" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		if gotAuth != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.ServeFile(w, r, archivePath)
	}))
	defer server.Close()

	targetDir := t.TempDir()
	skill := &models.SkillResponse{Skill: models.SkillJSON{
		Name:        "demo-skill",
		Description: "Demo skill",
		Version:     "1.0.0",
		Packages: []models.SkillPackageInfo{{
			RegistryType: "tarball",
			Identifier:   server.URL + "/v0/assets/local%2Fdemo-skill/versions/1.0.0/package",
			Version:      "1.0.0",
		}},
	}}
	installer := DefaultSourceInstaller{BaseURL: server.URL + "/v0", Token: "test-token"}
	if err := installer.Install(skill, targetDir); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization header = %q, want bearer token", gotAuth)
	}
}

func TestDefaultSourceInstallerDoesNotAddBearerForExternalTarball(t *testing.T) {
	if token := (DefaultSourceInstaller{
		BaseURL: "https://registry.example.com/v0",
		Token:   "test-token",
	}).tokenForPackageURL("https://github.com/acme/demo/archive/main.tar.gz"); token != "" {
		t.Fatalf("tokenForPackageURL returned %q for external tarball, want empty", token)
	}
}
