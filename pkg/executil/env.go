package executil

import (
	"os"
	"slices"
	"strings"
)

// WithoutEnv returns a copy of environ without the variables named in names.
// A nil environ means this process's environment, as it does for
// exec.Cmd.Env.
func WithoutEnv(environ []string, names ...string) []string {
	if environ == nil {
		environ = os.Environ()
	}
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(names, name) {
			out = append(out, kv)
		}
	}
	return out
}
