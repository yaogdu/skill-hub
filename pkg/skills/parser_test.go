package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

func TestParseDir(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
		wantBody    string
	}{
		{
			name: "legacy compatible frontmatter",
			content: `---
name: helper-skill
description: Helpful skill
---
# Heading
body line
`,
			wantBody: "# Heading\nbody line\n",
		},
		{
			name:        "missing delimiters",
			content:     "just text",
			wantErr:     true,
			errContains: "missing YAML frontmatter",
		},
		{
			name:        "missing description",
			content:     "---\nname: helper\n---\nbody",
			wantErr:     true,
			errContains: "missing required field: description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, models.SkillFileName)
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write SKILL.md: %v", err)
			}

			document, err := ParseDir(dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDir() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errContains != "" && err != nil && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if document.Frontmatter.Name != "helper-skill" {
				t.Fatalf("Name = %q, want %q", document.Frontmatter.Name, "helper-skill")
			}
			if document.Body != tt.wantBody {
				t.Fatalf("Body = %q, want %q", document.Body, tt.wantBody)
			}
		})
	}
}

func TestSkillDocumentToAsset(t *testing.T) {
	document := models.SkillDocument{
		Path: models.SkillFileName,
		Body: "# Java Analyzer\n\nAnalyze services.\n",
		Frontmatter: models.SkillFrontmatter{
			Name:         "java-analyzer",
			Description:  "Analyze Java services.",
			Version:      "1.2.0",
			AllowedTools: []string{"Read", "Bash"},
			Shub: models.SkillFrontmatterShub{
				SchemaVersion: models.ShubSkillSchemaVersion,
				ID:            "arch/java-analyzer",
				Category:      models.AssetCategoryAgent,
				Entry: models.AssetEntry{
					Kind: "command",
					Path: "bin/main.py",
				},
				Runtime: models.AssetRuntime{
					Type:    "python",
					Version: ">=3.10",
					Install: &models.AssetInstall{Strategy: "uv", Path: "pyproject.toml", Lockfile: "uv.lock"},
				},
				Exports: []models.AssetExport{{Target: "codex", Mode: "prompt-file", Source: "SKILL.md"}},
			},
		},
	}

	asset, err := document.ToAsset()
	if err != nil {
		t.Fatalf("ToAsset() error = %v", err)
	}
	if asset.ID != "arch/java-analyzer" {
		t.Fatalf("ID = %q, want %q", asset.ID, "arch/java-analyzer")
	}
	if asset.Manifest.SchemaVersion != models.ShubAssetSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", asset.Manifest.SchemaVersion, models.ShubAssetSchemaVersion)
	}
	if asset.SourceSkill.Path != models.SkillFileName {
		t.Fatalf("SourceSkill.Path = %q, want %q", asset.SourceSkill.Path, models.SkillFileName)
	}
	if len(asset.AllowedTools) != 2 {
		t.Fatalf("AllowedTools length = %d, want 2", len(asset.AllowedTools))
	}
}

func TestSkillDocumentToAsset_Errors(t *testing.T) {
	tests := []struct {
		name        string
		document    models.SkillDocument
		errContains string
	}{
		{
			name: "missing version",
			document: models.SkillDocument{
				Frontmatter: models.SkillFrontmatter{
					Name:        "helper",
					Description: "helpful",
				},
			},
			errContains: "missing required field: version",
		},
		{
			name: "missing shub id",
			document: models.SkillDocument{
				Frontmatter: models.SkillFrontmatter{
					Name:        "helper",
					Description: "helpful",
					Version:     "1.0.0",
					Shub: models.SkillFrontmatterShub{
						SchemaVersion: models.ShubSkillSchemaVersion,
						Category:      models.AssetCategoryAgent,
						Entry:         models.AssetEntry{Kind: "command", Path: "bin/run"},
						Runtime:       models.AssetRuntime{Type: "python"},
					},
				},
			},
			errContains: "missing required field: shub.id",
		},
		{
			name: "prompt semantic mismatch",
			document: models.SkillDocument{
				Frontmatter: models.SkillFrontmatter{
					Name:        "helper",
					Description: "helpful",
					Version:     "1.0.0",
					Shub: models.SkillFrontmatterShub{
						SchemaVersion: models.ShubSkillSchemaVersion,
						ID:            "demo/helper",
						Category:      models.AssetCategoryPrompt,
						Entry:         models.AssetEntry{Kind: "command", Path: "bin/run"},
						Runtime:       models.AssetRuntime{Type: "python"},
					},
				},
			},
			errContains: "prompt assets must use entry.kind=skill-body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.document.ToAsset()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.errContains)
			}
		})
	}
}
