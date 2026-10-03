package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashContent_EmptyIsStable(t *testing.T) {
	first := hashContent(normalizeContent(""))
	second := hashContent(normalizeContent(""))
	assert.Equal(t, first, second, "hashing empty content must be deterministic")
	assert.NotEmpty(t, first, "empty content still hashes to a fixed, non-empty digest")
}

func TestNormalizeContent_StripsSpinnerGlyphsFromAssessPackage(t *testing.T) {
	// The generic/spinner-shape rule's glyph inventory (assess.SpinnerGlyphs)
	// must be exactly what churn normalization strips — a drifted private
	// copy would let spinner animation alone register as churn.
	got := normalizeContent("⠋ Thinking…")
	assert.NotContains(t, got, "⠋")
}

func TestNormalizeContent_DynamicCountersDoNotAffectHash(t *testing.T) {
	a := hashContent(normalizeContent("Downloading update... 45%"))
	b := hashContent(normalizeContent("Downloading update... 78%"))
	assert.Equal(t, a, b, "only the live percentage differs; normalized content must hash identically")
}

func TestNormalizeContent_RealContentChangeAffectsHash(t *testing.T) {
	a := hashContent(normalizeContent("line one"))
	b := hashContent(normalizeContent("line two"))
	assert.NotEqual(t, a, b)
}
