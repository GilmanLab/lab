// Package credentials provides credential resolution for e2 storage.
package credentials

import "fmt"

// E2Credentials holds credentials for iDrive e2 storage.
type E2Credentials struct {
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Endpoint  string `yaml:"endpoint"`
	Bucket    string `yaml:"bucket"`
}

// Validate checks that all required fields are present.
func (c *E2Credentials) Validate() error {
	if c.AccessKey == "" {
		return fmt.Errorf("access_key is required")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("secret_key is required")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if c.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	return nil
}

// ResolveOptions configures credential resolution.
type ResolveOptions struct {
	// SOPSFile is the path to a SOPS-encrypted credentials file.
	SOPSFile string

	// AgeKeyFile is the path to the age private key for SOPS decryption.
	AgeKeyFile string
}

// Resolve attempts to resolve e2 credentials using the following order:
// 1. Environment variables (if all are present)
// 2. SOPS file (if specified in options)
func Resolve(opts ResolveOptions) (*E2Credentials, error) {
	// Try environment variables first
	creds, err := FromEnv()
	if err == nil {
		return creds, nil
	}

	// Try SOPS file if specified
	if opts.SOPSFile != "" {
		creds, err := FromSOPS(opts.SOPSFile, opts.AgeKeyFile)
		if err != nil {
			return nil, fmt.Errorf("resolve from SOPS: %w", err)
		}
		return creds, nil
	}

	return nil, fmt.Errorf("no credentials found: environment variables incomplete and no SOPS file specified")
}
