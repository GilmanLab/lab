package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		want    *E2Credentials
		wantErr string
	}{
		{
			name: "all variables set",
			envVars: map[string]string{
				"E2_ACCESS_KEY": "access123",
				"E2_SECRET_KEY": "secret456",
				"E2_ENDPOINT":   "https://e2.example.com",
				"E2_BUCKET":     "my-bucket",
			},
			want: &E2Credentials{
				AccessKey: "access123",
				SecretKey: "secret456",
				Endpoint:  "https://e2.example.com",
				Bucket:    "my-bucket",
			},
		},
		{
			name: "missing access key",
			envVars: map[string]string{
				"E2_SECRET_KEY": "secret456",
				"E2_ENDPOINT":   "https://e2.example.com",
				"E2_BUCKET":     "my-bucket",
			},
			wantErr: "E2_ACCESS_KEY",
		},
		{
			name: "missing secret key",
			envVars: map[string]string{
				"E2_ACCESS_KEY": "access123",
				"E2_ENDPOINT":   "https://e2.example.com",
				"E2_BUCKET":     "my-bucket",
			},
			wantErr: "E2_SECRET_KEY",
		},
		{
			name: "missing endpoint",
			envVars: map[string]string{
				"E2_ACCESS_KEY": "access123",
				"E2_SECRET_KEY": "secret456",
				"E2_BUCKET":     "my-bucket",
			},
			wantErr: "E2_ENDPOINT",
		},
		{
			name: "missing bucket",
			envVars: map[string]string{
				"E2_ACCESS_KEY": "access123",
				"E2_SECRET_KEY": "secret456",
				"E2_ENDPOINT":   "https://e2.example.com",
			},
			wantErr: "E2_BUCKET",
		},
		{
			name:    "all missing",
			envVars: map[string]string{},
			wantErr: "missing environment variables",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars first
			for _, key := range []string{EnvAccessKey, EnvSecretKey, EnvEndpoint, EnvBucket} {
				t.Setenv(key, "")
			}

			// Set test env vars
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			creds, err := FromEnv()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, creds)
		})
	}
}
