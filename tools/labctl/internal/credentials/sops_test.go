package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromSOPS(t *testing.T) {
	// Save original command runner and restore after tests
	originalRunner := commandRunner
	t.Cleanup(func() {
		commandRunner = originalRunner
	})

	t.Run("successful decryption", func(t *testing.T) {
		// Create a temporary file to satisfy the file existence check
		dir := t.TempDir()
		sopsFile := filepath.Join(dir, "credentials.sops.yaml")
		err := os.WriteFile(sopsFile, []byte("encrypted content"), 0o600)
		require.NoError(t, err)

		// Mock the command runner to return valid credentials
		commandRunner = func(name string, args []string, _ []string) ([]byte, []byte, error) {
			assert.Equal(t, "sops", name)
			assert.Equal(t, []string{"--decrypt", sopsFile}, args)

			yamlOutput := `access_key: test-access-key
secret_key: test-secret-key
endpoint: https://e2.example.com
bucket: test-bucket
`
			return []byte(yamlOutput), nil, nil
		}

		creds, err := FromSOPS(sopsFile, "")
		require.NoError(t, err)
		assert.Equal(t, "test-access-key", creds.AccessKey)
		assert.Equal(t, "test-secret-key", creds.SecretKey)
		assert.Equal(t, "https://e2.example.com", creds.Endpoint)
		assert.Equal(t, "test-bucket", creds.Bucket)
	})

	t.Run("successful decryption with age key file", func(t *testing.T) {
		dir := t.TempDir()
		sopsFile := filepath.Join(dir, "credentials.sops.yaml")
		err := os.WriteFile(sopsFile, []byte("encrypted content"), 0o600)
		require.NoError(t, err)

		ageKeyFile := "/path/to/age-key.txt"

		commandRunner = func(_ string, _ []string, env []string) ([]byte, []byte, error) {
			// Verify age key file is in environment
			found := false
			for _, e := range env {
				if strings.HasPrefix(e, "SOPS_AGE_KEY_FILE=") {
					assert.Equal(t, "SOPS_AGE_KEY_FILE="+ageKeyFile, e)
					found = true
					break
				}
			}
			assert.True(t, found, "SOPS_AGE_KEY_FILE should be set in environment")

			yamlOutput := `access_key: test-access-key
secret_key: test-secret-key
endpoint: https://e2.example.com
bucket: test-bucket
`
			return []byte(yamlOutput), nil, nil
		}

		creds, err := FromSOPS(sopsFile, ageKeyFile)
		require.NoError(t, err)
		assert.Equal(t, "test-access-key", creds.AccessKey)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := FromSOPS("/nonexistent/path/credentials.sops.yaml", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SOPS file not found")
	})

	t.Run("sops command fails", func(t *testing.T) {
		dir := t.TempDir()
		sopsFile := filepath.Join(dir, "credentials.sops.yaml")
		err := os.WriteFile(sopsFile, []byte("encrypted content"), 0o600)
		require.NoError(t, err)

		commandRunner = func(_ string, _ []string, _ []string) ([]byte, []byte, error) {
			return nil, []byte("error: could not decrypt"), errors.New("exit status 1")
		}

		_, err = FromSOPS(sopsFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sops decrypt failed")
		assert.Contains(t, err.Error(), "could not decrypt")
	})

	t.Run("invalid yaml output", func(t *testing.T) {
		dir := t.TempDir()
		sopsFile := filepath.Join(dir, "credentials.sops.yaml")
		err := os.WriteFile(sopsFile, []byte("encrypted content"), 0o600)
		require.NoError(t, err)

		commandRunner = func(_ string, _ []string, _ []string) ([]byte, []byte, error) {
			return []byte("invalid: yaml: content: ["), nil, nil
		}

		_, err = FromSOPS(sopsFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse decrypted credentials")
	})

	t.Run("missing required fields in credentials", func(t *testing.T) {
		dir := t.TempDir()
		sopsFile := filepath.Join(dir, "credentials.sops.yaml")
		err := os.WriteFile(sopsFile, []byte("encrypted content"), 0o600)
		require.NoError(t, err)

		commandRunner = func(_ string, _ []string, _ []string) ([]byte, []byte, error) {
			// Missing access_key
			yamlOutput := `secret_key: test-secret-key
endpoint: https://e2.example.com
bucket: test-bucket
`
			return []byte(yamlOutput), nil, nil
		}

		_, err = FromSOPS(sopsFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate credentials")
		assert.Contains(t, err.Error(), "access_key is required")
	})
}
