package registries

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchNuGetReadmeContentFallsBackToPackageArchive(t *testing.T) {
	t.Parallel()

	packageBytes := buildNuGetArchive(t, map[string]string{
		"Example.nuspec": `<?xml version="1.0"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <id>Example</id>
    <version>1.0.0</version>
    <readme>docs/guide.md</readme>
  </metadata>
</package>`,
		"docs/guide.md": "mcp-name: com.example/server\n",
	})

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/v3-flatcontainer/example/1.0.0/readme":
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			case "/v3-flatcontainer/example/1.0.0/example.1.0.0.nupkg":
				return newHTTPResponse(http.StatusOK, string(packageBytes)), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		}),
	}

	content, err := fetchNuGetReadmeContent(context.Background(), client, "https://nuget.example.test", "example", "1.0.0")
	require.NoError(t, err)
	require.Contains(t, content, "mcp-name: com.example/server")
}

func TestFetchNuGetReadmeContentPrefersReadmeEndpoint(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/v3-flatcontainer/example/1.0.0/readme":
				return newHTTPResponse(http.StatusOK, "mcp-name: com.example/endpoint\n"), nil
			case "/v3-flatcontainer/example/1.0.0/example.1.0.0.nupkg":
				t.Fatalf("package archive should not be requested when readme endpoint succeeds")
				return nil, nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		}),
	}

	content, err := fetchNuGetReadmeContent(context.Background(), client, "https://nuget.example.test", "example", "1.0.0")
	require.NoError(t, err)
	require.Contains(t, content, "mcp-name: com.example/endpoint")
}

func buildNuGetArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		fileWriter, err := writer.Create(name)
		require.NoError(t, err)
		_, err = fileWriter.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	return buffer.Bytes()
}

func TestParseNuGetReadmePathFromNuspec(t *testing.T) {
	t.Parallel()

	readmePath, err := parseNuGetReadmePathFromNuspec([]byte(`<?xml version="1.0"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <readme>docs\\README.md</readme>
  </metadata>
</package>`))
	require.NoError(t, err)
	require.Equal(t, "docs/README.md", readmePath)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func newHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
