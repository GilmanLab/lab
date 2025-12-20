package credentials

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"
)

// FromSOPS decrypts a SOPS-encrypted YAML file and parses e2 credentials.
// If ageKeyFile is provided, it sets SOPS_AGE_KEY_FILE for the sops command.
func FromSOPS(sopsFile, ageKeyFile string) (*E2Credentials, error) {
	// Verify the SOPS file exists
	if _, err := os.Stat(sopsFile); err != nil {
		return nil, fmt.Errorf("SOPS file not found: %w", err)
	}

	// Build sops command
	args := []string{"--decrypt", sopsFile}
	cmd := exec.Command("sops", args...)

	// Set age key file environment if provided
	if ageKeyFile != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("SOPS_AGE_KEY_FILE=%s", ageKeyFile))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sops decrypt failed: %w: %s", err, stderr.String())
	}

	// Parse decrypted YAML
	var creds E2Credentials
	if err := yaml.Unmarshal(stdout.Bytes(), &creds); err != nil {
		return nil, fmt.Errorf("parse decrypted credentials: %w", err)
	}

	if err := creds.Validate(); err != nil {
		return nil, fmt.Errorf("validate credentials: %w", err)
	}

	return &creds, nil
}
