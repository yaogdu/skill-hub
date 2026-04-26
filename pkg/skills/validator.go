package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

type ValidationResult struct {
	Asset *models.Asset `json:"asset"`
	Audit *AuditReport  `json:"audit,omitempty"`
}

func ValidateDir(dir string) (*ValidationResult, error) {
	asset, err := LoadAssetDir(dir)
	if err != nil {
		return nil, err
	}

	report, err := auditLoadedAsset(dir, asset)
	if err != nil {
		return nil, err
	}

	result := &ValidationResult{Asset: asset, Audit: report}
	if blocking := report.BlockingError(); blocking != nil {
		return result, blocking
	}
	return result, nil
}

func LoadAssetDir(dir string) (*models.Asset, error) {
	document, err := ParseDir(dir)
	if err != nil {
		return nil, err
	}

	asset, err := document.ToAsset()
	if err != nil {
		return nil, err
	}

	if err := validatePackageFiles(dir, asset); err != nil {
		return nil, err
	}

	return asset, nil
}

func validatePackageFiles(baseDir string, asset *models.Asset) error {
	if asset == nil {
		return fmt.Errorf("asset is nil")
	}

	if err := validateEntry(baseDir, asset.Manifest.Entry); err != nil {
		return err
	}
	if err := validateRuntime(baseDir, asset.Manifest.Runtime); err != nil {
		return err
	}
	if err := validateExports(baseDir, asset.Manifest.Exports); err != nil {
		return err
	}

	return nil
}

func validateEntry(baseDir string, entry models.AssetEntry) error {
	if entry.Kind == "skill-body" {
		if entry.Path != models.SkillFileName {
			return fmt.Errorf("skill-body entry must use path %s", models.SkillFileName)
		}
		return nil
	}

	return requirePackageFile(baseDir, entry.Path, "entry.path")
}

func validateRuntime(baseDir string, runtime models.AssetRuntime) error {
	if runtime.Install == nil {
		return nil
	}
	if runtime.Install.Path != "" {
		if err := requirePackageFile(baseDir, runtime.Install.Path, "runtime.install.path"); err != nil {
			return err
		}
	}
	if runtime.Install.Lockfile != "" {
		if err := requirePackageFile(baseDir, runtime.Install.Lockfile, "runtime.install.lockfile"); err != nil {
			return err
		}
	}
	return nil
}

func validateExports(baseDir string, exports []models.AssetExport) error {
	for index, export := range exports {
		if export.Source == models.SkillFileName {
			continue
		}
		field := fmt.Sprintf("exports[%d].source", index)
		if err := requirePackageFile(baseDir, export.Source, field); err != nil {
			return err
		}
	}
	return nil
}

func requirePackageFile(baseDir, relativePath, field string) error {
	if strings.TrimSpace(relativePath) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if filepath.IsAbs(relativePath) {
		return fmt.Errorf("%s must be a relative package path: %s", field, relativePath)
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("resolve base dir: %w", err)
	}
	absTarget, err := filepath.Abs(filepath.Join(absBase, relativePath))
	if err != nil {
		return fmt.Errorf("resolve %s: %w", field, err)
	}
	if !isWithinBase(absBase, absTarget) {
		return fmt.Errorf("%s must stay within the skill package: %s", field, relativePath)
	}

	if _, err := os.Stat(absTarget); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist: %s", field, relativePath)
		}
		return fmt.Errorf("stat %s: %w", field, err)
	}
	return nil
}

func isWithinBase(baseDir, target string) bool {
	rel, err := filepath.Rel(baseDir, target)
	if err != nil {
		return false
	}
	if rel == ".." {
		return false
	}
	prefix := ".." + string(filepath.Separator)
	return !strings.HasPrefix(rel, prefix)
}
