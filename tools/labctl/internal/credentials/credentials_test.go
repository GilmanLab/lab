package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2Credentials_Validate(t *testing.T) {
	tests := []struct {
		name    string
		creds   E2Credentials
		wantErr string
	}{
		{
			name: "valid credentials",
			creds: E2Credentials{
				AccessKey: "access123",
				SecretKey: "secret456",
				Endpoint:  "https://e2.example.com",
				Bucket:    "my-bucket",
			},
		},
		{
			name: "missing access key",
			creds: E2Credentials{
				SecretKey: "secret456",
				Endpoint:  "https://e2.example.com",
				Bucket:    "my-bucket",
			},
			wantErr: "access_key is required",
		},
		{
			name: "missing secret key",
			creds: E2Credentials{
				AccessKey: "access123",
				Endpoint:  "https://e2.example.com",
				Bucket:    "my-bucket",
			},
			wantErr: "secret_key is required",
		},
		{
			name: "missing endpoint",
			creds: E2Credentials{
				AccessKey: "access123",
				SecretKey: "secret456",
				Bucket:    "my-bucket",
			},
			wantErr: "endpoint is required",
		},
		{
			name: "missing bucket",
			creds: E2Credentials{
				AccessKey: "access123",
				SecretKey: "secret456",
				Endpoint:  "https://e2.example.com",
			},
			wantErr: "bucket is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.creds.Validate()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestResolve(t *testing.T) {
	t.Run("resolves from environment variables", func(t *testing.T) {
		t.Setenv("E2_ACCESS_KEY", "access123")
		t.Setenv("E2_SECRET_KEY", "secret456")
		t.Setenv("E2_ENDPOINT", "https://e2.example.com")
		t.Setenv("E2_BUCKET", "my-bucket")

		creds, err := Resolve(ResolveOptions{})
		require.NoError(t, err)
		assert.Equal(t, "access123", creds.AccessKey)
		assert.Equal(t, "secret456", creds.SecretKey)
		assert.Equal(t, "https://e2.example.com", creds.Endpoint)
		assert.Equal(t, "my-bucket", creds.Bucket)
	})

	t.Run("returns error when no credentials available", func(t *testing.T) {
		// Clear all env vars
		for _, key := range []string{EnvAccessKey, EnvSecretKey, EnvEndpoint, EnvBucket} {
			t.Setenv(key, "")
		}

		_, err := Resolve(ResolveOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no credentials found")
	})

	t.Run("returns error when SOPS file not found", func(t *testing.T) {
		// Clear all env vars
		for _, key := range []string{EnvAccessKey, EnvSecretKey, EnvEndpoint, EnvBucket} {
			t.Setenv(key, "")
		}

		_, err := Resolve(ResolveOptions{
			SOPSFile: "/nonexistent/path/credentials.sops.yaml",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SOPS file not found")
	})
}
