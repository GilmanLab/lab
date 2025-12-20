package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockS3API is a mock implementation of s3API for testing.
type mockS3API struct {
	putObjectFunc     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	getObjectFunc     func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	headObjectFunc    func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	deleteObjectFunc  func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	listObjectsV2Func func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

func (m *mockS3API) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3API) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{}, nil
}

func (m *mockS3API) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, params, optFns...)
	}
	return &s3.HeadObjectOutput{}, nil
}

func (m *mockS3API) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.deleteObjectFunc != nil {
		return m.deleteObjectFunc(ctx, params, optFns...)
	}
	return &s3.DeleteObjectOutput{}, nil
}

func (m *mockS3API) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listObjectsV2Func != nil {
		return m.listObjectsV2Func(ctx, params, optFns...)
	}
	return &s3.ListObjectsV2Output{}, nil
}

// nopCloser wraps an io.Reader to implement io.ReadCloser.
type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func TestS3Client_Upload(t *testing.T) {
	t.Run("successful upload", func(t *testing.T) {
		mock := &mockS3API{
			putObjectFunc: func(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				assert.Equal(t, "test-bucket", aws.ToString(params.Bucket))
				assert.Equal(t, "images/test.iso", aws.ToString(params.Key))
				assert.Equal(t, int64(100), aws.ToInt64(params.ContentLength))
				return &s3.PutObjectOutput{}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		err := client.Upload(context.Background(), "images/test.iso", bytes.NewReader(make([]byte, 100)), 100)
		require.NoError(t, err)
	})

	t.Run("upload error", func(t *testing.T) {
		mock := &mockS3API{
			putObjectFunc: func(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				return nil, errors.New("network error")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		err := client.Upload(context.Background(), "images/test.iso", bytes.NewReader(nil), 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upload to s3://test-bucket/images/test.iso")
		assert.Contains(t, err.Error(), "network error")
	})
}

func TestS3Client_Download(t *testing.T) {
	t.Run("successful download", func(t *testing.T) {
		expectedData := []byte("file contents")
		mock := &mockS3API{
			getObjectFunc: func(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				assert.Equal(t, "test-bucket", aws.ToString(params.Bucket))
				assert.Equal(t, "images/test.iso", aws.ToString(params.Key))
				return &s3.GetObjectOutput{
					Body: nopCloser{bytes.NewReader(expectedData)},
				}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		body, err := client.Download(context.Background(), "images/test.iso")
		require.NoError(t, err)
		defer func() { _ = body.Close() }()

		data, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, expectedData, data)
	})

	t.Run("download error", func(t *testing.T) {
		mock := &mockS3API{
			getObjectFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return nil, errors.New("not found")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		_, err := client.Download(context.Background(), "images/missing.iso")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download from s3://test-bucket/images/missing.iso")
	})
}

func TestS3Client_Exists(t *testing.T) {
	t.Run("object exists", func(t *testing.T) {
		mock := &mockS3API{
			headObjectFunc: func(_ context.Context, params *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
				assert.Equal(t, "test-bucket", aws.ToString(params.Bucket))
				assert.Equal(t, "images/test.iso", aws.ToString(params.Key))
				return &s3.HeadObjectOutput{}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		exists, err := client.Exists(context.Background(), "images/test.iso")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("object not found", func(t *testing.T) {
		mock := &mockS3API{
			headObjectFunc: func(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
				return nil, errors.New("NotFound: object does not exist")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		exists, err := client.Exists(context.Background(), "images/missing.iso")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("other error", func(t *testing.T) {
		mock := &mockS3API{
			headObjectFunc: func(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
				return nil, errors.New("permission denied")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		_, err := client.Exists(context.Background(), "images/test.iso")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check existence of s3://test-bucket/images/test.iso")
	})
}

func TestS3Client_List(t *testing.T) {
	t.Run("list objects", func(t *testing.T) {
		mock := &mockS3API{
			listObjectsV2Func: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
				assert.Equal(t, "test-bucket", aws.ToString(params.Bucket))
				assert.Equal(t, "images/", aws.ToString(params.Prefix))
				return &s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: aws.String("images/a.iso")},
						{Key: aws.String("images/b.iso")},
					},
					IsTruncated: aws.Bool(false),
				}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		keys, err := client.List(context.Background(), "images/")
		require.NoError(t, err)
		assert.Equal(t, []string{"images/a.iso", "images/b.iso"}, keys)
	})

	t.Run("list with pagination", func(t *testing.T) {
		callCount := 0
		mock := &mockS3API{
			listObjectsV2Func: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
				callCount++
				if callCount == 1 {
					return &s3.ListObjectsV2Output{
						Contents: []types.Object{
							{Key: aws.String("images/a.iso")},
						},
						IsTruncated:           aws.Bool(true),
						NextContinuationToken: aws.String("token1"),
					}, nil
				}
				assert.Equal(t, "token1", aws.ToString(params.ContinuationToken))
				return &s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: aws.String("images/b.iso")},
					},
					IsTruncated: aws.Bool(false),
				}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		keys, err := client.List(context.Background(), "images/")
		require.NoError(t, err)
		assert.Equal(t, []string{"images/a.iso", "images/b.iso"}, keys)
		assert.Equal(t, 2, callCount)
	})

	t.Run("list error", func(t *testing.T) {
		mock := &mockS3API{
			listObjectsV2Func: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
				return nil, errors.New("access denied")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		_, err := client.List(context.Background(), "images/")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list objects in s3://test-bucket/images/")
	})
}

func TestS3Client_Delete(t *testing.T) {
	t.Run("successful delete", func(t *testing.T) {
		mock := &mockS3API{
			deleteObjectFunc: func(_ context.Context, params *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
				assert.Equal(t, "test-bucket", aws.ToString(params.Bucket))
				assert.Equal(t, "images/test.iso", aws.ToString(params.Key))
				return &s3.DeleteObjectOutput{}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		err := client.Delete(context.Background(), "images/test.iso")
		require.NoError(t, err)
	})

	t.Run("delete error", func(t *testing.T) {
		mock := &mockS3API{
			deleteObjectFunc: func(_ context.Context, _ *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
				return nil, errors.New("access denied")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		err := client.Delete(context.Background(), "images/test.iso")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete s3://test-bucket/images/test.iso")
	})
}

func TestS3Client_GetMetadata(t *testing.T) {
	t.Run("successful get metadata", func(t *testing.T) {
		metadata := ImageMetadata{
			Name:       "test-image",
			Checksum:   "sha256:abc123",
			Size:       1024,
			UploadedAt: time.Date(2024, 12, 20, 10, 0, 0, 0, time.UTC),
			Source:     SourceMetadata{Type: "http", URL: "https://example.com/image.iso"},
		}
		metadataJSON, _ := json.Marshal(metadata)

		mock := &mockS3API{
			getObjectFunc: func(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				assert.Equal(t, "metadata/images/test.iso.json", aws.ToString(params.Key))
				return &s3.GetObjectOutput{
					Body: nopCloser{bytes.NewReader(metadataJSON)},
				}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		result, err := client.GetMetadata(context.Background(), "images/test.iso")
		require.NoError(t, err)
		assert.Equal(t, "test-image", result.Name)
		assert.Equal(t, "sha256:abc123", result.Checksum)
		assert.Equal(t, int64(1024), result.Size)
	})

	t.Run("metadata not found", func(t *testing.T) {
		mock := &mockS3API{
			getObjectFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return nil, errors.New("NotFound")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		_, err := client.GetMetadata(context.Background(), "images/missing.iso")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		mock := &mockS3API{
			getObjectFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: nopCloser{bytes.NewReader([]byte("invalid json"))},
				}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		_, err := client.GetMetadata(context.Background(), "images/test.iso")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse metadata")
	})
}

func TestS3Client_PutMetadata(t *testing.T) {
	t.Run("successful put metadata", func(t *testing.T) {
		metadata := &ImageMetadata{
			Name:       "test-image",
			Checksum:   "sha256:abc123",
			Size:       1024,
			UploadedAt: time.Date(2024, 12, 20, 10, 0, 0, 0, time.UTC),
			Source:     SourceMetadata{Type: "http", URL: "https://example.com/image.iso"},
		}

		var uploadedData []byte
		mock := &mockS3API{
			putObjectFunc: func(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				assert.Equal(t, "metadata/images/test.iso.json", aws.ToString(params.Key))
				uploadedData, _ = io.ReadAll(params.Body)
				return &s3.PutObjectOutput{}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		err := client.PutMetadata(context.Background(), "images/test.iso", metadata)
		require.NoError(t, err)

		// Verify the uploaded JSON
		var decoded ImageMetadata
		err = json.Unmarshal(uploadedData, &decoded)
		require.NoError(t, err)
		assert.Equal(t, metadata.Name, decoded.Name)
		assert.Equal(t, metadata.Checksum, decoded.Checksum)
	})

	t.Run("put metadata error", func(t *testing.T) {
		mock := &mockS3API{
			putObjectFunc: func(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				return nil, errors.New("access denied")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		err := client.PutMetadata(context.Background(), "images/test.iso", &ImageMetadata{})
		require.Error(t, err)
	})
}

func TestS3Client_ChecksumMatches(t *testing.T) {
	t.Run("checksum matches", func(t *testing.T) {
		metadata := ImageMetadata{Checksum: "sha256:abc123"}
		metadataJSON, _ := json.Marshal(metadata)

		mock := &mockS3API{
			headObjectFunc: func(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
				return &s3.HeadObjectOutput{}, nil
			},
			getObjectFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: nopCloser{bytes.NewReader(metadataJSON)},
				}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		matches, err := client.ChecksumMatches(context.Background(), "images/test.iso", "sha256:abc123")
		require.NoError(t, err)
		assert.True(t, matches)
	})

	t.Run("checksum does not match", func(t *testing.T) {
		metadata := ImageMetadata{Checksum: "sha256:abc123"}
		metadataJSON, _ := json.Marshal(metadata)

		mock := &mockS3API{
			headObjectFunc: func(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
				return &s3.HeadObjectOutput{}, nil
			},
			getObjectFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: nopCloser{bytes.NewReader(metadataJSON)},
				}, nil
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		matches, err := client.ChecksumMatches(context.Background(), "images/test.iso", "sha256:different")
		require.NoError(t, err)
		assert.False(t, matches)
	})

	t.Run("metadata does not exist", func(t *testing.T) {
		mock := &mockS3API{
			headObjectFunc: func(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
				return nil, errors.New("NotFound")
			},
		}

		client := newS3ClientWithAPI(mock, "test-bucket")
		matches, err := client.ChecksumMatches(context.Background(), "images/test.iso", "sha256:abc123")
		require.NoError(t, err)
		assert.False(t, matches)
	})
}

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
