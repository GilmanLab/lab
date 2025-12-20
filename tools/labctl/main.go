// Package main provides the entry point for the labctl CLI tool.
package main

import (
	"os"

	"github.com/GilmanLab/lab/tools/labctl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
