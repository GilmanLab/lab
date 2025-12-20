package images

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/GilmanLab/lab/tools/labctl/internal/config"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the image manifest",
	Long: `Validate manifest syntax, check source URLs, and verify updateFile regex patterns.

The validate command performs a dry-run validation of the image manifest,
checking that all URLs are reachable (via HEAD requests) and that regex
patterns in updateFile sections compile successfully.`,
	RunE: runValidate,
}

var validateManifest string

func init() {
	validateCmd.Flags().StringVar(&validateManifest, "manifest", "./images/images.yaml", "Path to images.yaml")
}

// httpClient defines the HTTP operations used for URL validation.
// This interface enables mocking for unit tests.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// defaultHTTPClient is the default HTTP client used for URL validation.
var defaultHTTPClient httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

func runValidate(_ *cobra.Command, _ []string) error {
	return runValidateWithClient(defaultHTTPClient)
}

func runValidateWithClient(client httpClient) error {
	// Load and parse manifest (validates YAML syntax, regexes, and HTTPS requirement)
	manifest, err := config.LoadManifest(validateManifest)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	fmt.Printf("Validating manifest: %s\n", validateManifest)
	fmt.Printf("Found %d image(s)\n\n", len(manifest.Spec.Images))

	// Check all source URLs via HEAD requests
	var errors []error
	for _, img := range manifest.Spec.Images {
		fmt.Printf("Checking %s... ", img.Name)

		if err := checkURL(context.Background(), client, img.Source.URL); err != nil {
			errors = append(errors, fmt.Errorf("image %q: %w", img.Name, err))
			fmt.Println("FAILED")
			fmt.Printf("  Error: %v\n", err)
		} else {
			fmt.Println("OK")
		}
	}

	if len(errors) > 0 {
		fmt.Printf("\nValidation failed with %d error(s)\n", len(errors))
		return fmt.Errorf("validation failed with %d error(s)", len(errors))
	}

	fmt.Println("\nAll validations passed")
	return nil
}

func checkURL(ctx context.Context, client httpClient, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set a user agent to avoid being blocked by some servers
	req.Header.Set("User-Agent", "labctl/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HEAD request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Accept 2xx and 3xx status codes as success
	// Some servers return 302/301 for downloads
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}
