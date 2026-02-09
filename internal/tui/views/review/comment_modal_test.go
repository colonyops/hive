package review

import (
	"strings"
	"testing"

	"github.com/hay-kot/hive/internal/tui/testutil"
)

func TestCommentModal_View(t *testing.T) {
	// Create modal with short context (5 lines)
	contextText := strings.Join([]string{
		"func main() {",
		"    fmt.Println(\"Hello\")",
		"    fmt.Println(\"World\")",
		"    return",
		"}",
	}, "\n")

	modal := NewCommentModal(10, 14, contextText, 80, 24)
	output := modal.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestCommentModal_ViewWithLongContext(t *testing.T) {
	// Create modal with 30 lines of context to test preview truncation
	lines := make([]string, 30)
	for i := range 30 {
		lines[i] = "Line " + string(rune('A'+i%26)) + " with some context text that is longer"
	}
	contextText := strings.Join(lines, "\n")

	modal := NewCommentModal(1, 30, contextText, 80, 24)
	output := modal.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}
