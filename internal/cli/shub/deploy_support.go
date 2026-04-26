package shub

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common/gitutil"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
)

type SkillPublisher interface {
	CreateSkill(skill *models.SkillJSON) (*models.SkillResponse, error)
}

type AssetPublisher interface {
	CreateAsset(request *models.AssetPublishRequest) (*models.AssetResponse, error)
}

type AssetPackageUploader interface {
	UploadAssetPackage(assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error)
	AssetPackageURL(assetID, version string) string
}

type DeployOptions struct {
	GitRepository string
	GitProvider   string
	DockerImage   string
	PackageURL    string
	DryRun        bool
}

type DeployResult struct {
	Asset        *models.Asset               `json:"asset"`
	AssetPayload *models.AssetPublishRequest `json:"assetPayload,omitempty"`
	Audit        *shubskills.AuditReport     `json:"audit,omitempty"`
	Payload      *models.SkillJSON           `json:"payload"`
	InputPath    string                      `json:"inputPath"`
	PackageURL   string                      `json:"packageUrl,omitempty"`
	Published    bool                        `json:"published"`
}

func DeployAsset(inputPath string, publisher SkillPublisher, opts DeployOptions) (*DeployResult, error) {
	asset, absInput, auditDir, inferredGitRepository, inferredPackageURL, localArchivePath, cleanup, err := loadDeployAsset(inputPath, opts.GitProvider)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if strings.TrimSpace(opts.GitRepository) == "" {
		opts.GitRepository = inferredGitRepository
	}
	if strings.TrimSpace(opts.PackageURL) == "" && strings.TrimSpace(localArchivePath) == "" {
		opts.PackageURL = inferredPackageURL
	}
	audit, err := shubskills.AuditDir(auditDir)
	if err != nil {
		return nil, fmt.Errorf("audit SHUB asset: %w", err)
	}
	if blocking := audit.BlockingError(); blocking != nil {
		return nil, fmt.Errorf("audit SHUB asset: %w", blocking)
	}
	if !opts.DryRun && strings.TrimSpace(opts.PackageURL) == "" && strings.TrimSpace(localArchivePath) != "" {
		uploadedURL, err := uploadArchiveIfSupported(localArchivePath, asset, publisher)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(uploadedURL) != "" {
			opts.PackageURL = uploadedURL
		} else {
			opts.PackageURL = inferredPackageURL
		}
	}

	assetPayload, err := buildAssetPublishRequest(asset, opts)
	if err != nil {
		return nil, err
	}
	payload, err := assetPayload.ToSkillJSON()
	if err != nil {
		return nil, fmt.Errorf("build compatibility skill payload: %w", err)
	}

	result := &DeployResult{
		Asset:        asset,
		AssetPayload: assetPayload,
		Audit:        audit,
		Payload:      payload,
		InputPath:    absInput,
		PackageURL:   opts.PackageURL,
	}
	if opts.DryRun {
		return result, nil
	}
	if publisher == nil {
		return nil, fmt.Errorf("API client not initialized")
	}

	if assetPublisher, ok := publisher.(AssetPublisher); ok {
		if _, err := assetPublisher.CreateAsset(assetPayload); err == nil {
			result.Published = true
			return result, nil
		} else if !shouldFallbackToSkillPublish(err) {
			return nil, fmt.Errorf("publish asset via asset API: %w", err)
		}
	}

	if _, err := publisher.CreateSkill(payload); err != nil {
		return nil, fmt.Errorf("publish asset via compatibility skill API: %w", err)
	}
	result.Published = true
	return result, nil
}

func uploadArchiveIfSupported(archivePath string, asset *models.Asset, publisher SkillPublisher) (string, error) {
	if publisher == nil {
		return "", nil
	}
	uploader, ok := publisher.(AssetPackageUploader)
	if !ok {
		return "", nil
	}
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		return "", fmt.Errorf("read SHUB package archive: %w", err)
	}
	response, err := uploader.UploadAssetPackage(asset.ID, asset.Version, archiveBytes, "application/gzip")
	if err != nil {
		return "", fmt.Errorf("upload SHUB package archive: %w", err)
	}
	if response != nil && strings.TrimSpace(response.DownloadURL) != "" {
		return strings.TrimSpace(response.DownloadURL), nil
	}
	return strings.TrimSpace(uploader.AssetPackageURL(asset.ID, asset.Version)), nil
}

func buildAssetPublishRequest(asset *models.Asset, opts DeployOptions) (*models.AssetPublishRequest, error) {
	if asset == nil {
		return nil, fmt.Errorf("asset is nil")
	}
	if !opts.DryRun && strings.TrimSpace(opts.GitRepository) == "" && strings.TrimSpace(opts.DockerImage) == "" && strings.TrimSpace(opts.PackageURL) == "" {
		return nil, fmt.Errorf("deploy requires --package-url, --git, or --docker-image unless --dry-run is used")
	}

	return &models.AssetPublishRequest{
		Manifest: asset.Manifest,
		Source:   buildDeploySource(opts),
	}, nil
}

func buildDeployPayload(asset *models.Asset, opts DeployOptions) (*models.SkillJSON, error) {
	request, err := buildAssetPublishRequest(asset, opts)
	if err != nil {
		return nil, err
	}
	return request.ToSkillJSON()
}

func buildDeploySource(opts DeployOptions) *models.AssetSource {
	source := &models.AssetSource{RepositoryURL: strings.TrimSpace(opts.GitRepository)}
	if strings.TrimSpace(opts.PackageURL) != "" {
		source.PackageType = "tarball"
		source.PackageRef = strings.TrimSpace(opts.PackageURL)
	}
	if strings.TrimSpace(opts.DockerImage) != "" {
		source.PackageType = "docker"
		source.PackageRef = strings.TrimSpace(opts.DockerImage)
	}
	if source.RepositoryURL == "" && source.PackageRef == "" {
		return nil
	}
	return source
}

func shouldFallbackToSkillPublish(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found")
}

func loadDeployAsset(inputPath, gitProvider string) (*models.Asset, string, string, string, string, string, func(), error) {
	absInput, err := filepath.Abs(inputPath)
	if err != nil {
		return nil, "", "", "", "", "", func() {}, fmt.Errorf("resolve input path %q: %w", inputPath, err)
	}
	info, err := os.Stat(absInput)
	if err != nil {
		return nil, "", "", "", "", "", func() {}, fmt.Errorf("stat input path %q: %w", inputPath, err)
	}
	if info.IsDir() {
		asset, err := shubskills.LoadAssetDir(absInput)
		if err != nil {
			return nil, "", "", "", "", "", func() {}, fmt.Errorf("load SHUB asset: %w", err)
		}
		inferredGitRepository := ""
		if gitURL, gitErr := gitutil.InferRepositoryTreeURLWithProvider(absInput, gitProvider); gitErr == nil {
			inferredGitRepository = gitURL
		}
		return asset, absInput, absInput, inferredGitRepository, "", "", func() {}, nil
	}
	if !isArchivePath(absInput) {
		return nil, "", "", "", "", "", func() {}, fmt.Errorf("unsupported deploy input %q: expected a directory or .tar.gz/.tgz archive", inputPath)
	}

	tempDir, err := os.MkdirTemp("", "shub-deploy-*")
	if err != nil {
		return nil, "", "", "", "", "", func() {}, fmt.Errorf("create deploy temp directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	if err := shubskills.ExtractPackage(absInput, tempDir); err != nil {
		cleanup()
		return nil, "", "", "", "", "", func() {}, fmt.Errorf("extract SHUB package: %w", err)
	}
	asset, err := shubskills.LoadAssetDir(tempDir)
	if err != nil {
		cleanup()
		return nil, "", "", "", "", "", func() {}, fmt.Errorf("load SHUB asset from package: %w", err)
	}
	return asset, absInput, tempDir, "", fileURL(absInput), absInput, cleanup, nil
}

func isArchivePath(path string) bool {
	return strings.HasSuffix(path, ".tar.gz") || strings.HasSuffix(path, ".tgz")
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
