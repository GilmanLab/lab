package images

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "bytes",
			bytes:    500,
			expected: "500 B",
		},
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
		{
			name:     "kilobytes",
			bytes:    1024,
			expected: "1.00 KB",
		},
		{
			name:     "kilobytes with decimal",
			bytes:    1536,
			expected: "1.50 KB",
		},
		{
			name:     "megabytes",
			bytes:    1024 * 1024,
			expected: "1.00 MB",
		},
		{
			name:     "megabytes with decimal",
			bytes:    1024*1024*10 + 1024*512,
			expected: "10.50 MB",
		},
		{
			name:     "gigabytes",
			bytes:    1024 * 1024 * 1024,
			expected: "1.00 GB",
		},
		{
			name:     "gigabytes with decimal",
			bytes:    1024*1024*1024*2 + 1024*1024*512,
			expected: "2.50 GB",
		},
		{
			name:     "large gigabytes",
			bytes:    1024 * 1024 * 1024 * 50,
			expected: "50.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}
