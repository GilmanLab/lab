// Package cmd provides the CLI commands for labctl.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/GilmanLab/lab/tools/labctl/cmd/images"
)

var rootCmd = &cobra.Command{
	Use:   "labctl",
	Short: "Lab control CLI for managing infrastructure",
	Long:  "labctl is a CLI tool for managing lab infrastructure including images, configurations, and deployments.",
}

func init() {
	rootCmd.AddCommand(images.Cmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
