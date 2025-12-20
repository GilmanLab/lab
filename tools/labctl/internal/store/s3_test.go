package store

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataKey(t *testing.T) {
	tests := []struct {
		name      string
		imagePath string
		want      string
	}{
		{
			name:      "simple path",
			imagePath: "image.iso",
			want:      "metadata/image.iso.json",
		},
		{
			name:      "nested path",
			imagePath: "vyos/vyos-1.5.iso",
			want:      "metadata/vyos/vyos-1.5.iso.json",
		},
		{
			name:      "deeply nested path",
			imagePath: "talos/v1.9.1/metal-amd64.raw",
			want:      "metadata/talos/v1.9.1/metal-amd64.raw.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MetadataKey(tt.imagePath))
		})
	}
}

func TestImageKey(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		want        string
	}{
		{
			name:        "simple path",
			destination: "image.iso",
			want:        "images/image.iso",
		},
		{
			name:        "nested path",
			destination: "vyos/vyos-1.5.iso",
			want:        "images/vyos/vyos-1.5.iso",
		},
		{
			name:        "deeply nested path",
			destination: "talos/v1.9.1/metal-amd64.raw",
			want:        "images/talos/v1.9.1/metal-amd64.raw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ImageKey(tt.destination))
		})
	}
}

func TestImageMetadata_JSON(t *testing.T) {
	t.Run("marshal and unmarshal HTTP source", func(t *testing.T) {
		metadata := ImageMetadata{
			Name:       "talos-1.9.1",
			Checksum:   "sha256:abc123",
			Size:       1234567890,
			UploadedAt: time.Date(2024, 12, 20, 10, 0, 0, 0, time.UTC),
			Source: SourceMetadata{
				Type: "http",
				URL:  "https://factory.talos.dev/image.raw",
			},
		}

		data, err := json.Marshal(metadata)
		require.NoError(t, err)

		var decoded ImageMetadata
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, metadata.Name, decoded.Name)
		assert.Equal(t, metadata.Checksum, decoded.Checksum)
		assert.Equal(t, metadata.Size, decoded.Size)
		assert.Equal(t, metadata.Source.Type, decoded.Source.Type)
		assert.Equal(t, metadata.Source.URL, decoded.Source.URL)
	})

	t.Run("marshal and unmarshal local source", func(t *testing.T) {
		metadata := ImageMetadata{
			Name:       "vyos-gateway",
			Checksum:   "sha256:def456",
			Size:       8589934592,
			UploadedAt: time.Date(2024, 12, 20, 12, 0, 0, 0, time.UTC),
			Source: SourceMetadata{
				Type: "local",
				Path: "infrastructure/network/vyos/packer/output/vyos-lab.raw",
			},
		}

		data, err := json.Marshal(metadata)
		require.NoError(t, err)

		var decoded ImageMetadata
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, metadata.Name, decoded.Name)
		assert.Equal(t, metadata.Checksum, decoded.Checksum)
		assert.Equal(t, metadata.Size, decoded.Size)
		assert.Equal(t, metadata.Source.Type, decoded.Source.Type)
		assert.Equal(t, metadata.Source.Path, decoded.Source.Path)
	})
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "NotFound error",
			err:  errors.New("NotFound: key does not exist"),
			want: true,
		},
		{
			name: "404 error",
			err:  errors.New("operation failed with status 404"),
			want: true,
		},
		{
			name: "NoSuchKey error",
			err:  errors.New("NoSuchKey: The specified key does not exist"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNotFoundError(tt.err))
		})
	}
}

func TestBytesReader(t *testing.T) {
	t.Run("reads all data", func(t *testing.T) {
		data := []byte("hello world")
		r := newBytesReader(data)

		buf := make([]byte, 5)
		n, err := r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "hello", string(buf))

		n, err = r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, " worl", string(buf))

		n, err = r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, "d", string(buf[:n]))
	})

	t.Run("returns EOF when exhausted", func(t *testing.T) {
		data := []byte("test")
		r := newBytesReader(data)

		buf := make([]byte, 10)
		n, err := r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 4, n)

		n, err = r.Read(buf)
		assert.Equal(t, 0, n)
		assert.ErrorIs(t, err, errEOF)
	})
}

// errEOF is used for comparison in tests
var errEOF = func() error {
	_, err := newBytesReader(nil).Read(make([]byte, 1))
	return err
}()
