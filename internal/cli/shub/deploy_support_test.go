package shub

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
)

type fakePublisher struct {
	createdAsset *models.AssetPublishRequest
	createdSkill *models.SkillJSON
	assetErr     error
}

func (publisher *fakePublisher) CreateAsset(request *models.AssetPublishRequest) (*models.AssetResponse, error) {
	if publisher.assetErr != nil {
		return nil, publisher.assetErr
	}
	publisher.createdAsset = request
	asset, err := request.ToAsset()
	if err != nil {
		return nil, err
	}
	return &models.AssetResponse{Asset: *asset}, nil
}

func (publisher *fakePublisher) CreateSkill(skill *models.SkillJSON) (*models.SkillResponse, error) {
	publisher.createdSkill = skill
	return &models.SkillResponse{Skill: *skill}, nil
}

type fakeUploadingPublisher struct {
	fakePublisher
	uploadedAssetID string
	uploadedVersion string
	uploadedBytes   []byte
}

func (publisher *fakeUploadingPublisher) UploadAssetPackage(assetID, version string, content []byte, _ string) (*models.AssetPackageResponse, error) {
	publisher.uploadedAssetID = assetID
	publisher.uploadedVersion = version
	publisher.uploadedBytes = append([]byte(nil), content...)
	return &models.AssetPackageResponse{
		Package: models.AssetPackage{
			AssetID:     assetID,
			Version:     version,
			ContentType: "application/gzip",
			SizeBytes:   len(content),
		},
		DownloadURL: publisher.AssetPackageURL(assetID, version),
	}, nil
}

func (publisher *fakeUploadingPublisher) AssetPackageURL(assetID, version string) string {
	return "https://registry.example.test/v0/assets/" + url.PathEscape(assetID) + "/versions/" + url.PathEscape(version) + "/package"
}

func TestDeployAssetDryRunFromDirectory(t *testing.T) {
	dir := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo\n")

	result, err := DeployAsset(dir, nil, DeployOptions{
		PackageURL: "https://gitlab.example.com/packages/demo-skill-1.0.0.tar.gz",
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("DeployAsset() error = %v", err)
	}
	if result.Published {
		t.Fatal("Published = true, want false for dry-run")
	}
	if result.AssetPayload == nil {
		t.Fatal("AssetPayload is nil")
	}
	if got := result.Payload.Packages[0].RegistryType; got != "tarball" {
		t.Fatalf("RegistryType = %q, want %q", got, "tarball")
	}
	if result.PackageURL == "" {
		t.Fatal("PackageURL is empty")
	}
}

func TestDeployAssetPublishesArchiveViaAssetAPI(t *testing.T) {
	dir := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo\n")
	archivePath := filepath.Join(t.TempDir(), "demo-skill.tar.gz")
	if _, err := shubskills.BuildPackage(dir, archivePath); err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}

	publisher := &fakePublisher{}
	result, err := DeployAsset(archivePath, publisher, DeployOptions{})
	if err != nil {
		t.Fatalf("DeployAsset(archive) error = %v", err)
	}
	if !result.Published {
		t.Fatal("Published = false, want true")
	}
	if publisher.createdAsset == nil {
		t.Fatal("publisher did not receive asset payload")
	}
	if publisher.createdSkill != nil {
		t.Fatal("compatibility skill payload should not be used when asset API succeeds")
	}
	if publisher.createdAsset.Source == nil || !strings.HasPrefix(publisher.createdAsset.Source.PackageRef, "file://") {
		t.Fatalf("package ref = %#v, want file:// URL", publisher.createdAsset.Source)
	}
}

func TestDeployAssetFallsBackToSkillAPIWhenAssetEndpointMissing(t *testing.T) {
	dir := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo\n")
	archivePath := filepath.Join(t.TempDir(), "demo-skill.tar.gz")
	if _, err := shubskills.BuildPackage(dir, archivePath); err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}

	publisher := &fakePublisher{assetErr: fmt.Errorf("unexpected status: 404 Not Found")}
	result, err := DeployAsset(archivePath, publisher, DeployOptions{})
	if err != nil {
		t.Fatalf("DeployAsset(archive) error = %v", err)
	}
	if !result.Published {
		t.Fatal("Published = false, want true")
	}
	if publisher.createdSkill == nil {
		t.Fatal("fallback compatibility payload was not used")
	}
}

func TestDeployAssetUploadsArchiveWhenPublisherSupportsRegistryHostedPackages(t *testing.T) {
	dir := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo\n")
	archivePath := filepath.Join(t.TempDir(), "demo-skill.tar.gz")
	if _, err := shubskills.BuildPackage(dir, archivePath); err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	publisher := &fakeUploadingPublisher{}
	result, err := DeployAsset(archivePath, publisher, DeployOptions{})
	if err != nil {
		t.Fatalf("DeployAsset(archive) error = %v", err)
	}
	if publisher.uploadedAssetID != "local/demo-skill" {
		t.Fatalf("uploaded asset id = %q, want local/demo-skill", publisher.uploadedAssetID)
	}
	if publisher.createdAsset == nil || publisher.createdAsset.Source == nil {
		t.Fatalf("created asset source = %#v, want package source", publisher.createdAsset)
	}
	if publisher.createdAsset.Source.PackageRef != "https://registry.example.test/v0/assets/local%2Fdemo-skill/versions/1.0.0/package" {
		t.Fatalf("package ref = %q, want registry-hosted package URL", publisher.createdAsset.Source.PackageRef)
	}
	if string(publisher.uploadedBytes) != string(archiveBytes) {
		t.Fatal("uploaded bytes mismatch")
	}
	if result.PackageURL != publisher.createdAsset.Source.PackageRef {
		t.Fatalf("result package URL = %q, want %q", result.PackageURL, publisher.createdAsset.Source.PackageRef)
	}
}

func TestBuildDeployPayloadIncludesSHUBMetadata(t *testing.T) {
	dir := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo\n")
	asset, err := shubskills.LoadAssetDir(dir)
	if err != nil {
		t.Fatalf("LoadAssetDir() error = %v", err)
	}

	payload, err := buildDeployPayload(asset, DeployOptions{PackageURL: "https://example.com/demo-skill-1.0.0.tgz"})
	if err != nil {
		t.Fatalf("buildDeployPayload() error = %v", err)
	}
	if payload.SHUB == nil || payload.SHUB.Manifest == nil {
		t.Fatal("payload SHUB metadata is missing")
	}
	if payload.SHUB.AssetID != "local/demo-skill" {
		t.Fatalf("AssetID = %q, want %q", payload.SHUB.AssetID, "local/demo-skill")
	}
}

func TestDeployAssetRejectsBlockingAuditFindings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: demo-skill
description: Demo
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/demo-skill
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: none
  hooks:
    post_install:
      run: ["bash", "-c", "echo hi"]
---
# Demo
`), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "main.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write main.py: %v", err)
	}

	_, err := DeployAsset(dir, nil, DeployOptions{DryRun: true})
	if err == nil {
		t.Fatal("expected audit error, got nil")
	}
	if !strings.Contains(err.Error(), "inline shell execution") {
		t.Fatalf("error = %q, want inline shell audit failure", err.Error())
	}
}

func TestDeployAssetInfersGitRepositoryFromLocalCheckout(t *testing.T) {
	repoDir := t.TempDir()
	skillDir := filepath.Join(repoDir, "skills", "demo-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: demo-skill
description: Demo
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/demo-skill
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Demo
`), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	runGitCommand(t, repoDir, "init")
	runGitCommand(t, repoDir, "checkout", "-b", "main")
	runGitCommand(t, repoDir, "remote", "add", "origin", "git@github.com:acme/platform-skills.git")

	publisher := &fakePublisher{}
	result, err := DeployAsset(skillDir, publisher, DeployOptions{})
	if err != nil {
		t.Fatalf("DeployAsset() error = %v", err)
	}
	if !result.Published {
		t.Fatal("Published = false, want true")
	}
	if publisher.createdAsset == nil || publisher.createdAsset.Source == nil {
		t.Fatalf("created asset source = %#v, want inferred repository source", publisher.createdAsset)
	}
	wantRepository := "https://github.com/acme/platform-skills/tree/main/skills/demo-skill"
	if publisher.createdAsset.Source.RepositoryURL != wantRepository {
		t.Fatalf("RepositoryURL = %q, want %q", publisher.createdAsset.Source.RepositoryURL, wantRepository)
	}
	if result.Payload.Repository == nil || result.Payload.Repository.URL != wantRepository {
		t.Fatalf("payload repository = %#v, want %q", result.Payload.Repository, wantRepository)
	}
}

func TestDeployAssetInfersSelfHostedGitLabRepositoryFromLocalCheckout(t *testing.T) {
	repoDir := t.TempDir()
	skillDir := filepath.Join(repoDir, "skills", "demo-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: demo-skill
description: Demo
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/demo-skill
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Demo
`), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	runGitCommand(t, repoDir, "init")
	runGitCommand(t, repoDir, "checkout", "-b", "main")
	runGitCommand(t, repoDir, "remote", "add", "origin", "ssh://git@code.acme.internal:2222/platform/skills/demo-repo.git")

	publisher := &fakePublisher{}
	result, err := DeployAsset(skillDir, publisher, DeployOptions{})
	if err != nil {
		t.Fatalf("DeployAsset() error = %v", err)
	}
	if !result.Published {
		t.Fatal("Published = false, want true")
	}
	if publisher.createdAsset == nil || publisher.createdAsset.Source == nil {
		t.Fatalf("created asset source = %#v, want inferred repository source", publisher.createdAsset)
	}
	wantRepository := "https://code.acme.internal/platform/skills/demo-repo/-/tree/main/skills/demo-skill"
	if publisher.createdAsset.Source.RepositoryURL != wantRepository {
		t.Fatalf("RepositoryURL = %q, want %q", publisher.createdAsset.Source.RepositoryURL, wantRepository)
	}
	if result.Payload.Repository == nil || result.Payload.Repository.URL != wantRepository {
		t.Fatalf("payload repository = %#v, want %q", result.Payload.Repository, wantRepository)
	}
}

func TestDeployAssetInfersHintedSelfHostedGitLabRepositoryFromLocalCheckout(t *testing.T) {
	repoDir := t.TempDir()
	skillDir := filepath.Join(repoDir, "skills", "demo-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: demo-skill
description: Demo
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/demo-skill
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Demo
`), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	runGitCommand(t, repoDir, "init")
	runGitCommand(t, repoDir, "checkout", "-b", "main")
	runGitCommand(t, repoDir, "remote", "add", "origin", "git@code.acme.internal:team/demo-repo.git")

	publisher := &fakePublisher{}
	result, err := DeployAsset(skillDir, publisher, DeployOptions{GitProvider: "gitlab"})
	if err != nil {
		t.Fatalf("DeployAsset() error = %v", err)
	}
	if !result.Published {
		t.Fatal("Published = false, want true")
	}
	if publisher.createdAsset == nil || publisher.createdAsset.Source == nil {
		t.Fatalf("created asset source = %#v, want inferred repository source", publisher.createdAsset)
	}
	wantRepository := "https://code.acme.internal/team/demo-repo/-/tree/main/skills/demo-skill"
	if publisher.createdAsset.Source.RepositoryURL != wantRepository {
		t.Fatalf("RepositoryURL = %q, want %q", publisher.createdAsset.Source.RepositoryURL, wantRepository)
	}
	if result.Payload.Repository == nil || result.Payload.Repository.URL != wantRepository {
		t.Fatalf("payload repository = %#v, want %q", result.Payload.Repository, wantRepository)
	}
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
