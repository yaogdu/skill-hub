package skills

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

const (
	PackageSchemaVersion = "shub.package/v1alpha1"
	DerivedManifestPath  = ".shub/asset-manifest.json"
	PackageMetadataPath  = ".shub/package.json"
)

type PackageBuildResult struct {
	Asset      *models.Asset `json:"asset"`
	OutputPath string        `json:"outputPath"`
	SHA256     string        `json:"sha256"`
	Size       int64         `json:"size"`
	Files      []string      `json:"files,omitempty"`
}

type PackageMetadata struct {
	SchemaVersion string               `json:"schemaVersion"`
	AssetID       string               `json:"assetId"`
	Name          string               `json:"name"`
	Category      models.AssetCategory `json:"category"`
	Version       string               `json:"version"`
	CreatedAt     time.Time            `json:"createdAt"`
	SkillPath     string               `json:"skillPath"`
	ManifestPath  string               `json:"manifestPath"`
}

type packageFile struct {
	AbsolutePath string
	RelativePath string
	Info         fs.FileInfo
}

func BuildPackage(dir, outputPath string) (*PackageBuildResult, error) {
	if strings.TrimSpace(outputPath) == "" {
		return nil, fmt.Errorf("output path is required")
	}

	asset, err := LoadAssetDir(dir)
	if err != nil {
		return nil, err
	}

	files, err := collectPackageFiles(dir)
	if err != nil {
		return nil, err
	}

	manifestJSON, err := json.MarshalIndent(asset.Manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal asset manifest: %w", err)
	}
	metadataJSON, err := json.MarshalIndent(PackageMetadata{
		SchemaVersion: PackageSchemaVersion,
		AssetID:       asset.ID,
		Name:          asset.Name,
		Category:      asset.Category,
		Version:       asset.Version,
		CreatedAt:     time.Now().UTC(),
		SkillPath:     models.SkillFileName,
		ManifestPath:  DerivedManifestPath,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal package metadata: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create package file: %w", err)
	}
	defer func() { _ = outputFile.Close() }()

	hash := sha256.New()
	multiWriter := io.MultiWriter(outputFile, hash)
	gzipWriter := gzip.NewWriter(multiWriter)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, file := range files {
		if err := writePackageFile(tarWriter, file); err != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return nil, err
		}
	}
	if err := writeVirtualPackageFile(tarWriter, DerivedManifestPath, manifestJSON, 0o644); err != nil {
		_ = tarWriter.Close()
		_ = gzipWriter.Close()
		return nil, err
	}
	if err := writeVirtualPackageFile(tarWriter, PackageMetadataPath, metadataJSON, 0o644); err != nil {
		_ = tarWriter.Close()
		_ = gzipWriter.Close()
		return nil, err
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()
		return nil, fmt.Errorf("close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}

	info, err := outputFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat package file: %w", err)
	}

	relativeFiles := make([]string, 0, len(files)+2)
	for _, file := range files {
		relativeFiles = append(relativeFiles, filepath.ToSlash(file.RelativePath))
	}
	relativeFiles = append(relativeFiles, DerivedManifestPath, PackageMetadataPath)
	sort.Strings(relativeFiles)

	return &PackageBuildResult{
		Asset:      asset,
		OutputPath: outputPath,
		SHA256:     hex.EncodeToString(hash.Sum(nil)),
		Size:       info.Size(),
		Files:      relativeFiles,
	}, nil
}

func ExtractPackage(archivePath, targetDir string) error {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open package archive: %w", err)
	}
	defer func() { _ = archiveFile.Close() }()

	return ExtractPackageReader(archiveFile, targetDir)
}

func ExtractPackageReader(reader io.Reader, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create extract directory: %w", err)
	}

	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		targetPath, err := safeArchivePath(targetDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", header.Name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create file parent %s: %w", header.Name, err)
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create extracted file %s: %w", header.Name, err)
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				_ = file.Close()
				return fmt.Errorf("write extracted file %s: %w", header.Name, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close extracted file %s: %w", header.Name, err)
			}
		default:
			return fmt.Errorf("unsupported tar entry type for %s", header.Name)
		}
	}
}

func collectPackageFiles(baseDir string) ([]packageFile, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve package directory: %w", err)
	}

	files := make([]packageFile, 0)
	err = filepath.Walk(absBase, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == absBase {
			return nil
		}

		rel, err := filepath.Rel(absBase, path)
		if err != nil {
			return fmt.Errorf("resolve package path: %w", err)
		}
		rel = filepath.ToSlash(rel)

		if shouldSkipPackagePath(rel, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in SHUB packages yet: %s", rel)
		}

		files = append(files, packageFile{AbsolutePath: path, RelativePath: rel, Info: info})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk package files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files, nil
}

func shouldSkipPackagePath(relativePath string, info os.FileInfo) bool {
	clean := strings.Trim(filepath.ToSlash(relativePath), "/")
	if clean == "" {
		return false
	}
	for _, prefix := range []string{".git", ".shub"} {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return true
		}
	}
	if strings.HasSuffix(clean, "/.DS_Store") || clean == ".DS_Store" {
		return true
	}
	if info.IsDir() && (clean == "dist" || strings.HasPrefix(clean, "dist/")) {
		return false
	}
	return false
}

func writePackageFile(writer *tar.Writer, file packageFile) error {
	header, err := tar.FileInfoHeader(file.Info, "")
	if err != nil {
		return fmt.Errorf("build tar header for %s: %w", file.RelativePath, err)
	}
	header.Name = filepath.ToSlash(file.RelativePath)
	header.ModTime = time.Time{}
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}

	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header for %s: %w", file.RelativePath, err)
	}

	sourceFile, err := os.Open(file.AbsolutePath)
	if err != nil {
		return fmt.Errorf("open package file %s: %w", file.RelativePath, err)
	}
	defer func() { _ = sourceFile.Close() }()

	if _, err := io.Copy(writer, sourceFile); err != nil {
		return fmt.Errorf("write tar contents for %s: %w", file.RelativePath, err)
	}
	return nil
}

func writeVirtualPackageFile(writer *tar.Writer, relativePath string, data []byte, mode os.FileMode) error {
	header := &tar.Header{
		Name:       filepath.ToSlash(relativePath),
		Mode:       int64(mode),
		Size:       int64(len(data)),
		ModTime:    time.Time{},
		AccessTime: time.Time{},
		ChangeTime: time.Time{},
	}
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header for %s: %w", relativePath, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write tar contents for %s: %w", relativePath, err)
	}
	return nil
}

func safeArchivePath(baseDir, entryName string) (string, error) {
	if strings.TrimSpace(entryName) == "" {
		return "", fmt.Errorf("package archive contains an empty path")
	}
	cleanName := filepath.Clean(entryName)
	if cleanName == "." {
		return "", fmt.Errorf("package archive contains an invalid path: %s", entryName)
	}
	if filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("package archive path must be relative: %s", entryName)
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve extract directory: %w", err)
	}
	absTarget, err := filepath.Abs(filepath.Join(absBase, cleanName))
	if err != nil {
		return "", fmt.Errorf("resolve archive entry %s: %w", entryName, err)
	}
	if !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) && absTarget != absBase {
		return "", fmt.Errorf("package archive path escapes target directory: %s", entryName)
	}
	return absTarget, nil
}
