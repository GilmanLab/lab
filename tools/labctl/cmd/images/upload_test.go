package images

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GilmanLab/lab/tools/labctl/internal/store"
)

func TestComputeFileChecksum(t *testing.T) {
	t.Run("computes SHA256 checksum correctly", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		// Known content with known SHA256
		content := "hello world\n"
		err := os.WriteFile(path, []byte(content), 0o644) //nolint:gosec
		require.NoError(t, err)

		checksum, err := computeFileChecksum(path)

		require.NoError(t, err)
		// SHA256 of "hello world\n"
		assert.Equal(t, "sha256:a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447", checksum)
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		checksum, err := computeFileChecksum("/nonexistent/path/file.txt")

		assert.Empty(t, checksum)
		assert.Error(t, err)
	})

	t.Run("handles empty file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")

		err := os.WriteFile(path, []byte{}, 0o644) //nolint:gosec
		require.NoError(t, err)

		checksum, err := computeFileChecksum(path)

		require.NoError(t, err)
		// SHA256 of empty content
		assert.Equal(t, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", checksum)
	})

	t.Run("handles binary content", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "binary.bin")

		// Binary content
		content := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
		err := os.WriteFile(path, content, 0o644) //nolint:gosec
		require.NoError(t, err)

		checksum, err := computeFileChecksum(path)

		require.NoError(t, err)
		assert.Contains(t, checksum, "sha256:")
		// Verify it's a valid hex string
		assert.Len(t, checksum, len("sha256:")+64) // sha256 produces 64 hex chars
	})
}

func TestRunUploadWithClient(t *testing.T) {
	// Save and restore globals
	origSource := uploadSource
	origDest := uploadDestination
	origName := uploadName
	defer func() {
		uploadSource = origSource
		uploadDestination = origDest
		uploadName = origName
	}()

	t.Run("successful upload", func(t *testing.T) {
		dir := t.TempDir()
		sourcePath := filepath.Join(dir, "test.iso")
		content := []byte("test image content")
		err := os.WriteFile(sourcePath, content, 0o644) //nolint:gosec
		require.NoError(t, err)

		uploadSource = sourcePath
		uploadDestination = "test/test.iso"
		uploadName = "test-image"

		client := &mockStoreClient{}

		err = runUploadWithClient(context.Background(), client)

		require.NoError(t, err)
		assert.Len(t, client.uploadedKeys, 1)
		assert.Equal(t, "images/test/test.iso", client.uploadedKeys[0])
		assert.Len(t, client.putMetadataCalls, 1)
		assert.Equal(t, "test-image", client.putMetadataCalls[0].Name)
		assert.Contains(t, client.putMetadataCalls[0].Checksum, "sha256:")
	})

	t.Run("upload error", func(t *testing.T) {
		dir := t.TempDir()
		sourcePath := filepath.Join(dir, "test.iso")
		err := os.WriteFile(sourcePath, []byte("content"), 0o644) //nolint:gosec
		require.NoError(t, err)

		uploadSource = sourcePath
		uploadDestination = "test/test.iso"
		uploadName = ""

		client := &mockStoreClient{
			uploadFunc: func(_ context.Context, _ string, _ io.Reader, _ int64) error {
				return errors.New("upload failed")
			},
		}

		err = runUploadWithClient(context.Background(), client)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "upload image")
	})

	t.Run("metadata write error", func(t *testing.T) {
		dir := t.TempDir()
		sourcePath := filepath.Join(dir, "test.iso")
		err := os.WriteFile(sourcePath, []byte("content"), 0o644) //nolint:gosec
		require.NoError(t, err)

		uploadSource = sourcePath
		uploadDestination = "test/test.iso"
		uploadName = ""

		client := &mockStoreClient{
			putMetadataFunc: func(_ context.Context, _ string, _ *store.ImageMetadata) error {
				return errors.New("metadata write failed")
			},
		}

		err = runUploadWithClient(context.Background(), client)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write metadata")
	})

	t.Run("source file not found", func(t *testing.T) {
		uploadSource = "/nonexistent/path/test.iso"
		uploadDestination = "test/test.iso"
		uploadName = ""

		client := &mockStoreClient{}

		err := runUploadWithClient(context.Background(), client)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "stat source file")
	})

	t.Run("uses destination filename when name not provided", func(t *testing.T) {
		dir := t.TempDir()
		sourcePath := filepath.Join(dir, "test.iso")
		err := os.WriteFile(sourcePath, []byte("content"), 0o644) //nolint:gosec
		require.NoError(t, err)

		uploadSource = sourcePath
		uploadDestination = "images/my-image.iso"
		uploadName = "" // Not provided

		client := &mockStoreClient{}

		err = runUploadWithClient(context.Background(), client)

		require.NoError(t, err)
		assert.Len(t, client.putMetadataCalls, 1)
		assert.Equal(t, "my-image", client.putMetadataCalls[0].Name) // Extension removed
	})
}
