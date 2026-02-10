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

	modal := NewCommentModal(10, 14, contextText, 80, 24, 80)
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

	modal := NewCommentModal(1, 30, contextText, 80, 24, 80)
	output := modal.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestCommentModal_ViewMultiline(t *testing.T) {
	// Create modal with simple multiline input
	contextText := "func example() {\n    return true\n}"

	modal := NewCommentModal(1, 3, contextText, 80, 24, 80)
	// Simulate multiline textArea input
	modal.textArea.SetValue("Line 1\nLine 2\nLine 3")

	output := modal.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestCommentModal_ViewLongMultiline(t *testing.T) {
	// Create modal with 10 lines to test textArea scrolling
	contextText := "func example() {\n    return true\n}"

	modal := NewCommentModal(1, 3, contextText, 80, 24, 80)
	// Simulate 10 lines of input
	lines := make([]string, 10)
	for i := range 10 {
		lines[i] = "This is comment line " + string(rune('0'+i)) + " with some text"
	}
	modal.textArea.SetValue(strings.Join(lines, "\n"))

	output := modal.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}
