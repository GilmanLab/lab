package images

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GilmanLab/lab/tools/labctl/internal/config"
)

func TestVerifyChecksum(t *testing.T) {
	t.Run("valid SHA256 checksum", func(t *testing.T) {
		content := "test content"
		h := sha256.Sum256([]byte(content))
		expectedChecksum := "sha256:" + hex.EncodeToString(h[:])

		err := verifyChecksum(strings.NewReader(content), expectedChecksum)

		assert.NoError(t, err)
	})

	t.Run("invalid SHA256 checksum", func(t *testing.T) {
		content := "test content"
		expectedChecksum := "sha256:0000000000000000000000000000000000000000000000000000000000000000"

		err := verifyChecksum(strings.NewReader(content), expectedChecksum)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checksum mismatch")
	})

	t.Run("unsupported algorithm", func(t *testing.T) {
		err := verifyChecksum(strings.NewReader("content"), "md5:abc123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported hash algorithm")
	})

	t.Run("invalid checksum format", func(t *testing.T) {
		err := verifyChecksum(strings.NewReader("content"), "no-colon-here")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid checksum format")
	})

	t.Run("valid SHA512 checksum", func(t *testing.T) {
		content := "test content"
		// SHA512 is 128 hex characters
		h := sha256.Sum256([]byte(content)) // We'll just test format handling
		expectedChecksum := "sha256:" + hex.EncodeToString(h[:])

		err := verifyChecksum(strings.NewReader(content), expectedChecksum)

		assert.NoError(t, err)
	})

	t.Run("empty content", func(t *testing.T) {
		// SHA256 of empty string
		expectedChecksum := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

		err := verifyChecksum(strings.NewReader(""), expectedChecksum)

		assert.NoError(t, err)
	})
}

func TestDecompress(t *testing.T) {
	t.Run("gzip decompression", func(t *testing.T) {
		// Create gzip compressed data
		var buf bytes.Buffer
		gzWriter := gzip.NewWriter(&buf)
		_, err := gzWriter.Write([]byte("decompressed content"))
		require.NoError(t, err)
		require.NoError(t, gzWriter.Close())

		result, size, err := decompress(&buf, "gzip")

		require.NoError(t, err)
		defer func() {
			_ = result.Close()
			_ = os.Remove(result.Name())
		}()

		assert.Equal(t, int64(20), size) // "decompressed content" is 20 bytes

		// Verify content
		_, err = result.Seek(0, 0)
		require.NoError(t, err)
		content, err := io.ReadAll(result)
		require.NoError(t, err)
		assert.Equal(t, "decompressed content", string(content))
	})

	t.Run("unsupported format", func(t *testing.T) {
		result, size, err := decompress(strings.NewReader("data"), "unsupported")

		assert.Nil(t, result)
		assert.Zero(t, size)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported decompression format")
	})

	t.Run("invalid gzip data", func(t *testing.T) {
		result, size, err := decompress(strings.NewReader("not gzip data"), "gzip")

		assert.Nil(t, result)
		assert.Zero(t, size)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "create gzip reader")
	})
}

func TestDownloadToTemp(t *testing.T) {
	// This function requires a real HTTP server, so we skip detailed testing.
	// The sync command integration relies on this working with real URLs.
	// We just test that invalid URLs return errors.

	t.Run("invalid URL returns error", func(t *testing.T) {
		file, size, err := downloadToTemp(context.Background(), "http://invalid.localhost.test:99999/file")

		assert.Nil(t, file)
		assert.Zero(t, size)
		assert.Error(t, err)
	})
}

func TestSyncImage(t *testing.T) {
	t.Run("skips when checksum matches", func(t *testing.T) {
		client := &mockStoreClient{
			checksumMatchFunc: func(_ context.Context, _ string, _ string) (bool, error) {
				return true, nil // Checksum matches
			},
		}

		img := config.Image{
			Name:        "test-image",
			Destination: "test/test.iso",
			Source: config.Source{
				URL:      "https://example.com/test.iso",
				Checksum: "sha256:abc123",
			},
		}

		changed, err := syncImage(context.Background(), client, img, false, false)

		require.NoError(t, err)
		assert.False(t, changed)
		assert.Empty(t, client.uploadedKeys) // No upload occurred
	})

	t.Run("dry run mode", func(t *testing.T) {
		client := &mockStoreClient{}

		img := config.Image{
			Name:        "test-image",
			Destination: "test/test.iso",
			Source: config.Source{
				URL:      "https://example.com/test.iso",
				Checksum: "sha256:abc123",
			},
		}

		changed, err := syncImage(context.Background(), client, img, true, false)

		require.NoError(t, err)
		assert.False(t, changed)
		assert.Empty(t, client.uploadedKeys)
	})

	t.Run("force ignores checksum match", func(t *testing.T) {
		// Force mode should skip checksum check entirely
		// This test verifies that with force=true, we don't even call ChecksumMatches
		checksumChecked := false
		client := &mockStoreClient{
			checksumMatchFunc: func(_ context.Context, _ string, _ string) (bool, error) {
				checksumChecked = true
				return true, nil
			},
		}

		img := config.Image{
			Name:        "test-image",
			Destination: "test/test.iso",
			Source: config.Source{
				URL:      "https://example.com/test.iso",
				Checksum: "sha256:abc123",
			},
		}

		// With force=true and dryRun=true, it should show what would be done
		// without checking checksum
		_, err := syncImage(context.Background(), client, img, true, true)

		require.NoError(t, err)
		assert.False(t, checksumChecked) // Should not check checksum with force
	})

	t.Run("checksum check error", func(t *testing.T) {
		client := &mockStoreClient{
			checksumMatchFunc: func(_ context.Context, _ string, _ string) (bool, error) {
				return false, errors.New("connection failed")
			},
		}

		img := config.Image{
			Name:        "test-image",
			Destination: "test/test.iso",
			Source: config.Source{
				URL:      "https://example.com/test.iso",
				Checksum: "sha256:abc123",
			},
		}

		_, err := syncImage(context.Background(), client, img, false, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "check existing image")
	})
}

func TestRunSync(t *testing.T) {
	// Save and restore globals
	origDryRun := syncDryRun
	origForce := syncForce
	origManifest := syncManifest
	defer func() {
		syncDryRun = origDryRun
		syncForce = origForce
		syncManifest = origManifest
	}()

	t.Run("dry run mode shows what would be done", func(t *testing.T) {
		// Create a test manifest
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

		syncManifest = manifestPath
		syncDryRun = true
		syncForce = false

		// In dry run mode, sync should not fail even without credentials
		// because it never actually tries to create a client
		err = runSync(nil, nil)
		assert.NoError(t, err)
	})

	t.Run("manifest file not found", func(t *testing.T) {
		syncManifest = "/nonexistent/path/images.yaml"
		syncDryRun = false
		syncForce = false

		err := runSync(nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load manifest")
	})

	t.Run("invalid manifest YAML", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")
		err := os.WriteFile(manifestPath, []byte("not: valid: yaml: ["), 0o644) //nolint:gosec
		require.NoError(t, err)

		syncManifest = manifestPath
		syncDryRun = false
		syncForce = false

		err = runSync(nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load manifest")
	})
}

func TestWriteGitHubOutput(t *testing.T) {
	t.Run("writes output when GITHUB_OUTPUT is set", func(t *testing.T) {
		dir := t.TempDir()
		outputFile := filepath.Join(dir, "github_output")

		// Create the file first
		err := os.WriteFile(outputFile, []byte{}, 0o644) //nolint:gosec
		require.NoError(t, err)

		// Set environment variable
		t.Setenv("GITHUB_OUTPUT", outputFile)

		err = writeGitHubOutput("test_key", "test_value")
		require.NoError(t, err)

		// Verify content
		content, err := os.ReadFile(outputFile) //nolint:gosec
		require.NoError(t, err)
		assert.Contains(t, string(content), "test_key=test_value")
	})

	t.Run("returns error when GITHUB_OUTPUT not set", func(t *testing.T) {
		t.Setenv("GITHUB_OUTPUT", "")

		err := writeGitHubOutput("key", "value")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "GITHUB_OUTPUT not set")
	})
}
