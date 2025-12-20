package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/GilmanLab/lab/tools/labctl/internal/credentials"
	"github.com/GilmanLab/lab/tools/labctl/internal/store"
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
	uploadSource         string
	uploadDestination    string
	uploadCredentials    string
	uploadSOPSAgeKeyFile string
	uploadName           string
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

func runUpload(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	// Resolve credentials
	creds, err := credentials.Resolve(credentials.ResolveOptions{
		SOPSFile:   uploadCredentials,
		AgeKeyFile: uploadSOPSAgeKeyFile,
	})
	if err != nil {
		return fmt.Errorf("resolve credentials: %w", err)
	}

	// Create S3 client
	client, err := store.NewS3Client(creds, store.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create S3 client: %w", err)
	}

	return runUploadWithClient(ctx, client)
}

// runUploadWithClient performs the upload using the provided store client.
// This function enables dependency injection for testing.
func runUploadWithClient(ctx context.Context, client store.Client) error {
	// Get file info
	info, err := os.Stat(uploadSource)
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}

	// Compute checksum
	fmt.Printf("Computing checksum for %s...\n", uploadSource)
	checksum, err := computeFileChecksum(uploadSource)
	if err != nil {
		return fmt.Errorf("compute checksum: %w", err)
	}
	fmt.Printf("Checksum: %s\n", checksum)

	// Open file for upload
	file, err := os.Open(uploadSource) //nolint:gosec // G304: Path is provided by user
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Upload to e2
	imageKey := store.ImageKey(uploadDestination)
	fmt.Printf("Uploading to %s...\n", imageKey)
	if err := client.Upload(ctx, imageKey, file, info.Size()); err != nil {
		return fmt.Errorf("upload image: %w", err)
	}

	// Determine image name
	imageName := uploadName
	if imageName == "" {
		imageName = filepath.Base(uploadDestination)
		// Remove extension if present
		if ext := filepath.Ext(imageName); ext != "" {
			imageName = imageName[:len(imageName)-len(ext)]
		}
	}

	// Write metadata
	metadata := &store.ImageMetadata{
		Name:       imageName,
		Checksum:   checksum,
		Size:       info.Size(),
		UploadedAt: time.Now().UTC(),
		Source: store.SourceMetadata{
			Type: "local",
			Path: uploadSource,
		},
	}

	fmt.Printf("Writing metadata...\n")
	if err := client.PutMetadata(ctx, uploadDestination, metadata); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	fmt.Printf("Successfully uploaded %s to %s\n", uploadSource, imageKey)
	return nil
}

func computeFileChecksum(path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // G304: Path is provided by user
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
