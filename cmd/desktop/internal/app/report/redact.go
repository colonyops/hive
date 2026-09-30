package report

import (
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// secretKeyParts match a map key (case-insensitive substring) whose value is
// dropped wholesale. Kept narrow enough to avoid mangling common config keys
// like "keybindings".
var secretKeyParts = []string{
	"token", "secret", "credential", "password", "passwd",
	"apikey", "api_key", "access_key", "private_key",
	"client_secret", "authorization", "cookie", "session",
}

// carrierKeys name maps of arbitrary user keys whose values routinely hold
// secrets (action env, request headers). Every value under them is dropped.
var carrierKeys = map[string]bool{
	"env": true, "environment": true, "headers": true, "secrets": true,
}

var valuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[0-9A-Za-z]{20,}`),
	regexp.MustCompile(`github_pat_[0-9A-Za-z_]{20,}`),
	regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]{10,}`),
	regexp.MustCompile(`(?i)bearer\s+[0-9A-Za-z._~+/-]{16,}=*`),
}

// Redact returns a copy of a decoded YAML/JSON value with secret-bearing map
// keys, arbitrary-key secret carriers, and token-shaped string values removed.
func Redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			switch {
			case isSecretKey(k):
				out[k] = redacted
			case carrierKeys[strings.ToLower(k)]:
				out[k] = redactCarrier(val)
			default:
				out[k] = Redact(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = Redact(val)
		}
		return out
	case string:
		return scrubText(t)
	default:
		return v
	}
}

func redactCarrier(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k := range t {
			out[k] = redacted
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = redacted
		}
		return out
	default:
		return redacted
	}
}

func isSecretKey(k string) bool {
	lk := strings.ToLower(k)
	for _, p := range secretKeyParts {
		if strings.Contains(lk, p) {
			return true
		}
	}
	return false
}

func scrubText(s string) string {
	for _, re := range valuePatterns {
		s = re.ReplaceAllString(s, redacted)
	}
	return s
}
