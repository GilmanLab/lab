package images

import (
	"fmt"

	"github.com/spf13/cobra"
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
	// TODO(HOM-20): Implement prune command
	fmt.Println("prune command not yet implemented")
	return nil
}
