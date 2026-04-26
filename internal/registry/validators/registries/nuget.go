package registries

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/modelcontextprotocol/registry/pkg/model"
)

var (
	ErrMissingIdentifierForNuget = errors.New("package identifier is required for NuGet packages")
	ErrMissingVersionForNuget    = errors.New("package version is required for NuGet packages")
)

const (
	maxNuGetPackageBytes = 32 << 20
	maxNuGetReadmeBytes  = 1 << 20
)

// ValidateNuGet validates that a NuGet package contains the correct MCP server name
func ValidateNuGet(ctx context.Context, pkg model.Package, serverName string) error {
	// Set default registry base URL if empty
	if pkg.RegistryBaseURL == "" {
		pkg.RegistryBaseURL = model.RegistryURLNuGet
	}

	if pkg.Identifier == "" {
		return ErrMissingIdentifierForNuget
	}

	// Validate that MCPB-specific fields are not present
	if pkg.FileSHA256 != "" {
		return fmt.Errorf("NuGet packages must not have 'fileSha256' field - this is only for MCPB packages")
	}

	// Validate that the registry base URL matches NuGet exactly
	if pkg.RegistryBaseURL != model.RegistryURLNuGet {
		return fmt.Errorf("registry type and base URL do not match: '%s' is not valid for registry type '%s'. Expected: %s",
			pkg.RegistryBaseURL, model.RegistryTypeNuGet, model.RegistryURLNuGet)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	lowerID := strings.ToLower(pkg.Identifier)
	lowerVersion := strings.ToLower(pkg.Version)
	if lowerVersion == "" {
		return ErrMissingVersionForNuget
	}

	readmeContent, err := fetchNuGetReadmeContent(ctx, client, pkg.RegistryBaseURL, lowerID, lowerVersion)
	if err != nil {
		return fmt.Errorf("failed to resolve NuGet README: %w", err)
	}

	// Check for mcp-name: format (more specific)
	mcpNamePattern := "mcp-name: " + serverName
	if strings.Contains(readmeContent, mcpNamePattern) {
		return nil
	}

	return fmt.Errorf("NuGet package '%s' ownership validation failed. The server name '%s' must appear as 'mcp-name: %s' in the package README. Add it to your package README", pkg.Identifier, serverName, serverName)
}

func fetchNuGetReadmeContent(ctx context.Context, client *http.Client, registryBaseURL, packageID, version string) (string, error) {
	readmeContent, found, err := fetchNuGetReadmeEndpoint(ctx, client, registryBaseURL, packageID, version)
	if err != nil {
		return "", err
	}
	if found {
		return readmeContent, nil
	}

	return fetchNuGetReadmeFromPackage(ctx, client, registryBaseURL, packageID, version)
}

func fetchNuGetReadmeEndpoint(ctx context.Context, client *http.Client, registryBaseURL, packageID, version string) (string, bool, error) {
	readmeURL := fmt.Sprintf("%s/v3-flatcontainer/%s/%s/readme", registryBaseURL, packageID, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readmeURL, nil)
	if err != nil {
		return "", false, fmt.Errorf("failed to create README request: %w", err)
	}

	req.Header.Set("User-Agent", "agent-registry-Validator/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("failed to fetch README from NuGet: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		readmeBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxNuGetReadmeBytes+1))
		if err != nil {
			return "", false, fmt.Errorf("failed to read README content: %w", err)
		}
		if len(readmeBytes) > maxNuGetReadmeBytes {
			return "", false, fmt.Errorf("NuGet package README exceeds %d bytes", maxNuGetReadmeBytes)
		}
		return string(readmeBytes), true, nil
	case http.StatusNotFound:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("unexpected status code %d when fetching README from NuGet", resp.StatusCode)
	}
}

func fetchNuGetReadmeFromPackage(ctx context.Context, client *http.Client, registryBaseURL, packageID, version string) (string, error) {
	packageURL := fmt.Sprintf("%s/v3-flatcontainer/%s/%s/%s.%s.nupkg", registryBaseURL, packageID, version, packageID, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create package request: %w", err)
	}

	req.Header.Set("User-Agent", "agent-registry-Validator/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download NuGet package: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", nil
	default:
		return "", fmt.Errorf("unexpected status code %d when downloading NuGet package", resp.StatusCode)
	}

	packageBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxNuGetPackageBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read NuGet package: %w", err)
	}
	if len(packageBytes) > maxNuGetPackageBytes {
		return "", fmt.Errorf("NuGet package exceeds %d bytes", maxNuGetPackageBytes)
	}

	return extractNuGetReadmeFromPackage(packageBytes)
}

func extractNuGetReadmeFromPackage(packageBytes []byte) (string, error) {
	archiveReader, err := zip.NewReader(bytes.NewReader(packageBytes), int64(len(packageBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to read NuGet package archive: %w", err)
	}

	filesByPath := make(map[string]*zip.File, len(archiveReader.File))
	for _, file := range archiveReader.File {
		filesByPath[normalizeNuGetPath(file.Name)] = file
	}

	if readmePath, err := nuGetReadmePathFromNuspec(archiveReader.File); err != nil {
		return "", err
	} else if readmePath != "" {
		if file, ok := filesByPath[readmePath]; ok {
			return readNuGetArchiveFile(file)
		}
	}

	for _, file := range archiveReader.File {
		if isNuGetReadmeFile(file.Name) {
			return readNuGetArchiveFile(file)
		}
	}

	return "", nil
}

func nuGetReadmePathFromNuspec(files []*zip.File) (string, error) {
	for _, file := range files {
		if file.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(file.Name), ".nuspec") {
			continue
		}

		xmlContent, err := readNuGetArchiveFile(file)
		if err != nil {
			return "", fmt.Errorf("failed to read NuGet nuspec: %w", err)
		}

		readmePath, err := parseNuGetReadmePathFromNuspec([]byte(xmlContent))
		if err != nil {
			return "", err
		}
		if readmePath != "" {
			return readmePath, nil
		}
	}

	return "", nil
}

func parseNuGetReadmePathFromNuspec(nuspec []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(nuspec))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("failed to parse NuGet nuspec: %w", err)
		}

		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "readme" {
			continue
		}

		var readmePath string
		if err := decoder.DecodeElement(&readmePath, &start); err != nil {
			return "", fmt.Errorf("failed to decode NuGet readme path: %w", err)
		}
		return normalizeNuGetPath(readmePath), nil
	}
}

func readNuGetArchiveFile(file *zip.File) (string, error) {
	if file.UncompressedSize64 > maxNuGetReadmeBytes {
		return "", fmt.Errorf("NuGet archive file %q exceeds %d bytes", file.Name, maxNuGetReadmeBytes)
	}

	reader, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open %q in NuGet package: %w", file.Name, err)
	}
	defer func() { _ = reader.Close() }()

	content, err := io.ReadAll(io.LimitReader(reader, maxNuGetReadmeBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read %q in NuGet package: %w", file.Name, err)
	}
	if len(content) > maxNuGetReadmeBytes {
		return "", fmt.Errorf("NuGet archive file %q exceeds %d bytes", file.Name, maxNuGetReadmeBytes)
	}

	return string(content), nil
}

func isNuGetReadmeFile(fileName string) bool {
	baseName := strings.ToLower(path.Base(normalizeNuGetPath(fileName)))
	return baseName == "readme" || strings.HasPrefix(baseName, "readme.")
}

func normalizeNuGetPath(fileName string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(fileName), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean("/"+normalized), "/")
}
