package credentials

import (
	"fmt"
	"os"
)

// Environment variable names for e2 credentials.
const (
	EnvAccessKey = "E2_ACCESS_KEY"
	EnvSecretKey = "E2_SECRET_KEY"
	EnvEndpoint  = "E2_ENDPOINT"
	EnvBucket    = "E2_BUCKET"
)

// FromEnv resolves e2 credentials from environment variables.
// All four variables must be set: E2_ACCESS_KEY, E2_SECRET_KEY, E2_ENDPOINT, E2_BUCKET.
func FromEnv() (*E2Credentials, error) {
	accessKey := os.Getenv(EnvAccessKey)
	secretKey := os.Getenv(EnvSecretKey)
	endpoint := os.Getenv(EnvEndpoint)
	bucket := os.Getenv(EnvBucket)

	// Check if any are missing
	var missing []string
	if accessKey == "" {
		missing = append(missing, EnvAccessKey)
	}
	if secretKey == "" {
		missing = append(missing, EnvSecretKey)
	}
	if endpoint == "" {
		missing = append(missing, EnvEndpoint)
	}
	if bucket == "" {
		missing = append(missing, EnvBucket)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment variables: %v", missing)
	}

	return &E2Credentials{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Endpoint:  endpoint,
		Bucket:    bucket,
	}, nil
}
