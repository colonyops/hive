package randid

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"length 0", 0},
		{"length 1", 1},
		{"length 4", 4},
		{"length 8", 8},
		{"length 16", 16},
	}

	pattern := regexp.MustCompile(`^[a-z0-9]*$`)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Generate(tt.length)

			assert.Len(t, result, tt.length, "Generate(%d) returned length %d, want %d", tt.length, len(result), tt.length)
			assert.True(t, pattern.MatchString(result), "Generate(%d) = %q, want only lowercase alphanumeric [a-z0-9]", tt.length, result)
		})
	}
}

func TestGenerate_Uniqueness(t *testing.T) {
	// Generate multiple IDs and verify they're not all identical
	// (statistical check - extremely unlikely to fail with proper randomness)
	seen := make(map[string]bool)
	for range 100 {
		id := Generate(8)
		seen[id] = true
	}

	// With 36^8 possible combinations, getting fewer than 90 unique values
	// in 100 tries would indicate a serious randomness problem
	assert.GreaterOrEqual(t, len(seen), 90, "Generate produced only %d unique values in 100 calls, expected near 100", len(seen))
}

func TestGenerate_CharacterDistribution(t *testing.T) {
	// Generate a large sample and verify all expected characters appear
	charCounts := make(map[byte]int)
	for range 1000 {
		id := Generate(10)
		for j := range len(id) {
			charCounts[id[j]]++
		}
	}

	// Verify we see both letters and numbers
	hasLetter := false
	hasDigit := false
	for c := range charCounts {
		if c >= 'a' && c <= 'z' {
			hasLetter = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}

	assert.True(t, hasLetter, "Generate never produced any lowercase letters")
	assert.True(t, hasDigit, "Generate never produced any digits")
}
