// Package config provides configuration parsing for the image pipeline.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Supported API version for the image manifest.
const SupportedAPIVersion = "images.lab.gilman.io/v1alpha1"

// ImageManifest represents the top-level image manifest configuration.
type ImageManifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata contains manifest metadata.
type Metadata struct {
	Name string `yaml:"name"`
}

// Spec contains the list of images to manage.
type Spec struct {
	Images []Image `yaml:"images"`
}

// Image represents a single image configuration.
type Image struct {
	Name        string      `yaml:"name"`
	Source      Source      `yaml:"source"`
	Destination string      `yaml:"destination"`
	Validation  *Validation `yaml:"validation,omitempty"`
	UpdateFile  *UpdateFile `yaml:"updateFile,omitempty"`
}

// Source defines where to download the image from.
type Source struct {
	URL        string `yaml:"url"`
	Checksum   string `yaml:"checksum"`
	Decompress string `yaml:"decompress,omitempty"` // xz, gzip, zstd
}

// Validation defines post-processing validation rules.
type Validation struct {
	Algorithm string `yaml:"algorithm"` // sha256, sha512
	Expected  string `yaml:"expected"`
}

// UpdateFile defines file updates to trigger downstream builds.
type UpdateFile struct {
	Path         string        `yaml:"path"`
	Replacements []Replacement `yaml:"replacements"`
}

// Replacement defines a regex-based replacement in a file.
type Replacement struct {
	Pattern string `yaml:"pattern"` // Regex pattern
	Value   string `yaml:"value"`   // Template: {{ .Source.URL }}, {{ .Source.Checksum }}
}

// EffectiveChecksum returns the checksum to use for idempotency checks.
// If validation.expected is set, use that; otherwise use source.checksum.
func (i *Image) EffectiveChecksum() string {
	if i.Validation != nil && i.Validation.Expected != "" {
		return i.Validation.Expected
	}
	return i.Source.Checksum
}

// LoadManifest reads and parses an image manifest from a file.
func LoadManifest(path string) (*ImageManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest file: %w", err)
	}

	return ParseManifest(data)
}

// ParseManifest parses an image manifest from YAML data.
func ParseManifest(data []byte) (*ImageManifest, error) {
	var manifest ImageManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest YAML: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	return &manifest, nil
}

// Validate checks that the manifest is well-formed.
func (m *ImageManifest) Validate() error {
	if m.APIVersion != SupportedAPIVersion {
		return fmt.Errorf("unsupported apiVersion %q, expected %q", m.APIVersion, SupportedAPIVersion)
	}

	if m.Kind != "ImageManifest" {
		return fmt.Errorf("unsupported kind %q, expected %q", m.Kind, "ImageManifest")
	}

	if m.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	for i, img := range m.Spec.Images {
		if err := img.Validate(); err != nil {
			return fmt.Errorf("image[%d] %q: %w", i, img.Name, err)
		}
	}

	return nil
}

// Validate checks that the image configuration is valid.
func (i *Image) Validate() error {
	if i.Name == "" {
		return fmt.Errorf("name is required")
	}

	if i.Source.URL == "" {
		return fmt.Errorf("source.url is required")
	}

	if !strings.HasPrefix(i.Source.URL, "https://") {
		return fmt.Errorf("source.url must use HTTPS")
	}

	if i.Source.Checksum == "" {
		return fmt.Errorf("source.checksum is required")
	}

	if i.Destination == "" {
		return fmt.Errorf("destination is required")
	}

	// Validate decompress option
	if i.Source.Decompress != "" {
		switch i.Source.Decompress {
		case "xz", "gzip", "zstd":
			// valid
		default:
			return fmt.Errorf("unsupported decompress format %q, must be xz, gzip, or zstd", i.Source.Decompress)
		}

		// validation.expected is required when decompress is used
		if i.Validation == nil || i.Validation.Expected == "" {
			return fmt.Errorf("validation.expected is required when decompress is used")
		}
	}

	// Validate algorithm if validation is specified
	if i.Validation != nil {
		switch i.Validation.Algorithm {
		case "sha256", "sha512":
			// valid
		default:
			return fmt.Errorf("unsupported validation algorithm %q, must be sha256 or sha512", i.Validation.Algorithm)
		}
	}

	// Validate updateFile regex patterns compile
	if i.UpdateFile != nil {
		if i.UpdateFile.Path == "" {
			return fmt.Errorf("updateFile.path is required")
		}

		for j, r := range i.UpdateFile.Replacements {
			if r.Pattern == "" {
				return fmt.Errorf("updateFile.replacements[%d].pattern is required", j)
			}

			if _, err := regexp.Compile(r.Pattern); err != nil {
				return fmt.Errorf("updateFile.replacements[%d].pattern is invalid: %w", j, err)
			}

			if r.Value == "" {
				return fmt.Errorf("updateFile.replacements[%d].value is required", j)
			}
		}
	}

	return nil
}
