package asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
)

const assetPackageContentType = "application/gzip"

type PackageStore interface {
	Put(ctx context.Context, assetID, version string, content []byte, uploadedAt time.Time) (*models.AssetPackage, error)
	Get(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error)
}

type filesystemPackageStore struct {
	rootDir string
	authz   auth.Authorizer
}

func NewFilesystemPackageStore(rootDir string, authz auth.Authorizer) (PackageStore, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil, fmt.Errorf("asset package storage dir is required")
	}
	return &filesystemPackageStore{rootDir: filepath.Clean(rootDir), authz: authz}, nil
}

func (store *filesystemPackageStore) Put(ctx context.Context, assetID, version string, content []byte, uploadedAt time.Time) (*models.AssetPackage, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionPublish, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return nil, err
	}
	packagePath, err := store.packagePath(assetID, version)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(packagePath), 0o755); err != nil {
		return nil, fmt.Errorf("create asset package directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(packagePath), "package-*.tgz")
	if err != nil {
		return nil, fmt.Errorf("create asset package temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return nil, fmt.Errorf("write asset package temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("close asset package temp file: %w", err)
	}
	if !uploadedAt.IsZero() {
		if err := os.Chtimes(tempPath, uploadedAt, uploadedAt); err != nil {
			return nil, fmt.Errorf("set asset package timestamps: %w", err)
		}
	}
	if err := os.Rename(tempPath, packagePath); err != nil {
		if removeErr := os.Remove(packagePath); removeErr == nil {
			err = os.Rename(tempPath, packagePath)
		}
		if err != nil {
			return nil, fmt.Errorf("move asset package into place: %w", err)
		}
	}

	return buildAssetPackageMetadata(assetID, version, content, uploadedAt), nil
}

func (store *filesystemPackageStore) Get(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return nil, err
	}
	packagePath, err := store.packagePath(assetID, version)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(packagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, database.ErrNotFound
		}
		return nil, fmt.Errorf("read asset package: %w", err)
	}
	info, err := os.Stat(packagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, database.ErrNotFound
		}
		return nil, fmt.Errorf("stat asset package: %w", err)
	}
	return &models.AssetPackageDownload{
		Package: *buildAssetPackageMetadata(assetID, version, content, info.ModTime()),
		Content: content,
	}, nil
}

func (store *filesystemPackageStore) packagePath(assetID, version string) (string, error) {
	assetID = strings.TrimSpace(assetID)
	version = strings.TrimSpace(version)
	if assetID == "" || version == "" {
		return "", fmt.Errorf("asset id and version are required")
	}
	escapedID := url.PathEscape(assetID)
	escapedVersion := url.PathEscape(version)
	return filepath.Join(store.rootDir, "assets", escapedID, escapedVersion, "package.tgz"), nil
}

func buildAssetPackageMetadata(assetID, version string, content []byte, uploadedAt time.Time) *models.AssetPackage {
	sum := sha256.Sum256(content)
	return &models.AssetPackage{
		AssetID:     assetID,
		Version:     version,
		ContentType: assetPackageContentType,
		SizeBytes:   len(content),
		SHA256:      hex.EncodeToString(sum[:]),
		UploadedAt:  uploadedAt.UTC(),
	}
}
