package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBranchName(t *testing.T) {
	valid := []string{
		"main",
		"feat/my-feature",
		"hive/my-session-abc123",
		"dev/fix-something",
		"release/1.2.3",
		"UPPER-CASE",
		"123numeric",
		"a",
	}
	for _, name := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			assert.NoError(t, ValidateBranchName(name))
		})
	}

	invalid := []struct {
		name   string
		input  string
		errMsg string
	}{
		{"empty", "", "empty"},
		{"at-alone", "@", "\"@\""},
		{"space", "feat/my feature", "spaces"},
		{"tilde", "feat~1", "'~'"},
		{"caret", "feat^2", "'^'"},
		{"colon", "feat:something", "':'"},
		{"question", "feat?", "'?'"},
		{"asterisk", "feat*", "'*'"},
		{"bracket", "feat[0]", "'['"},
		{"backslash", "feat\\branch", "'\\'"},
		{"double-dot", "feat..fix", "'..'"},
		{"at-brace", "feat@{1}", "'@{'"},
		{"double-slash", "feat//bar", "'//'"},
		{"starts-with-dot", ".hidden", "start with '.'"},
		{"starts-with-slash", "/feat", "start with '/'"},
		{"ends-with-dot", "feat.", "end with '.'"},
		{"ends-with-slash", "feat/", "end with '/'"},
		{"ends-with-lock", "feat.lock", "'.lock'"},
		{"component-starts-with-dot", "feat/.hidden", "start with '.'"},
		{"component-ends-with-lock", "feat/refs.lock", "'.lock'"},
		{"control-char", "feat\x01branch", "control character"},
		{"newline", "feat\nbranch", "control character"},
	}
	for _, tt := range invalid {
		t.Run("invalid/"+tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}
