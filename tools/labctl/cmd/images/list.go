package images

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List images stored in e2",
	Long:  "List all images currently stored in the e2 bucket with their metadata.",
	RunE:  runList,
}

var (
	listCredentials    string
	listSOPSAgeKeyFile string
)

func init() {
	listCmd.Flags().StringVar(&listCredentials, "credentials", "", "Path to SOPS-encrypted credentials file")
	listCmd.Flags().StringVar(&listSOPSAgeKeyFile, "sops-age-key-file", "", "Path to age private key")
}

func runList(cmd *cobra.Command, args []string) error {
	// TODO(HOM-20): Implement list command
	fmt.Println("list command not yet implemented")
	return nil
}
