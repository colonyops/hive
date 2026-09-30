package mcpcatalog

import (
	"embed"
	"fmt"
)

// docsFS holds one markdown file per registered MCP type, named
// docs/<type>.md, following the actions/flow registry↔docs bijection (ADR
// 0009). TestRegistryDocsBijection fails if either a registered type has no
// doc or a doc names a type that is not registered.
//
//go:embed docs/*.md
var docsFS embed.FS

// Doc returns the markdown documentation for a registered MCP type.
func Doc(mcpType string) (string, error) {
	data, err := docsFS.ReadFile("docs/" + mcpType + ".md")
	if err != nil {
		return "", fmt.Errorf("mcpcatalog: no documentation for MCP type %q", mcpType)
	}
	return string(data), nil
}
