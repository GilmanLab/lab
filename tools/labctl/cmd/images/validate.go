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
	fmt.Printf("Validating manifest: %s\n", validateManifest)

	// Load manifest without validation to collect all errors
	manifest, err := config.LoadManifestRaw(validateManifest)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	fmt.Printf("Found %d image(s)\n\n", len(manifest.Spec.Images))

	// Collect all errors
	var allErrors []error

	// Get all manifest validation errors
	fmt.Println("Checking manifest structure...")
	manifestErrors := manifest.ValidateAll()
	for _, err := range manifestErrors {
		fmt.Printf("  ERROR: %v\n", err)
		allErrors = append(allErrors, err)
	}
	if len(manifestErrors) == 0 {
		fmt.Println("  OK")
	}
	fmt.Println()

	// Check all source URLs via HEAD requests (only for images with valid URLs)
	fmt.Println("Checking source URLs...")
	for _, img := range manifest.Spec.Images {
		// Skip URL check if the image doesn't have a valid URL
		if img.Source.URL == "" || img.Name == "" {
			continue
		}

		fmt.Printf("  %s... ", img.Name)

		if err := checkURL(context.Background(), client, img.Source.URL); err != nil {
			allErrors = append(allErrors, fmt.Errorf("image %q URL check: %w", img.Name, err))
			fmt.Println("FAILED")
			fmt.Printf("    Error: %v\n", err)
		} else {
			fmt.Println("OK")
		}
	}

	fmt.Println()
	if len(allErrors) > 0 {
		fmt.Printf("Validation failed with %d error(s)\n", len(allErrors))
		return fmt.Errorf("validation failed with %d error(s)", len(allErrors))
	}

	fmt.Println("All validations passed")
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
