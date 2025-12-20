package images

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GilmanLab/lab/tools/labctl/internal/config"
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

func TestRunPruneWithClient(t *testing.T) {
	t.Run("no orphaned images", func(t *testing.T) {
		manifest := &config.ImageManifest{
			Spec: config.Spec{
				Images: []config.Image{
					{Name: "vyos", Destination: "vyos/vyos.iso"},
					{Name: "talos", Destination: "talos/talos.iso"},
				},
			},
		}

		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{"images/vyos/vyos.iso", "images/talos/talos.iso"}, nil
			},
		}

		err := runPruneWithClient(context.Background(), client, manifest, true)

		require.NoError(t, err)
		assert.Empty(t, client.deletedKeys)
	})

	t.Run("finds and removes orphaned images", func(t *testing.T) {
		manifest := &config.ImageManifest{
			Spec: config.Spec{
				Images: []config.Image{
					{Name: "vyos", Destination: "vyos/vyos.iso"},
				},
			},
		}

		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				// talos is orphaned - not in manifest
				return []string{"images/vyos/vyos.iso", "images/talos/talos.iso"}, nil
			},
		}

		err := runPruneWithClient(context.Background(), client, manifest, false)

		require.NoError(t, err)
		// Should delete the orphaned image and its metadata
		assert.Contains(t, client.deletedKeys, "images/talos/talos.iso")
		assert.Contains(t, client.deletedKeys, "metadata/talos/talos.iso.json")
	})

	t.Run("dry run mode does not delete", func(t *testing.T) {
		manifest := &config.ImageManifest{
			Spec: config.Spec{
				Images: []config.Image{
					{Name: "vyos", Destination: "vyos/vyos.iso"},
				},
			},
		}

		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{"images/vyos/vyos.iso", "images/orphan/orphan.iso"}, nil
			},
		}

		err := runPruneWithClient(context.Background(), client, manifest, true)

		require.NoError(t, err)
		// Dry run should not delete anything
		assert.Empty(t, client.deletedKeys)
	})

	t.Run("list error", func(t *testing.T) {
		manifest := &config.ImageManifest{}

		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return nil, errors.New("connection failed")
			},
		}

		err := runPruneWithClient(context.Background(), client, manifest, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "list images")
	})

	t.Run("delete error", func(t *testing.T) {
		manifest := &config.ImageManifest{
			Spec: config.Spec{
				Images: []config.Image{}, // Empty - all images are orphaned
			},
		}

		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{"images/orphan.iso"}, nil
			},
			deleteFunc: func(_ context.Context, _ string) error {
				return errors.New("delete failed")
			},
		}

		err := runPruneWithClient(context.Background(), client, manifest, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delete image")
	})

	t.Run("skips directory entries", func(t *testing.T) {
		manifest := &config.ImageManifest{
			Spec: config.Spec{
				Images: []config.Image{},
			},
		}

		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{"images/", "images/test/"}, nil
			},
		}

		err := runPruneWithClient(context.Background(), client, manifest, false)

		require.NoError(t, err)
		// Should not attempt to delete directories
		assert.Empty(t, client.deletedKeys)
	})

	t.Run("handles multiple orphaned images", func(t *testing.T) {
		manifest := &config.ImageManifest{
			Spec: config.Spec{
				Images: []config.Image{
					{Name: "keep", Destination: "keep/keep.iso"},
				},
			},
		}

		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{
					"images/keep/keep.iso",
					"images/orphan1/orphan1.iso",
					"images/orphan2/orphan2.iso",
				}, nil
			},
		}

		err := runPruneWithClient(context.Background(), client, manifest, false)

		require.NoError(t, err)
		// Should delete both orphaned images
		assert.Contains(t, client.deletedKeys, "images/orphan1/orphan1.iso")
		assert.Contains(t, client.deletedKeys, "images/orphan2/orphan2.iso")
		// Should not delete the kept image
		for _, key := range client.deletedKeys {
			assert.NotContains(t, key, "keep/keep.iso")
		}
	})
}
