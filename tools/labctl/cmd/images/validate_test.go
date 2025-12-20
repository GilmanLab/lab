package images

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHTTPClient implements httpClient for testing.
type mockHTTPClient struct {
	responses map[string]*http.Response
	errors    map[string]error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	if err, ok := m.errors[url]; ok {
		return nil, err
	}
	if resp, ok := m.responses[url]; ok {
		return resp, nil
	}
	// Default: return 200 OK
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestRunValidateWithClient(t *testing.T) {
	// Save and restore the global validateManifest
	origManifest := validateManifest
	defer func() { validateManifest = origManifest }()

	t.Run("valid manifest with accessible URLs", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")

		manifest := `apiVersion: images.lab.gilman.io/v1alpha1
kind: ImageManifest
metadata:
  name: test-images
spec:
  images:
    - name: test-image
      source:
        url: https://example.com/test.iso
        checksum: sha256:abc123
      destination: test/test.iso
`
		err := os.WriteFile(manifestPath, []byte(manifest), 0o644) //nolint:gosec
		require.NoError(t, err)

		validateManifest = manifestPath
		client := &mockHTTPClient{
			responses: map[string]*http.Response{
				"https://example.com/test.iso": {
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("")),
				},
			},
		}

		err = runValidateWithClient(client)
		assert.NoError(t, err)
	})

	t.Run("manifest with multiple validation errors", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")

		// Manifest with multiple errors: http URL, missing checksum
		manifest := `apiVersion: images.lab.gilman.io/v1alpha1
kind: ImageManifest
metadata:
  name: test-images
spec:
  images:
    - name: bad-image-1
      source:
        url: http://insecure.com/test.iso
        checksum: sha256:abc123
      destination: test/test1.iso
    - name: bad-image-2
      source:
        url: https://example.com/test.iso
        checksum: ""
      destination: test/test2.iso
`
		err := os.WriteFile(manifestPath, []byte(manifest), 0o644) //nolint:gosec
		require.NoError(t, err)

		validateManifest = manifestPath
		client := &mockHTTPClient{}

		err = runValidateWithClient(client)
		assert.Error(t, err)
		// Should report multiple errors
		assert.Contains(t, err.Error(), "2 error(s)")
	})

	t.Run("manifest with URL check failure", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")

		manifest := `apiVersion: images.lab.gilman.io/v1alpha1
kind: ImageManifest
metadata:
  name: test-images
spec:
  images:
    - name: unreachable-image
      source:
        url: https://unreachable.example.com/test.iso
        checksum: sha256:abc123
      destination: test/test.iso
`
		err := os.WriteFile(manifestPath, []byte(manifest), 0o644) //nolint:gosec
		require.NoError(t, err)

		validateManifest = manifestPath
		client := &mockHTTPClient{
			responses: map[string]*http.Response{
				"https://unreachable.example.com/test.iso": {
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Body:       io.NopCloser(strings.NewReader("")),
				},
			},
		}

		err = runValidateWithClient(client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "1 error(s)")
	})

	t.Run("manifest file not found", func(t *testing.T) {
		validateManifest = "/nonexistent/path/images.yaml"
		client := &mockHTTPClient{}

		err := runValidateWithClient(client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load manifest")
	})

	t.Run("invalid YAML", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")

		err := os.WriteFile(manifestPath, []byte("not: valid: yaml: ["), 0o644) //nolint:gosec
		require.NoError(t, err)

		validateManifest = manifestPath
		client := &mockHTTPClient{}

		err = runValidateWithClient(client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load manifest")
	})

	t.Run("collects both manifest and URL errors", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")

		// One image with http URL (manifest error), one with unreachable URL
		manifest := `apiVersion: images.lab.gilman.io/v1alpha1
kind: ImageManifest
metadata:
  name: test-images
spec:
  images:
    - name: http-image
      source:
        url: http://insecure.com/test.iso
        checksum: sha256:abc123
      destination: test/test1.iso
    - name: unreachable-image
      source:
        url: https://unreachable.example.com/test.iso
        checksum: sha256:def456
      destination: test/test2.iso
`
		err := os.WriteFile(manifestPath, []byte(manifest), 0o644) //nolint:gosec
		require.NoError(t, err)

		validateManifest = manifestPath
		client := &mockHTTPClient{
			responses: map[string]*http.Response{
				"https://unreachable.example.com/test.iso": {
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Body:       io.NopCloser(strings.NewReader("")),
				},
			},
		}

		err = runValidateWithClient(client)
		assert.Error(t, err)
		// Should report 2 errors: http URL + unreachable URL
		assert.Contains(t, err.Error(), "2 error(s)")
	})
}

func TestCheckURL(t *testing.T) {
	t.Run("successful HEAD request", func(t *testing.T) {
		client := &mockHTTPClient{
			responses: map[string]*http.Response{
				"https://example.com/test.iso": {
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("")),
				},
			},
		}

		err := checkURL(context.Background(), client, "https://example.com/test.iso")
		assert.NoError(t, err)
	})

	t.Run("redirect is acceptable", func(t *testing.T) {
		client := &mockHTTPClient{
			responses: map[string]*http.Response{
				"https://example.com/redirect": {
					StatusCode: http.StatusFound,
					Status:     "302 Found",
					Body:       io.NopCloser(strings.NewReader("")),
				},
			},
		}

		err := checkURL(context.Background(), client, "https://example.com/redirect")
		assert.NoError(t, err)
	})

	t.Run("404 returns error", func(t *testing.T) {
		client := &mockHTTPClient{
			responses: map[string]*http.Response{
				"https://example.com/notfound": {
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Body:       io.NopCloser(strings.NewReader("")),
				},
			},
		}

		err := checkURL(context.Background(), client, "https://example.com/notfound")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("500 returns error", func(t *testing.T) {
		client := &mockHTTPClient{
			responses: map[string]*http.Response{
				"https://example.com/error": {
					StatusCode: http.StatusInternalServerError,
					Status:     "500 Internal Server Error",
					Body:       io.NopCloser(strings.NewReader("")),
				},
			},
		}

		err := checkURL(context.Background(), client, "https://example.com/error")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("network error", func(t *testing.T) {
		client := &mockHTTPClient{
			errors: map[string]error{
				"https://example.com/network-error": io.EOF,
			},
		}

		err := checkURL(context.Background(), client, "https://example.com/network-error")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD request failed")
	})
}
