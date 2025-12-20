package images

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
