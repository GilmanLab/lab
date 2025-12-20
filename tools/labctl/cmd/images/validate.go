package images

import (
	"fmt"

	"github.com/spf13/cobra"
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

func runValidate(_ *cobra.Command, _ []string) error {
	// TODO(HOM-20): Implement validate command
	fmt.Println("validate command not yet implemented")
	return nil
}
