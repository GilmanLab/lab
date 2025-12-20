package images

import (
	"fmt"

	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload a local file to e2",
	Long: `Upload a local file to e2 storage.

The upload command is used by Packer workflows to upload built images.
It computes the SHA256 checksum and writes metadata JSON in the same
format as the sync command.`,
	RunE: runUpload,
}

var (
	uploadSource        string
	uploadDestination   string
	uploadCredentials   string
	uploadSOPSAgeKeyFile string
	uploadName          string
)

func init() {
	uploadCmd.Flags().StringVar(&uploadSource, "source", "", "Path to local file to upload (required)")
	uploadCmd.Flags().StringVar(&uploadDestination, "destination", "", "Destination path in e2 bucket (required)")
	uploadCmd.Flags().StringVar(&uploadCredentials, "credentials", "", "Path to SOPS-encrypted credentials file")
	uploadCmd.Flags().StringVar(&uploadSOPSAgeKeyFile, "sops-age-key-file", "", "Path to age private key")
	uploadCmd.Flags().StringVar(&uploadName, "name", "", "Image name for metadata (defaults to destination filename)")

	_ = uploadCmd.MarkFlagRequired("source")
	_ = uploadCmd.MarkFlagRequired("destination")
}

func runUpload(cmd *cobra.Command, args []string) error {
	// TODO(HOM-20): Implement upload command
	fmt.Println("upload command not yet implemented")
	return nil
}
