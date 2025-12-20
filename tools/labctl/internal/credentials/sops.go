package credentials

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"
)

// commandRunner executes a command and returns stdout, stderr, and error.
// This is a variable to allow mocking in tests.
var commandRunner = func(name string, args []string, env []string) (stdout, stderr []byte, err error) {
	cmd := exec.Command(name, args...) //nolint:gosec // G204: sops execution is intended behavior
	if len(env) > 0 {
		cmd.Env = env
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

// FromSOPS decrypts a SOPS-encrypted YAML file and parses e2 credentials.
// If ageKeyFile is provided, it sets SOPS_AGE_KEY_FILE for the sops command.
func FromSOPS(sopsFile, ageKeyFile string) (*E2Credentials, error) {
	// Verify the SOPS file exists
	if _, err := os.Stat(sopsFile); err != nil {
		return nil, fmt.Errorf("SOPS file not found: %w", err)
	}

	// Build sops command
	args := []string{"--decrypt", sopsFile}
	var env []string
	if ageKeyFile != "" {
		env = append(os.Environ(), fmt.Sprintf("SOPS_AGE_KEY_FILE=%s", ageKeyFile))
	}

	stdout, stderr, err := commandRunner("sops", args, env)
	if err != nil {
		return nil, fmt.Errorf("sops decrypt failed: %w: %s", err, string(stderr))
	}

	// Parse decrypted YAML
	var creds E2Credentials
	if err := yaml.Unmarshal(stdout, &creds); err != nil {
		return nil, fmt.Errorf("parse decrypted credentials: %w", err)
	}

	if err := creds.Validate(); err != nil {
		return nil, fmt.Errorf("validate credentials: %w", err)
	}

	return &creds, nil
}
