package shub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	yaml "gopkg.in/yaml.v3"
)

type fakeRegistry struct {
	latest   map[string]*models.SkillResponse
	versions map[string]map[string]*models.SkillResponse
	list     []*models.SkillResponse
}

func (registry *fakeRegistry) GetSkill(name string) (*models.SkillResponse, error) {
	return registry.latest[name], nil
}

func (registry *fakeRegistry) GetSkillVersion(name, version string) (*models.SkillResponse, error) {
	if registry.versions[name] == nil {
		return nil, nil
	}
	return registry.versions[name][version], nil
}

func (registry *fakeRegistry) GetSkillVersions(name string) ([]*models.SkillResponse, error) {
	entries := registry.versions[name]
	result := make([]*models.SkillResponse, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

func (registry *fakeRegistry) GetSkills() ([]*models.SkillResponse, error) {
	if registry.list != nil {
		return registry.list, nil
	}
	result := make([]*models.SkillResponse, 0, len(registry.latest))
	for _, entry := range registry.latest {
		result = append(result, entry)
	}
	return result, nil
}

type fakeInstaller struct {
	sources map[string]string
}

func (installer fakeInstaller) Install(skill *models.SkillResponse, targetDir string) error {
	source := installer.sources[skill.Skill.Version]
	if source == "" {
		return fmt.Errorf("no fixture for version %s", skill.Skill.Version)
	}
	return copyDir(source, targetDir)
}

func TestManagerAddUseDoctor(t *testing.T) {
	homeDir := t.TempDir()
	fixtureV1 := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo v1\n")
	fixtureV2 := createSkillFixture(t, "1.1.0", "local/demo-skill", "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	resultV1, err := manager.Add("demo-skill", "1.0.0")
	if err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
	if resultV1.Asset.ID != "local/demo-skill" {
		t.Fatalf("Asset.ID = %q, want %q", resultV1.Asset.ID, "local/demo-skill")
	}
	if _, err := os.Stat(filepath.Join(homeDir, "config.json")); err != nil {
		t.Fatalf("config.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "state.json")); err != nil {
		t.Fatalf("state.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".lock")); err != nil {
		t.Fatalf(".lock missing: %v", err)
	}

	_, err = manager.Add("demo-skill", "1.1.0")
	if err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v2")

	installed, err := manager.Use("local/demo-skill@1.0.0", "")
	if err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if installed.Version != "1.0.0" {
		t.Fatalf("installed.Version = %q, want 1.0.0", installed.Version)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")

	if err := os.RemoveAll(installed.EnvDir); err != nil {
		t.Fatalf("remove env dir: %v", err)
	}
	if err := os.Remove(filepath.Join(homeDir, "exports", "local-demo-skill.md")); err != nil {
		t.Fatalf("remove export file: %v", err)
	}
	doctorResult, err := manager.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if doctorResult.Checked < 2 {
		t.Fatalf("doctor checked %d, want >= 2", doctorResult.Checked)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
	if _, err := os.Stat(installed.EnvDir); err != nil {
		t.Fatalf("env dir not repaired: %v", err)
	}

	state := readState(t, filepath.Join(homeDir, "state.json"))
	if state.Active["local/demo-skill"] != "1.0.0" {
		t.Fatalf("active version = %q, want 1.0.0", state.Active["local/demo-skill"])
	}
	installedState := findInstalledAsset(state, "local/demo-skill", "1.0.0")
	if installedState == nil {
		t.Fatal("installed state missing local/demo-skill@1.0.0")
	}
	if installedState.Runtime.Status != "ready" {
		t.Fatalf("runtime status = %q, want ready", installedState.Runtime.Status)
	}
	if len(installedState.Exports) == 0 || installedState.Exports[0].Status != "ready" {
		t.Fatalf("export state = %#v, want ready export metadata", installedState.Exports)
	}
}

func TestManagerSyncKeepsActiveVersion(t *testing.T) {
	homeDir := t.TempDir()
	fixtureV1 := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo v1\n")
	fixtureV2 := createSkillFixture(t, "1.1.0", "local/demo-skill", "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	stateBefore := readState(t, filepath.Join(homeDir, "state.json"))
	if stateBefore.Active["local/demo-skill"] != "1.0.0" {
		t.Fatalf("active before sync = %q, want 1.0.0", stateBefore.Active["local/demo-skill"])
	}

	result, err := manager.Sync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Installed != 1 {
		t.Fatalf("Installed = %d, want 1", result.Installed)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
	stateAfter := readState(t, filepath.Join(homeDir, "state.json"))
	if stateAfter.Active["local/demo-skill"] != "1.0.0" {
		t.Fatalf("active after sync = %q, want 1.0.0", stateAfter.Active["local/demo-skill"])
	}
	if stateAfter.Sync.Cursor == "" {
		t.Fatal("sync cursor should be persisted after sync")
	}
	if findInstalledByRegistryName(stateAfter, "demo-skill", "1.1.0") == nil {
		t.Fatal("latest version was not installed during sync")
	}
}

func TestManagerSearchFallsBackToLocalState(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo v1\n")
	registry := &fakeRegistry{}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixture}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	registry.versions = map[string]map[string]*models.SkillResponse{"demo-skill": {"1.0.0": skillResponse("demo-skill", "1.0.0")}}
	registry.latest = map[string]*models.SkillResponse{"demo-skill": skillResponse("demo-skill", "1.0.0")}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	state := readState(t, filepath.Join(homeDir, "state.json"))
	installed := findInstalledAsset(state, "local/demo-skill", "1.0.0")
	if installed == nil {
		t.Fatal("installed asset missing from state")
	}
	if installed.Description != "Demo skill" {
		t.Fatalf("Description = %q, want %q", installed.Description, "Demo skill")
	}
	if !strings.Contains(installed.SearchText, "Demo skill") {
		t.Fatalf("SearchText = %q, want it to include description", installed.SearchText)
	}
	if err := os.RemoveAll(installed.InstallDir); err != nil {
		t.Fatalf("remove install dir: %v", err)
	}

	results, err := manager.Search("demo", true)
	if err != nil {
		t.Fatalf("Search(local) error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].AssetID != "local/demo-skill" {
		t.Fatalf("AssetID = %q, want %q", results[0].AssetID, "local/demo-skill")
	}
	if results[0].Description != "Demo skill" {
		t.Fatalf("Description = %q, want %q", results[0].Description, "Demo skill")
	}
}

func TestManagerSearchUsesRegistryWhenAvailable(t *testing.T) {
	manager, err := NewManager(t.TempDir(), &fakeRegistry{list: []*models.SkillResponse{
		skillResponseWithDescription("demo-skill", "1.0.0", "Demo description"),
		skillResponseWithDescription("other-skill", "1.0.0", "Other"),
	}}, fakeInstaller{}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	results, err := manager.Search("demo", false)
	if err != nil {
		t.Fatalf("Search(registry) error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].RegistryName != "demo-skill" {
		t.Fatalf("RegistryName = %q, want %q", results[0].RegistryName, "demo-skill")
	}
}

func TestManagerDoctorReconcilesMissingActiveVersionAndRebuildsExports(t *testing.T) {
	homeDir := t.TempDir()
	fixtureV1 := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo v1\n")
	fixtureV2 := createSkillFixture(t, "1.1.0", "local/demo-skill", "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}

	statePath := filepath.Join(homeDir, "state.json")
	state := readState(t, statePath)
	state.Active["local/demo-skill"] = "9.9.9"
	if err := writeJSONFile(statePath, state); err != nil {
		t.Fatalf("write corrupted state: %v", err)
	}
	if err := os.Remove(filepath.Join(homeDir, "exports", "local-demo-skill.md")); err != nil {
		t.Fatalf("remove export file: %v", err)
	}

	result, err := manager.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if result.Repaired == 0 {
		t.Fatal("Doctor() should report repaired state")
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v2")

	repaired := readState(t, statePath)
	if repaired.Active["local/demo-skill"] != "1.1.0" {
		t.Fatalf("active version = %q, want %q", repaired.Active["local/demo-skill"], "1.1.0")
	}
}

func TestManagerDoctorNoopReportsZeroRepairs(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo v1\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.0.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
			},
		},
	}
	manager, err := NewManager(homeDir, registry, fakeInstaller{sources: map[string]string{"1.0.0": fixture}}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	result, err := manager.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if result.Repaired != 0 {
		t.Fatalf("Doctor() repaired = %d, want 0 for no-op check", result.Repaired)
	}
}

func TestManagerDoctorCountsMissingPromptExportRepair(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "local/demo-skill", "# Demo v1\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.0.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
			},
		},
	}
	manager, err := NewManager(homeDir, registry, fakeInstaller{sources: map[string]string{"1.0.0": fixture}}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	exportPath := filepath.Join(homeDir, "exports", "local-demo-skill.md")
	if err := os.Remove(exportPath); err != nil {
		t.Fatalf("remove export: %v", err)
	}

	result, err := manager.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if result.Repaired == 0 {
		t.Fatal("Doctor() should count rebuilt prompt export as a repair")
	}
	assertFileContains(t, exportPath, "# Demo v1")
}

func skillResponse(name, version string) *models.SkillResponse {
	return skillResponseWithDescription(name, version, "")
}

func skillResponseWithDescription(name, version, description string) *models.SkillResponse {
	return &models.SkillResponse{Skill: models.SkillJSON{Name: name, Version: version, Description: description}}
}

func createSkillFixture(t *testing.T, version, assetID, body string) string {
	t.Helper()
	dir := t.TempDir()
	content := fmt.Sprintf(`---
name: demo-skill
description: Demo skill
version: %s
shub:
  schemaVersion: shub.skill/v1alpha1
  id: %s
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
%s`, version, assetID, body)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return dir
}

func assertFileContains(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	if !contains(string(data), expected) {
		t.Fatalf("file %s = %q, want it to contain %q", path, string(data), expected)
	}
}

func readState(t *testing.T, path string) *State {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	return &state
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func contains(got, want string) bool {
	return strings.Contains(got, want)
}

func TestManagerUseSwitchesTargetedExportsAndCleansStalePaths(t *testing.T) {
	homeDir := t.TempDir()
	fixtureV1 := createSkillFixtureWithExports(t, "1.0.0", `
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
      targetPath: codex/demo.md
`, "# Demo v1\n")
	fixtureV2 := createSkillFixtureWithExports(t, "1.1.0", `
  exports:
    - target: skills
      mode: skill-dir
      source: .
      targetPath: skills/demo-skill
`, "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "codex", "demo.md"), "# Demo v1")

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "codex", "demo.md")); !os.IsNotExist(err) {
		t.Fatalf("old prompt export should be removed, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "skills", "demo-skill", "SKILL.md"), "# Demo v2")
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "skills", "demo-skill", ".shub")); !os.IsNotExist(err) {
		t.Fatalf(".shub directory should not be exported, stat err = %v", err)
	}

	if _, err := manager.Use("local/demo-skill@1.0.0", ""); err != nil {
		t.Fatalf("Use(v1) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "skills", "demo-skill")); !os.IsNotExist(err) {
		t.Fatalf("skill-dir export should be removed after switching back, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "codex", "demo.md"), "# Demo v1")
}

func TestManagerExportsNativeCodexSkillDir(t *testing.T) {
	homeDir := t.TempDir()
	codexSkillsDir := filepath.Join(t.TempDir(), "codex-skills")
	t.Setenv("SHUB_CODEX_SKILLS_DIR", codexSkillsDir)

	fixtureV1 := createSkillFixtureWithExports(t, "1.0.0", `
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
`, "# Demo v1\n")
	fixtureV2 := createSkillFixtureWithExports(t, "1.1.0", `
  exports:
    - target: codex
      mode: skill-dir
      source: .
`, "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("prompt export should be removed after switching to native codex skill-dir, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(codexSkillsDir, "local-demo-skill", "SKILL.md"), "# Demo v2")

	if _, err := manager.Use("local/demo-skill@1.0.0", ""); err != nil {
		t.Fatalf("Use(v1) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexSkillsDir, "local-demo-skill")); !os.IsNotExist(err) {
		t.Fatalf("native codex skill-dir export should be removed after switching back, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
}

func TestManagerExportsNativeCursorRules(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("SHUB_CURSOR_WORKSPACE_DIR", workspaceDir)

	fixtureV1 := createSkillFixtureWithExports(t, "1.0.0", `
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
`, "# Demo v1\n")
	fixtureV2 := createSkillFixtureWithExports(t, "1.1.0", `
  exports:
    - target: cursor
      mode: rules-file
      source: SKILL.md
`, "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("prompt export should be removed after switching to native cursor rules export, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(workspaceDir, ".cursor", "rules", "local-demo-skill.mdc"), "# Demo v2")

	if _, err := manager.Use("local/demo-skill@1.0.0", ""); err != nil {
		t.Fatalf("Use(v1) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, ".cursor", "rules", "local-demo-skill.mdc")); !os.IsNotExist(err) {
		t.Fatalf("native cursor rules export should be removed after switching back, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
}

func TestManagerExportsNativeClaudeCommands(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("SHUB_CLAUDE_WORKSPACE_DIR", workspaceDir)

	fixtureV1 := createSkillFixtureWithExports(t, "1.0.0", `
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
`, "# Demo v1\n")
	fixtureV2 := createSkillFixtureWithExports(t, "1.1.0", `
  exports:
    - target: claude-code
      mode: prompt-file
      source: SKILL.md
`, "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("prompt export should be removed after switching to native claude command export, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(workspaceDir, ".claude", "commands", "local-demo-skill.md"), "# Demo v2")

	if _, err := manager.Use("local/demo-skill@1.0.0", ""); err != nil {
		t.Fatalf("Use(v1) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, ".claude", "commands", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("native claude command export should be removed after switching back, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
}

func TestManagerExportsNativeClaudeSkills(t *testing.T) {
	homeDir := t.TempDir()
	claudeSkillsDir := filepath.Join(t.TempDir(), "claude-skills")
	t.Setenv("SHUB_CLAUDE_SKILLS_DIR", claudeSkillsDir)

	fixtureV1 := createSkillFixtureWithExports(t, "1.0.0", `
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
`, "# Demo v1\n")
	fixtureV2 := createSkillFixtureWithExports(t, "1.1.0", `
  exports:
    - target: claude-code
      mode: skill-dir
      source: .
`, "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("prompt export should be removed after switching to native claude skill-dir export, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(claudeSkillsDir, "local-demo-skill", "SKILL.md"), "# Demo v2")

	if _, err := manager.Use("local/demo-skill@1.0.0", ""); err != nil {
		t.Fatalf("Use(v1) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(claudeSkillsDir, "local-demo-skill")); !os.IsNotExist(err) {
		t.Fatalf("native claude skill-dir export should be removed after switching back, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
}

func TestManagerExportsNativeClaudeMCPConfig(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("SHUB_CLAUDE_WORKSPACE_DIR", workspaceDir)

	configPath := filepath.Join(workspaceDir, ".mcp.json")
	settingsPath := filepath.Join(workspaceDir, ".claude", "settings.local.json")
	if err := os.WriteFile(configPath, []byte("{\n  \"mcpServers\": {\n    \"user-server\": {\"type\": \"http\", \"url\": \"https://user.example.com/mcp\"}\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("seed .mcp.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{\n  \"enabledMcpjsonServers\": [\"user-approved\"],\n  \"permissions\": {\n    \"allow\": [\"Bash(git status)\"]\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("seed settings.local.json: %v", err)
	}

	fixtureV1 := createSkillFixtureWithExports(t, "1.0.0", `
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
`, "# Demo v1\n")
	fixtureV2 := createSkillFixtureWithFiles(t, "demo-skill", "local/demo-skill", "1.1.0", `
  exports:
    - target: claude-code
      mode: mcp-config
      source: server.json
`, "# Demo v2\n", map[string]string{
		"server.json": `{"mcpServers":{"weather":{"type":"http","url":"https://weather.example.com/mcp"}}}`,
	})

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("prompt export should be removed after switching to native claude mcp export, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(workspaceDir, ".claude", "mcp", "shub", "local-demo-skill.json"), `"weather"`)
	assertClaudeMCPServers(t, configPath, map[string]string{
		"user-server":                     "https://user.example.com/mcp",
		"shub__local-demo-skill__weather": "https://weather.example.com/mcp",
	})
	assertClaudeSettings(t, settingsPath,
		[]string{"shub__local-demo-skill__weather", "user-approved"},
		[]string{"Bash(git status)", "mcp__shub__local-demo-skill__weather"},
	)

	if _, err := manager.Use("local/demo-skill@1.0.0", ""); err != nil {
		t.Fatalf("Use(v1) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, ".claude", "mcp", "shub", "local-demo-skill.json")); !os.IsNotExist(err) {
		t.Fatalf("native claude mcp export should be removed after switching back, stat err = %v", err)
	}
	assertClaudeMCPServers(t, configPath, map[string]string{
		"user-server": "https://user.example.com/mcp",
	})
	assertClaudeSettings(t, settingsPath,
		[]string{"user-approved"},
		[]string{"Bash(git status)"},
	)
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
}

func TestManagerClaudeMCPConfigMergesMultipleAssets(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("SHUB_CLAUDE_WORKSPACE_DIR", workspaceDir)

	fixtureDemo := createSkillFixtureWithFiles(t, "demo-skill", "local/demo-skill", "1.1.0", `
  exports:
    - target: claude-code
      mode: mcp-config
      source: server.json
`, "# Demo weather\n", map[string]string{
		"server.json": `{"mcpServers":{"weather":{"type":"http","url":"https://weather.example.com/mcp"}}}`,
	})
	fixtureDocs := createSkillFixtureWithFiles(t, "docs-skill", "local/docs-skill", "2.1.0", `
  exports:
    - target: claude-code
      mode: mcp-config
      source: server.json
`, "# Demo docs\n", map[string]string{
		"server.json": `{"mcpServers":{"docs":{"type":"http","url":"https://docs.example.com/mcp"}}}`,
	})

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
			"docs-skill": skillResponse("docs-skill", "2.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
			"docs-skill": {
				"2.1.0": skillResponse("docs-skill", "2.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{
		"1.1.0": fixtureDemo,
		"2.1.0": fixtureDocs,
	}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(demo) error = %v", err)
	}
	if _, err := manager.Add("docs-skill", "2.1.0"); err != nil {
		t.Fatalf("Add(docs) error = %v", err)
	}

	assertClaudeMCPServers(t, filepath.Join(workspaceDir, ".mcp.json"), map[string]string{
		"shub__local-demo-skill__weather": "https://weather.example.com/mcp",
		"shub__local-docs-skill__docs":    "https://docs.example.com/mcp",
	})
	assertClaudeSettings(t, filepath.Join(workspaceDir, ".claude", "settings.local.json"),
		[]string{"shub__local-demo-skill__weather", "shub__local-docs-skill__docs"},
		[]string{"mcp__shub__local-demo-skill__weather", "mcp__shub__local-docs-skill__docs"},
	)
}

func TestManagerDoctorRestoresClaudeManagedConfigs(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("SHUB_CLAUDE_WORKSPACE_DIR", workspaceDir)

	fixture := createSkillFixtureWithFiles(t, "demo-skill", "local/demo-skill", "1.1.0", `
  exports:
    - target: claude-code
      mode: mcp-config
      source: server.json
`, "# Demo weather\n", map[string]string{
		"server.json": `{"mcpServers":{"weather":{"type":"http","url":"https://weather.example.com/mcp"}}}`,
	})

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	manager, err := NewManager(homeDir, registry, fakeInstaller{sources: map[string]string{"1.1.0": fixture}}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	configPath := filepath.Join(workspaceDir, ".mcp.json")
	settingsPath := filepath.Join(workspaceDir, ".claude", "settings.local.json")
	exportPath := filepath.Join(workspaceDir, ".claude", "mcp", "shub", "local-demo-skill.json")
	for _, path := range []string{configPath, settingsPath, exportPath} {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}

	result, err := manager.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if result.Repaired == 0 {
		t.Fatal("Doctor() should count restored managed Claude config state as repairs")
	}
	assertClaudeMCPServers(t, configPath, map[string]string{
		"shub__local-demo-skill__weather": "https://weather.example.com/mcp",
	})
	assertClaudeSettings(t, settingsPath,
		[]string{"shub__local-demo-skill__weather"},
		[]string{"mcp__shub__local-demo-skill__weather"},
	)
	assertFileContains(t, exportPath, `"weather"`)
}

func TestManagerExportsNativeAiderRules(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("SHUB_AIDER_WORKSPACE_DIR", workspaceDir)

	fixtureV1 := createSkillFixtureWithExports(t, "1.0.0", `
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
`, "# Demo v1\n")
	fixtureV2 := createSkillFixtureWithExports(t, "1.1.0", `
  exports:
    - target: aider
      mode: rules-file
      source: SKILL.md
`, "# Demo v2\n")

	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.0.0": skillResponse("demo-skill", "1.0.0"),
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	installer := fakeInstaller{sources: map[string]string{"1.0.0": fixtureV1, "1.1.0": fixtureV2}}
	manager, err := NewManager(homeDir, registry, installer, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Add("demo-skill", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")

	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add(v2) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "exports", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("prompt export should be removed after switching to native aider rules export, stat err = %v", err)
	}
	assertFileContains(t, filepath.Join(workspaceDir, ".aider", "shub", "local-demo-skill.md"), "# Demo v2")
	assertAiderConfigReads(t, filepath.Join(workspaceDir, ".aider.conf.yml"), ".aider/shub/local-demo-skill.md")

	if _, err := manager.Use("local/demo-skill@1.0.0", ""); err != nil {
		t.Fatalf("Use(v1) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, ".aider", "shub", "local-demo-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("native aider rules export should be removed after switching back, stat err = %v", err)
	}
	assertAiderConfigReads(t, filepath.Join(workspaceDir, ".aider.conf.yml"))
	assertFileContains(t, filepath.Join(homeDir, "exports", "local-demo-skill.md"), "# Demo v1")
}

func TestManagerDoctorRestoresAiderManagedConfig(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("SHUB_AIDER_WORKSPACE_DIR", workspaceDir)

	fixture := createSkillFixtureWithExports(t, "1.1.0", `
  exports:
    - target: aider
      mode: rules-file
      source: SKILL.md
`, "# Demo v2\n")
	registry := &fakeRegistry{
		latest: map[string]*models.SkillResponse{
			"demo-skill": skillResponse("demo-skill", "1.1.0"),
		},
		versions: map[string]map[string]*models.SkillResponse{
			"demo-skill": {
				"1.1.0": skillResponse("demo-skill", "1.1.0"),
			},
		},
	}
	manager, err := NewManager(homeDir, registry, fakeInstaller{sources: map[string]string{"1.1.0": fixture}}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if _, err := manager.Add("demo-skill", "1.1.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	configPath := filepath.Join(workspaceDir, ".aider.conf.yml")
	exportPath := filepath.Join(workspaceDir, ".aider", "shub", "local-demo-skill.md")
	for _, path := range []string{configPath, exportPath} {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}

	result, err := manager.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if result.Repaired == 0 {
		t.Fatal("Doctor() should count restored managed Aider config state as repairs")
	}
	assertAiderConfigReads(t, configPath, ".aider/shub/local-demo-skill.md")
	assertFileContains(t, exportPath, "# Demo v2")
}

func assertAiderConfigReads(t *testing.T, path string, want ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if len(want) == 0 && os.IsNotExist(err) {
			return
		}
		t.Fatalf("read aider config %s: %v", path, err)
	}

	var config struct {
		Read []string `yaml:"read"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse aider config %s: %v", path, err)
	}
	if len(config.Read) != len(want) {
		t.Fatalf("aider config reads = %#v, want %#v", config.Read, want)
	}
	for index, entry := range want {
		if config.Read[index] != entry {
			t.Fatalf("aider config read[%d] = %q, want %q", index, config.Read[index], entry)
		}
	}
}

func assertClaudeMCPServers(t *testing.T, path string, want map[string]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read claude mcp config %s: %v", path, err)
	}

	var config struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse claude mcp config %s: %v", path, err)
	}
	if len(config.MCPServers) != len(want) {
		t.Fatalf("claude mcp servers = %#v, want %#v", config.MCPServers, want)
	}
	for name, url := range want {
		server, ok := config.MCPServers[name]
		if !ok {
			t.Fatalf("missing claude mcp server %q in %#v", name, config.MCPServers)
		}
		if server.URL != url {
			t.Fatalf("claude mcp server %q url = %q, want %q", name, server.URL, url)
		}
	}
}

func assertClaudeSettings(t *testing.T, path string, wantEnabled, wantAllow []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read claude settings %s: %v", path, err)
	}

	var config struct {
		EnabledMCPJSONServers []string `json:"enabledMcpjsonServers"`
		Permissions           struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse claude settings %s: %v", path, err)
	}
	if strings.Join(config.EnabledMCPJSONServers, "|") != strings.Join(wantEnabled, "|") {
		t.Fatalf("enabledMcpjsonServers = %#v, want %#v", config.EnabledMCPJSONServers, wantEnabled)
	}
	if strings.Join(config.Permissions.Allow, "|") != strings.Join(wantAllow, "|") {
		t.Fatalf("permissions.allow = %#v, want %#v", config.Permissions.Allow, wantAllow)
	}
}

func createSkillFixtureWithExports(t *testing.T, version, exportsYAML, body string) string {
	return createSkillFixtureWithFiles(t, "demo-skill", "local/demo-skill", version, exportsYAML, body, nil)
}

func createSkillFixtureWithFiles(t *testing.T, name, assetID, version, exportsYAML, body string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	content := fmt.Sprintf(`---
name: %s
description: Demo skill
version: %s
shub:
  schemaVersion: shub.skill/v1alpha1
  id: %s
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none%s
---
%s`, name, version, assetID, exportsYAML, body)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	for relativePath, fileBody := range files {
		path := filepath.Join(dir, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture dir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(fileBody), 0o644); err != nil {
			t.Fatalf("write fixture file %s: %v", relativePath, err)
		}
	}
	return dir
}
