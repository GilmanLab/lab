package images

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/GilmanLab/lab/tools/labctl/internal/config"
	"github.com/GilmanLab/lab/tools/labctl/internal/credentials"
	"github.com/GilmanLab/lab/tools/labctl/internal/store"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove orphaned images from e2",
	Long: `Remove images from e2 that are not in the manifest.

The prune command compares images in e2 storage against the manifest and
removes any that are no longer referenced. This is a manual-only operation
and is not run automatically.`,
	RunE: runPrune,
}

var (
	pruneManifest       string
	pruneCredentials    string
	pruneSOPSAgeKeyFile string
	pruneDryRun         bool
)

func init() {
	pruneCmd.Flags().StringVar(&pruneManifest, "manifest", "./images/images.yaml", "Path to images.yaml")
	pruneCmd.Flags().StringVar(&pruneCredentials, "credentials", "", "Path to SOPS-encrypted credentials file")
	pruneCmd.Flags().StringVar(&pruneSOPSAgeKeyFile, "sops-age-key-file", "", "Path to age private key")
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "Show what would be removed")
}

func runPrune(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	// Load manifest
	manifest, err := config.LoadManifest(pruneManifest)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Resolve credentials
	creds, err := credentials.Resolve(credentials.ResolveOptions{
		SOPSFile:   pruneCredentials,
		AgeKeyFile: pruneSOPSAgeKeyFile,
	})
	if err != nil {
		return fmt.Errorf("resolve credentials: %w", err)
	}

	// Create S3 client
	client, err := store.NewS3Client(creds, store.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create S3 client: %w", err)
	}

	return runPruneWithClient(ctx, client, manifest, pruneDryRun)
}

// runPruneWithClient performs the prune operation using the provided store client.
// This function enables dependency injection for testing.
func runPruneWithClient(ctx context.Context, client store.Client, manifest *config.ImageManifest, dryRun bool) error {
	// Build set of expected destinations from manifest
	expected := make(map[string]bool)
	for _, img := range manifest.Spec.Images {
		expected[img.Destination] = true
	}

	// List all images in storage
	keys, err := client.List(ctx, "images/")
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}

	// Find orphaned images
	var orphaned []string
	for _, key := range keys {
		// Skip directories
		if strings.HasSuffix(key, "/") {
			continue
		}

		// Convert key to destination path
		destPath := strings.TrimPrefix(key, "images/")

		if !expected[destPath] {
			orphaned = append(orphaned, destPath)
		}
	}

	if len(orphaned) == 0 {
		fmt.Println("No orphaned images found")
		return nil
	}

	// Report and optionally delete orphaned images
	fmt.Printf("Found %d orphaned image(s):\n", len(orphaned))
	for _, dest := range orphaned {
		if dryRun {
			fmt.Printf("  Would remove: %s\n", dest)
		} else {
			fmt.Printf("  Removing: %s\n", dest)

			// Delete image
			imageKey := store.ImageKey(dest)
			if err := client.Delete(ctx, imageKey); err != nil {
				return fmt.Errorf("delete image %s: %w", dest, err)
			}

			// Delete metadata
			metadataKey := store.MetadataKey(dest)
			// Ignore metadata deletion errors (might not exist)
			_ = client.Delete(ctx, metadataKey)
		}
	}

	if dryRun {
		fmt.Printf("\nDry run: no changes made\n")
	} else {
		fmt.Printf("\nRemoved %d orphaned image(s)\n", len(orphaned))
	}

	return nil
}
