package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAndExtractPackage(t *testing.T) {
	dir := t.TempDir()
	writePackageFixtureFile(t, filepath.Join(dir, "SKILL.md"), `---
name: helper-skill
description: Helpful prompt skill
version: 1.2.3
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/helper-skill
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Helper Skill
`)
	writePackageFixtureFile(t, filepath.Join(dir, "docs", "notes.md"), "hello from docs\n")

	archivePath := filepath.Join(t.TempDir(), "helper-skill.tar.gz")
	result, err := BuildPackage(dir, archivePath)
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	if result.Asset.ID != "local/helper-skill" {
		t.Fatalf("Asset.ID = %q, want %q", result.Asset.ID, "local/helper-skill")
	}
	if result.SHA256 == "" {
		t.Fatal("SHA256 is empty")
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
	joined := strings.Join(result.Files, "\n")
	for _, expected := range []string{"SKILL.md", "docs/notes.md", DerivedManifestPath, PackageMetadataPath} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("package files = %q, want it to contain %q", joined, expected)
		}
	}

	extractDir := t.TempDir()
	if err := ExtractPackage(archivePath, extractDir); err != nil {
		t.Fatalf("ExtractPackage() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(extractDir, DerivedManifestPath)); err != nil {
		t.Fatalf("derived manifest missing after extract: %v", err)
	}
	asset, err := LoadAssetDir(extractDir)
	if err != nil {
		t.Fatalf("LoadAssetDir(extracted) error = %v", err)
	}
	if asset.Version != "1.2.3" {
		t.Fatalf("Version = %q, want %q", asset.Version, "1.2.3")
	}
}

func TestBuildPackageSkipsDistDirectory(t *testing.T) {
	dir := t.TempDir()
	writePackageFixtureFile(t, filepath.Join(dir, "SKILL.md"), `---
name: helper-skill
description: Helpful prompt skill
version: 1.2.3
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/helper-skill
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Helper Skill
`)
	writePackageFixtureFile(t, filepath.Join(dir, "dist", "helper-skill-1.2.3.tar.gz"), "old archive\n")
	writePackageFixtureFile(t, filepath.Join(dir, "dist", "notes.txt"), "generated note\n")

	archivePath := filepath.Join(dir, "dist", "helper-skill-1.2.3.tar.gz")
	result, err := BuildPackage(dir, archivePath)
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	for _, file := range result.Files {
		if strings.HasPrefix(file, "dist/") {
			t.Fatalf("package files = %#v, want dist files skipped", result.Files)
		}
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
}

func TestExamplePackagesValidateAndBuild(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples", "shub")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", examplesDir, err)
	}

	found := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		exampleDir := filepath.Join(examplesDir, entry.Name())
		if _, err := os.Stat(filepath.Join(exampleDir, "SKILL.md")); err != nil {
			continue
		}

		found++
		t.Run(entry.Name(), func(t *testing.T) {
			result, err := ValidateDir(exampleDir)
			if err != nil {
				t.Fatalf("ValidateDir(%s) error = %v", exampleDir, err)
			}
			if result.Asset == nil || result.Asset.ID == "" {
				t.Fatalf("ValidateDir(%s) returned empty asset", exampleDir)
			}

			archivePath := filepath.Join(t.TempDir(), entry.Name()+".tar.gz")
			buildResult, err := BuildPackage(exampleDir, archivePath)
			if err != nil {
				t.Fatalf("BuildPackage(%s) error = %v", exampleDir, err)
			}
			if buildResult.Asset == nil || buildResult.Asset.ID == "" {
				t.Fatalf("BuildPackage(%s) returned empty asset", exampleDir)
			}
			if _, err := os.Stat(archivePath); err != nil {
				t.Fatalf("archive %s missing: %v", archivePath, err)
			}
		})
	}
	if found == 0 {
		t.Fatal("no SHUB example packages found")
	}
}

func writePackageFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}
