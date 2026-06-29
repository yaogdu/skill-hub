package shub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
)

func TestManagerResolveWritesPinnedDependencyLockfile(t *testing.T) {
	root := createAgentFixtureWithDependencies(t, "1.0.0", `    prompts:
      - security/code-review@1.2.0
    mcps:
      - infra/k8s-readonly@0.8.3
`)
	prompt := mustLoadFixtureAsset(t, createPromptFixture(t, "1.2.0", "security/code-review"))
	prompt.Source = &models.AssetSource{PackageType: "tarball", PackageRef: "https://registry.example.com/prompt.tgz", Commit: "abc123"}
	mcp := mustLoadFixtureAsset(t, createMCPFixture(t, "0.8.3", "infra/k8s-readonly"))
	mcp.Source = &models.AssetSource{PackageType: "tarball", PackageRef: "https://registry.example.com/mcp.tgz", Commit: "def456"}

	registry := &assetBackedRegistry{
		assets: map[string]*models.AssetResponse{},
		assetVersions: map[string]map[string]*models.AssetResponse{
			"security/code-review": {
				"1.2.0": assetRegistryResponse(prompt, prompt.Source.PackageRef, time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)),
			},
			"infra/k8s-readonly": {
				"0.8.3": assetRegistryResponse(mcp, mcp.Source.PackageRef, time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)),
			},
		},
	}
	manager, err := NewManager(t.TempDir(), registry, DefaultSourceInstaller{}, "https://registry.example.com/v0")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	lock, err := manager.Resolve(root)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if lock.Asset.ID != "payments/risk-agent" {
		t.Fatalf("root asset = %q, want payments/risk-agent", lock.Asset.ID)
	}
	if len(lock.ResolvedAssets) != 2 {
		t.Fatalf("resolved assets = %d, want 2: %#v", len(lock.ResolvedAssets), lock.ResolvedAssets)
	}
	if lock.ResolvedAssets[0].ID != "infra/k8s-readonly" || lock.ResolvedAssets[0].Version != "0.8.3" {
		t.Fatalf("first resolved asset = %+v, want sorted infra/k8s-readonly@0.8.3", lock.ResolvedAssets[0])
	}
	if lock.ResolvedAssets[1].ID != "security/code-review" || lock.ResolvedAssets[1].SourceCommit != "abc123" {
		t.Fatalf("second resolved asset = %+v, want security/code-review with commit", lock.ResolvedAssets[1])
	}
}

func TestManagerResolveReturnsErrorForMissingDependency(t *testing.T) {
	root := createAgentFixtureWithDependencies(t, "1.0.0", `    prompts:
      - security/missing@1.0.0
`)
	registry := &assetBackedRegistry{assets: map[string]*models.AssetResponse{}, assetVersions: map[string]map[string]*models.AssetResponse{}}
	manager, err := NewManager(t.TempDir(), registry, DefaultSourceInstaller{}, "https://registry.example.com/v0")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = manager.Resolve(root)
	if err == nil {
		t.Fatal("expected missing dependency error, got nil")
	}
	if !strings.Contains(err.Error(), "dependency security/missing@1.0.0 not found") {
		t.Fatalf("error = %q, want missing dependency", err.Error())
	}
}

func TestManagerResolveReturnsErrorForCategoryMismatch(t *testing.T) {
	root := createAgentFixtureWithDependencies(t, "1.0.0", `    prompts:
      - security/code-review@1.2.0
`)
	mcp := mustLoadFixtureAsset(t, createMCPFixture(t, "1.2.0", "security/code-review"))
	registry := &assetBackedRegistry{
		assets: map[string]*models.AssetResponse{},
		assetVersions: map[string]map[string]*models.AssetResponse{
			"security/code-review": {
				"1.2.0": assetRegistryResponse(mcp, "https://registry.example.com/mcp.tgz", time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)),
			},
		},
	}
	manager, err := NewManager(t.TempDir(), registry, DefaultSourceInstaller{}, "https://registry.example.com/v0")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = manager.Resolve(root)
	if err == nil {
		t.Fatal("expected category mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "category mismatch") {
		t.Fatalf("error = %q, want category mismatch", err.Error())
	}
}

func TestCheckLockfileDetectsOutOfDateLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "shub.lock")
	current := &Lockfile{
		LockfileVersion: lockfileVersion,
		Asset:           LockfileRootAsset{ID: "payments/risk-agent", Version: "1.0.0", Category: models.AssetCategoryAgent},
		GeneratedAt:     time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		ResolvedAssets:  []ResolvedLockAsset{{ID: "security/code-review", Version: "1.2.0", Category: models.AssetCategoryPrompt}},
	}
	if err := writeLockfile(lockPath, current); err != nil {
		t.Fatalf("writeLockfile() error = %v", err)
	}
	next := *current
	next.GeneratedAt = time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	if err := checkLockfile(lockPath, &next); err != nil {
		t.Fatalf("checkLockfile() should ignore generatedAt changes: %v", err)
	}
	next.ResolvedAssets[0].Version = "1.3.0"
	err := checkLockfile(lockPath, &next)
	if err == nil {
		t.Fatal("expected out of date lockfile error, got nil")
	}
	if !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("error = %q, want out of date", err.Error())
	}
}

func createAgentFixtureWithDependencies(t *testing.T, version, dependenciesYAML string) string {
	t.Helper()
	dir := t.TempDir()
	writeResolveTestFile(t, filepath.Join(dir, "bin", "main.py"), "print('risk')\n")
	writeResolveTestSkillFile(t, dir, `---
name: risk-agent
description: Payment risk agent
version: `+version+`
shub:
  schemaVersion: shub.skill/v1alpha1
  id: payments/risk-agent
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: none
  dependencies:
`+dependenciesYAML+`---
# Risk Agent
`)
	return dir
}

func createPromptFixture(t *testing.T, version, id string) string {
	t.Helper()
	dir := t.TempDir()
	writeResolveTestSkillFile(t, dir, `---
name: code-review
description: Code review prompt
version: `+version+`
shub:
  schemaVersion: shub.skill/v1alpha1
  id: `+id+`
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Code Review
`)
	return dir
}

func createMCPFixture(t *testing.T, version, id string) string {
	t.Helper()
	dir := t.TempDir()
	writeResolveTestFile(t, filepath.Join(dir, "server.json"), `{"name":"k8s"}`)
	writeResolveTestSkillFile(t, dir, `---
name: k8s-readonly
description: Read-only Kubernetes MCP
version: `+version+`
shub:
  schemaVersion: shub.skill/v1alpha1
  id: `+id+`
  category: mcp
  entry:
    kind: mcp-config
    path: server.json
  runtime:
    type: none
---
# K8s Readonly
`)
	return dir
}

func mustLoadFixtureAsset(t *testing.T, dir string) *models.Asset {
	t.Helper()
	asset, err := shubskills.LoadAssetDir(dir)
	if err != nil {
		t.Fatalf("LoadAssetDir(%s) error = %v", dir, err)
	}
	return asset
}

func writeResolveTestSkillFile(t *testing.T, dir, content string) {
	t.Helper()
	writeResolveTestFile(t, filepath.Join(dir, "SKILL.md"), content)
}

func writeResolveTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
