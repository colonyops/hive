package slug

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValid(t *testing.T) {
	for _, s := range []string{"a", "0", "review-pr", "a-1-b", strings.Repeat("a", MaxLen)} {
		assert.Truef(t, Valid(s), "Valid(%q)", s)
	}
	for _, s := range []string{"", "-lead", "Upper", "snake_case", "dot.ted", "sp ace", strings.Repeat("a", MaxLen+1)} {
		assert.Falsef(t, Valid(s), "Valid(%q)", s)
	}
}
