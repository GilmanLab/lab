package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromSOPS(t *testing.T) {
	t.Run("returns error when file not found", func(t *testing.T) {
		_, err := FromSOPS("/nonexistent/path/credentials.sops.yaml", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SOPS file not found")
	})

	// Note: Testing actual SOPS decryption would require:
	// 1. A valid SOPS-encrypted file
	// 2. The sops binary installed
	// 3. A valid age or GPG key
	// These are integration test requirements and should be tested separately.
}
