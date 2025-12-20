package images

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/GilmanLab/lab/tools/labctl/internal/credentials"
	"github.com/GilmanLab/lab/tools/labctl/internal/store"
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

func runList(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	// Resolve credentials
	creds, err := credentials.Resolve(credentials.ResolveOptions{
		SOPSFile:   listCredentials,
		AgeKeyFile: listSOPSAgeKeyFile,
	})
	if err != nil {
		return fmt.Errorf("resolve credentials: %w", err)
	}

	// Create S3 client
	client, err := store.NewS3Client(creds, store.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create S3 client: %w", err)
	}

	// List all images
	keys, err := client.List(ctx, "images/")
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("No images found")
		return nil
	}

	// Create tabwriter for formatted output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPATH\tSIZE\tCHECKSUM\tUPLOADED")
	_, _ = fmt.Fprintln(w, "----\t----\t----\t--------\t--------")

	for _, key := range keys {
		// Skip directories (keys ending with /)
		if strings.HasSuffix(key, "/") {
			continue
		}

		// Convert image key to destination path
		// images/vyos/vyos-1.5.iso -> vyos/vyos-1.5.iso
		destPath := strings.TrimPrefix(key, "images/")

		// Try to get metadata
		metadata, err := client.GetMetadata(ctx, destPath)
		if err != nil {
			// Metadata might not exist for all images
			_, _ = fmt.Fprintf(w, "-\t%s\t-\t-\t-\n", destPath)
			continue
		}

		// Format size
		sizeStr := formatSize(metadata.Size)

		// Truncate checksum for display
		checksumStr := metadata.Checksum
		if len(checksumStr) > 20 {
			checksumStr = checksumStr[:20] + "..."
		}

		// Format upload time
		uploadedStr := metadata.UploadedAt.Format("2006-01-02 15:04")

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			metadata.Name,
			destPath,
			sizeStr,
			checksumStr,
			uploadedStr,
		)
	}

	return w.Flush()
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/gb)
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/mb)
	case bytes >= kb:
		return fmt.Sprintf("%.2f KB", float64(bytes)/kb)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
