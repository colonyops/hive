package stores

import "strings"

// likePrefix turns a literal topic prefix into the LIKE pattern that matches
// it and nothing wider: metacharacters in the prefix are escaped for the
// queries' ESCAPE '\' clause before the trailing wildcard goes on.
func likePrefix(prefix string) string {
	prefix = strings.ReplaceAll(prefix, `\`, `\\`)
	prefix = strings.ReplaceAll(prefix, `%`, `\%`)
	prefix = strings.ReplaceAll(prefix, `_`, `\_`)
	return prefix + "%"
}
