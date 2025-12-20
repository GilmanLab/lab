// Package updater provides file update operations for the image pipeline.
package updater

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"text/template"
)

// TemplateData contains variables available for template substitution.
type TemplateData struct {
	Source SourceData
}

// SourceData contains source-related template variables.
type SourceData struct {
	URL      string
	Checksum string
}

// Replacement defines a regex-based replacement operation.
type Replacement struct {
	Pattern string // Regex pattern to match
	Value   string // Replacement value (may contain Go templates)
}

// FileUpdater performs regex-based file updates with template substitution.
type FileUpdater struct {
	replacements []compiledReplacement
	data         TemplateData
}

type compiledReplacement struct {
	regex    *regexp.Regexp
	template *template.Template
}

// New creates a new FileUpdater with the given replacements and template data.
func New(replacements []Replacement, data TemplateData) (*FileUpdater, error) {
	compiled := make([]compiledReplacement, 0, len(replacements))

	for i, r := range replacements {
		regex, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile pattern[%d] %q: %w", i, r.Pattern, err)
		}

		tmpl, err := template.New(fmt.Sprintf("replacement-%d", i)).Parse(r.Value)
		if err != nil {
			return nil, fmt.Errorf("parse template[%d] %q: %w", i, r.Value, err)
		}

		compiled = append(compiled, compiledReplacement{
			regex:    regex,
			template: tmpl,
		})
	}

	return &FileUpdater{
		replacements: compiled,
		data:         data,
	}, nil
}

// UpdateContent applies all replacements to the given content.
// Returns the modified content and whether any changes were made.
func (u *FileUpdater) UpdateContent(content []byte) (result []byte, modified bool, err error) {
	result = content

	for i, r := range u.replacements {
		// Execute the template to get the replacement value
		var buf bytes.Buffer
		if err = r.template.Execute(&buf, u.data); err != nil {
			return nil, false, fmt.Errorf("execute template[%d]: %w", i, err)
		}
		replacement := buf.Bytes()

		// Check if the pattern matches
		if r.regex.Match(result) {
			newResult := r.regex.ReplaceAll(result, replacement)
			if !bytes.Equal(result, newResult) {
				modified = true
				result = newResult
			}
		}
	}

	return result, modified, nil
}

// UpdateFile reads a file, applies replacements, and writes back if modified.
// Returns whether the file was modified.
func (u *FileUpdater) UpdateFile(path string) (bool, error) {
	content, err := os.ReadFile(path) //nolint:gosec // G304: Path is provided by user
	if err != nil {
		return false, fmt.Errorf("read file %s: %w", path, err)
	}

	updated, modified, err := u.UpdateContent(content)
	if err != nil {
		return false, fmt.Errorf("update content: %w", err)
	}

	if !modified {
		return false, nil
	}

	// Get original file permissions
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat file %s: %w", path, err)
	}

	if err := os.WriteFile(path, updated, info.Mode()); err != nil {
		return false, fmt.Errorf("write file %s: %w", path, err)
	}

	return true, nil
}
