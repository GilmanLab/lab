package images

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPrune(t *testing.T) {
	// Save and restore globals
	origManifest := pruneManifest
	origDryRun := pruneDryRun
	defer func() {
		pruneManifest = origManifest
		pruneDryRun = origDryRun
	}()

	t.Run("manifest file not found", func(t *testing.T) {
		pruneManifest = "/nonexistent/path/images.yaml"
		pruneDryRun = true

		err := runPrune(nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load manifest")
	})

	t.Run("invalid manifest YAML", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")

		err := os.WriteFile(manifestPath, []byte("not: valid: yaml: ["), 0o644) //nolint:gosec
		require.NoError(t, err)

		pruneManifest = manifestPath
		pruneDryRun = true

		err = runPrune(nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load manifest")
	})

	t.Run("invalid manifest structure", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "images.yaml")

		// Valid YAML but missing required fields
		manifest := `apiVersion: wrong/version
kind: WrongKind
metadata:
  name: ""
spec:
  images: []
`
		err := os.WriteFile(manifestPath, []byte(manifest), 0o644) //nolint:gosec
		require.NoError(t, err)

		pruneManifest = manifestPath
		pruneDryRun = true

		err = runPrune(nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load manifest")
	})
}
