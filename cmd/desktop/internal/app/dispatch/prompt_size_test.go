package dispatch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The quotes shq adds count toward the limit, so the largest prompt that fits
// is two bytes under it.
func TestValidatePromptSize(t *testing.T) {
	require.NoError(t, ValidatePromptSize(""))
	require.NoError(t, ValidatePromptSize(strings.Repeat("a", MaxPromptBytes-2)))

	err := ValidatePromptSize(strings.Repeat("a", MaxPromptBytes-1))
	require.EqualError(t, err, fmt.Sprintf("prompt is %d bytes after shell quoting, over the %d-byte limit", MaxPromptBytes+1, MaxPromptBytes))
}

// A single quote becomes four bytes once quoted, so a prompt a quarter of the
// limit long can still overflow it.
func TestValidatePromptSize_CountsQuotedBytes(t *testing.T) {
	require.Error(t, ValidatePromptSize(strings.Repeat("'", MaxPromptBytes/4)))
}

func TestValidatePromptSize_CountsBytesNotRunes(t *testing.T) {
	require.Error(t, ValidatePromptSize(strings.Repeat("é", MaxPromptBytes/2)))
}
