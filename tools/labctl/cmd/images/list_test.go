package images

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GilmanLab/lab/tools/labctl/internal/store"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "bytes",
			bytes:    500,
			expected: "500 B",
		},
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
		{
			name:     "kilobytes",
			bytes:    1024,
			expected: "1.00 KB",
		},
		{
			name:     "kilobytes with decimal",
			bytes:    1536,
			expected: "1.50 KB",
		},
		{
			name:     "megabytes",
			bytes:    1024 * 1024,
			expected: "1.00 MB",
		},
		{
			name:     "megabytes with decimal",
			bytes:    1024*1024*10 + 1024*512,
			expected: "10.50 MB",
		},
		{
			name:     "gigabytes",
			bytes:    1024 * 1024 * 1024,
			expected: "1.00 GB",
		},
		{
			name:     "gigabytes with decimal",
			bytes:    1024*1024*1024*2 + 1024*1024*512,
			expected: "2.50 GB",
		},
		{
			name:     "large gigabytes",
			bytes:    1024 * 1024 * 1024 * 50,
			expected: "50.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRunListWithClient(t *testing.T) {
	t.Run("lists images with metadata", func(t *testing.T) {
		uploadTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		client := &mockStoreClient{
			listFunc: func(_ context.Context, prefix string) ([]string, error) {
				assert.Equal(t, "images/", prefix)
				return []string{"images/vyos/vyos-1.5.iso", "images/talos/talos-1.6.iso"}, nil
			},
			getMetadataFunc: func(_ context.Context, imagePath string) (*store.ImageMetadata, error) {
				if imagePath == "vyos/vyos-1.5.iso" {
					return &store.ImageMetadata{
						Name:       "vyos",
						Checksum:   "sha256:abc123def456789012345678901234567890",
						Size:       1024 * 1024 * 500,
						UploadedAt: uploadTime,
					}, nil
				}
				return &store.ImageMetadata{
					Name:       "talos",
					Checksum:   "sha256:xyz789",
					Size:       1024 * 1024 * 200,
					UploadedAt: uploadTime,
				}, nil
			},
		}

		var buf bytes.Buffer
		err := runListWithClient(context.Background(), client, &buf)

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "vyos")
		assert.Contains(t, output, "talos")
		assert.Contains(t, output, "500.00 MB")
		assert.Contains(t, output, "200.00 MB")
		assert.Contains(t, output, "2024-01-15")
	})

	t.Run("no images found", func(t *testing.T) {
		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{}, nil
			},
		}

		var buf bytes.Buffer
		err := runListWithClient(context.Background(), client, &buf)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No images found")
	})

	t.Run("list error", func(t *testing.T) {
		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return nil, errors.New("connection failed")
			},
		}

		var buf bytes.Buffer
		err := runListWithClient(context.Background(), client, &buf)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "list images")
	})

	t.Run("handles missing metadata gracefully", func(t *testing.T) {
		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{"images/test/image.iso"}, nil
			},
			getMetadataFunc: func(_ context.Context, _ string) (*store.ImageMetadata, error) {
				return nil, errors.New("metadata not found")
			},
		}

		var buf bytes.Buffer
		err := runListWithClient(context.Background(), client, &buf)

		require.NoError(t, err)
		output := buf.String()
		// Should show dash for missing metadata (tabwriter converts tabs to spaces)
		assert.Contains(t, output, "-")
		assert.Contains(t, output, "test/image.iso")
	})

	t.Run("skips directory entries", func(t *testing.T) {
		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{"images/", "images/test/", "images/test/image.iso"}, nil
			},
			getMetadataFunc: func(_ context.Context, _ string) (*store.ImageMetadata, error) {
				return &store.ImageMetadata{
					Name:       "test",
					Checksum:   "sha256:abc",
					Size:       1024,
					UploadedAt: time.Now(),
				}, nil
			},
		}

		var buf bytes.Buffer
		err := runListWithClient(context.Background(), client, &buf)

		require.NoError(t, err)
		output := buf.String()
		// Should only show the actual image, not directories
		lines := strings.Split(output, "\n")
		dataLines := 0
		for _, line := range lines {
			if strings.HasPrefix(line, "test") {
				dataLines++
			}
		}
		assert.Equal(t, 1, dataLines)
	})

	t.Run("truncates long checksums", func(t *testing.T) {
		longChecksum := "sha256:abcdef123456789012345678901234567890abcdef123456789012345678901234"
		client := &mockStoreClient{
			listFunc: func(_ context.Context, _ string) ([]string, error) {
				return []string{"images/test.iso"}, nil
			},
			getMetadataFunc: func(_ context.Context, _ string) (*store.ImageMetadata, error) {
				return &store.ImageMetadata{
					Name:       "test",
					Checksum:   longChecksum,
					Size:       1024,
					UploadedAt: time.Now(),
				}, nil
			},
		}

		var buf bytes.Buffer
		err := runListWithClient(context.Background(), client, &buf)

		require.NoError(t, err)
		output := buf.String()
		// Should contain truncated checksum with ... (first 20 chars + ...)
		assert.Contains(t, output, "sha256:abcdef1234567...")
		// Should not contain the full checksum
		assert.NotContains(t, output, longChecksum)
	})
}
