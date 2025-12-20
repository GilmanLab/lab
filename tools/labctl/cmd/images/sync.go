package images

import (
	"fmt"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync images to e2 storage",
	Long: `Download source images, upload to e2, update files, and create PR if needed.

The sync command reads the image manifest, downloads any new or updated images,
uploads them to e2 storage, and optionally updates file references to trigger
downstream builds.`,
	RunE: runSync,
}

var (
	syncManifest       string
	syncCredentials    string
	syncSOPSAgeKeyFile string
	syncDryRun         bool
	syncForce          bool
)

func init() {
	syncCmd.Flags().StringVar(&syncManifest, "manifest", "./images/images.yaml", "Path to images.yaml")
	syncCmd.Flags().StringVar(&syncCredentials, "credentials", "", "Path to SOPS-encrypted credentials file")
	syncCmd.Flags().StringVar(&syncSOPSAgeKeyFile, "sops-age-key-file", "", "Path to age private key for SOPS decryption")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Show what would be done without executing")
	syncCmd.Flags().BoolVar(&syncForce, "force", false, "Force re-upload even if checksums match")
}

func runSync(cmd *cobra.Command, args []string) error {
	// TODO(HOM-20): Implement sync command
	fmt.Println("sync command not yet implemented")
	return nil
}
