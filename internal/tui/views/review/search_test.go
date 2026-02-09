package review

import (
	"testing"

	"github.com/hay-kot/hive/internal/tui/testutil"
)

func TestSearchMode_View(t *testing.T) {
	// Create SearchMode with empty search input
	searchMode := NewSearchMode()
	searchMode.Activate()

	output := searchMode.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestSearchMode_ViewWithQuery(t *testing.T) {
	// Create SearchMode with a query
	searchMode := NewSearchMode()
	searchMode.Activate()
	searchMode.input.SetValue("test query")

	output := searchMode.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}
