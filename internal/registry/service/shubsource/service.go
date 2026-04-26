package shubsource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	yaml "gopkg.in/yaml.v3"
)

type SourceStore interface {
	ListSHUBSources(ctx context.Context) ([]*models.SHUBSource, error)
	GetSHUBSource(ctx context.Context, name string) (*models.SHUBSource, error)
	PutSHUBSource(ctx context.Context, source *models.SHUBSource) (*models.SHUBSource, error)
	DeleteSHUBSource(ctx context.Context, name string) error
}

type AssetRegistry interface {
	GetAsset(ctx context.Context, assetID string) (*models.AssetResponse, error)
	GetAssetVersion(ctx context.Context, assetID, version string) (*models.AssetResponse, error)
	PublishAsset(ctx context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error)
	UploadAssetPackage(ctx context.Context, assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error)
}

type Dependencies struct {
	Sources SourceStore
	Assets  AssetRegistry
	Fetcher Fetcher
}

type Registry interface {
	ListSources(ctx context.Context) ([]*models.SHUBSource, error)
	GetSource(ctx context.Context, name string) (*models.SHUBSource, error)
	SetSource(ctx context.Context, name, address string) (*models.SHUBSource, error)
	DeleteSource(ctx context.Context, name string) error
	PullAsset(ctx context.Context, sourceName, assetID, version string) (*models.AssetResponse, error)
}

type registry struct {
	sources SourceStore
	assets  AssetRegistry
	fetcher Fetcher
}

const importedAssetFallbackVersion = "0.0.0-imported"

func New(deps Dependencies) Registry {
	fetcher := deps.Fetcher
	if fetcher == nil {
		fetcher = gitFetcher{}
	}
	return &registry{sources: deps.Sources, assets: deps.Assets, fetcher: fetcher}
}

func (r *registry) ListSources(ctx context.Context) ([]*models.SHUBSource, error) {
	builtIns := listBuiltInSources()
	if r.sources == nil {
		return builtIns, nil
	}
	customSources, err := r.sources.ListSHUBSources(ctx)
	if err != nil {
		return nil, err
	}
	if len(customSources) == 0 {
		return builtIns, nil
	}
	result := make([]*models.SHUBSource, 0, len(builtIns)+len(customSources))
	seen := make(map[string]struct{}, len(builtIns)+len(customSources))
	for _, source := range builtIns {
		result = append(result, source)
		seen[source.Name] = struct{}{}
	}
	for _, source := range customSources {
		if source == nil {
			continue
		}
		if _, ok := seen[source.Name]; ok {
			continue
		}
		result = append(result, source)
		seen[source.Name] = struct{}{}
	}
	return result, nil
}

func (r *registry) GetSource(ctx context.Context, name string) (*models.SHUBSource, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, fmt.Errorf("%w: SHUB source name is required", database.ErrInvalidInput)
	}
	if builtIn, ok := getBuiltInSource(trimmedName); ok {
		return builtIn, nil
	}
	if r.sources == nil {
		return nil, database.ErrStoreNotConfigured
	}
	return r.sources.GetSHUBSource(ctx, trimmedName)
}

func (r *registry) SetSource(ctx context.Context, name, address string) (*models.SHUBSource, error) {
	if r.sources == nil {
		return nil, database.ErrStoreNotConfigured
	}
	trimmedName := strings.TrimSpace(name)
	trimmedAddress := strings.TrimSpace(address)
	if trimmedName == "" {
		return nil, fmt.Errorf("%w: SHUB source name is required", database.ErrInvalidInput)
	}
	if trimmedAddress == "" {
		return nil, fmt.Errorf("%w: SHUB source address is required", database.ErrInvalidInput)
	}
	if _, ok := getBuiltInSource(trimmedName); ok {
		return nil, fmt.Errorf("%w: SHUB source %q is built in and cannot be edited", database.ErrInvalidInput, trimmedName)
	}
	if err := validateSourceAddress(trimmedAddress); err != nil {
		return nil, err
	}
	return r.sources.PutSHUBSource(ctx, &models.SHUBSource{Name: trimmedName, Address: trimmedAddress})
}

func (r *registry) DeleteSource(ctx context.Context, name string) error {
	if r.sources == nil {
		return database.ErrStoreNotConfigured
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return fmt.Errorf("%w: SHUB source name is required", database.ErrInvalidInput)
	}
	if _, ok := getBuiltInSource(trimmedName); ok {
		return fmt.Errorf("%w: SHUB source %q is built in and cannot be deleted", database.ErrInvalidInput, trimmedName)
	}
	return r.sources.DeleteSHUBSource(ctx, trimmedName)
}

func (r *registry) PullAsset(ctx context.Context, sourceName, assetID, version string) (*models.AssetResponse, error) {
	if r.assets == nil {
		return nil, database.ErrStoreNotConfigured
	}
	trimmedSource := strings.TrimSpace(sourceName)
	trimmedAssetID := strings.TrimSpace(assetID)
	trimmedVersion := strings.TrimSpace(version)
	if trimmedSource == "" {
		return nil, fmt.Errorf("%w: SHUB source name is required", database.ErrInvalidInput)
	}
	if trimmedAssetID == "" {
		return nil, fmt.Errorf("%w: asset id is required", database.ErrInvalidInput)
	}

	if existing, err := r.lookupExisting(ctx, trimmedAssetID, trimmedVersion); err == nil && existing != nil {
		return existing, nil
	} else if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	source, err := r.GetSource(ctx, trimmedSource)
	if err != nil {
		return nil, err
	}

	workDir, err := os.MkdirTemp("", "agentregistry-shub-source-*")
	if err != nil {
		return nil, fmt.Errorf("create SHUB source work directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	resolvedAddress, err := r.fetcher.Fetch(ctx, source, trimmedAssetID, trimmedVersion, workDir)
	if err != nil {
		return nil, fmt.Errorf("fetch asset from SHUB source %q: %w", trimmedSource, err)
	}

	asset, err := loadFetchedAssetDir(workDir, trimmedAssetID, trimmedVersion)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(asset.ID) != trimmedAssetID {
		return nil, fmt.Errorf("%w: fetched asset id %q does not match requested %q", database.ErrInvalidInput, asset.ID, trimmedAssetID)
	}
	if trimmedVersion != "" && strings.TrimSpace(asset.Version) != trimmedVersion {
		return nil, fmt.Errorf("%w: fetched asset version %q does not match requested %q", database.ErrInvalidInput, asset.Version, trimmedVersion)
	}

	archivePath, err := newTempArchivePath()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(archivePath) }()

	build, err := shubskills.BuildPackage(workDir, archivePath)
	if err != nil {
		return nil, fmt.Errorf("build SHUB package from fetched asset: %w", err)
	}
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("read built SHUB package: %w", err)
	}
	if _, err := r.assets.UploadAssetPackage(ctx, build.Asset.ID, build.Asset.Version, archiveBytes, "application/gzip"); err != nil {
		return nil, fmt.Errorf("upload fetched SHUB package into registry storage: %w", err)
	}

	publishSource := cloneSource(build.Asset.Source)
	if publishSource == nil {
		publishSource = &models.AssetSource{}
	}
	if strings.TrimSpace(publishSource.RepositoryURL) == "" {
		publishSource.RepositoryURL = resolvedAddress
	}
	publishSource.PackageType = "tarball"
	publishSource.PackageRef = registryPackageRef(build.Asset.ID, build.Asset.Version)

	published, err := r.assets.PublishAsset(ctx, &models.AssetPublishRequest{Manifest: build.Asset.Manifest, Source: publishSource})
	if err != nil {
		if errors.Is(err, database.ErrInvalidVersion) {
			return r.assets.GetAssetVersion(ctx, build.Asset.ID, build.Asset.Version)
		}
		return nil, fmt.Errorf("publish fetched SHUB asset into registry: %w", err)
	}
	return published, nil
}

func (r *registry) lookupExisting(ctx context.Context, assetID, version string) (*models.AssetResponse, error) {
	if strings.TrimSpace(version) == "" {
		asset, err := r.assets.GetAsset(ctx, assetID)
		if err != nil {
			return nil, err
		}
		if asset == nil {
			return nil, database.ErrNotFound
		}
		return asset, nil
	}
	asset, err := r.assets.GetAssetVersion(ctx, assetID, version)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, database.ErrNotFound
	}
	return asset, nil
}

func validateSourceAddress(address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("%w: invalid SHUB source address: %v", database.ErrInvalidInput, err)
	}
	if scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme)); scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: SHUB source address must use http or https", database.ErrInvalidInput)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("%w: SHUB source address must include a host", database.ErrInvalidInput)
	}
	return nil
}

func registryPackageRef(assetID, version string) string {
	return "/v0/assets/" + url.PathEscape(assetID) + "/versions/" + url.PathEscape(version) + "/package"
}

func loadFetchedAssetDir(dir, assetID, requestedVersion string) (*models.Asset, error) {
	asset, err := shubskills.LoadAssetDir(dir)
	if err == nil {
		return asset, nil
	}
	if compatErr := synthesizeImportedSkillAsset(dir, assetID, requestedVersion); compatErr != nil {
		return nil, fmt.Errorf("load fetched SHUB asset: %w; compat import failed: %v", err, compatErr)
	}
	asset, err = shubskills.LoadAssetDir(dir)
	if err != nil {
		return nil, fmt.Errorf("load fetched SHUB asset: %w", err)
	}
	return asset, nil
}

func synthesizeImportedSkillAsset(dir, assetID, requestedVersion string) error {
	skillPath := filepath.Join(dir, models.SkillFileName)
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		return err
	}

	frontmatter, body, err := splitSkillFrontmatter(string(raw))
	if err != nil {
		return err
	}

	var payload map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &payload); err != nil {
		return fmt.Errorf("parse imported skill frontmatter: %w", err)
	}
	if strings.TrimSpace(stringValue(payload["name"])) == "" {
		return fmt.Errorf("imported skill frontmatter missing required field: name")
	}
	if strings.TrimSpace(stringValue(payload["description"])) == "" {
		return fmt.Errorf("imported skill frontmatter missing required field: description")
	}
	if strings.TrimSpace(stringValue(payload["version"])) == "" {
		payload["version"] = deriveImportedAssetVersion(dir, requestedVersion)
	}

	shubPayload := mapValue(payload["shub"])
	if strings.TrimSpace(stringValue(shubPayload["schemaVersion"])) == "" {
		shubPayload["schemaVersion"] = models.ShubSkillSchemaVersion
	}
	shubPayload["id"] = assetID
	if !models.AssetCategory(stringValue(shubPayload["category"])).IsValid() {
		shubPayload["category"] = string(models.AssetCategoryPrompt)
	}
	shubPayload["entry"] = map[string]any{
		"kind": "skill-body",
		"path": models.SkillFileName,
	}
	shubPayload["runtime"] = map[string]any{
		"type": "none",
	}
	if !hasExports(shubPayload["exports"]) {
		shubPayload["exports"] = []map[string]any{
			{
				"target": "codex",
				"mode":   "skill-dir",
				"source": ".",
			},
			{
				"target": "claude-code",
				"mode":   "skill-dir",
				"source": ".",
			},
		}
	}
	payload["shub"] = shubPayload

	encoded, err := yaml.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal imported skill frontmatter: %w", err)
	}
	rewritten := "---\n" + string(encoded) + "---\n" + body
	if err := os.WriteFile(skillPath, []byte(rewritten), 0o644); err != nil {
		return fmt.Errorf("write synthesized SHUB skill metadata: %w", err)
	}
	return nil
}

func deriveImportedAssetVersion(dir, requestedVersion string) string {
	if trimmed := strings.TrimSpace(requestedVersion); trimmed != "" {
		return trimmed
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--short=12", "HEAD")
	output, err := cmd.Output()
	if err == nil {
		if hash := strings.TrimSpace(string(output)); hash != "" {
			return "0.0.0-git." + hash
		}
	}
	return importedAssetFallbackVersion
}

func splitSkillFrontmatter(content string) (string, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.TrimPrefix(normalized, "\ufeff")
	if strings.TrimSpace(normalized) == "" {
		return "", "", fmt.Errorf("SKILL.md is empty")
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", fmt.Errorf("SKILL.md missing YAML frontmatter delimited by ---")
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) != "---" {
			continue
		}
		frontmatter := strings.Join(lines[1:index], "\n")
		body := strings.Join(lines[index+1:], "\n")
		body = strings.TrimLeft(body, "\n")
		return frontmatter, body, nil
	}
	return "", "", fmt.Errorf("SKILL.md missing YAML frontmatter delimited by ---")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func mapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, entry := range typed {
			normalized[fmt.Sprint(key)] = entry
		}
		return normalized
	default:
		return make(map[string]any)
	}
}

func hasExports(value any) bool {
	switch typed := value.(type) {
	case []any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	default:
		return false
	}
}

func newTempArchivePath() (string, error) {
	file, err := os.CreateTemp("", "agentregistry-shub-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create SHUB package temp file: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close SHUB package temp file: %w", err)
	}
	return path, nil
}

func cloneSource(source *models.AssetSource) *models.AssetSource {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}
