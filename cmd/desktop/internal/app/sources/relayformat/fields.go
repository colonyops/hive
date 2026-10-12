package relayformat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// header matches names case-insensitively because a stored delivery keeps the
// sender's spelling for anything Go's canonicalization does not reach.
func header(h map[string][]string, names ...string) string {
	for _, name := range names {
		for key, values := range h {
			if !strings.EqualFold(key, name) {
				continue
			}
			for _, v := range values {
				if v = strings.TrimSpace(v); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

func hasHeader(h map[string][]string, names ...string) bool {
	for _, name := range names {
		for key := range h {
			if strings.EqualFold(key, name) {
				return true
			}
		}
	}
	return false
}

func query(q url.Values, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(q.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

// field reads a scalar from a decoded object as a string, so a JSON number
// and a string compare the same way.
func field(obj map[string]any, name string) string {
	switch v := obj[name].(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func deliveryHash(d Delivery) string {
	sum := sha256.Sum256([]byte(d.ID))
	return hex.EncodeToString(sum[:])
}

func httpURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	return ""
}

// summary is the first line of a message, cut to 80 runes, for a title.
func summary(message string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	line = strings.TrimSpace(line)
	if r := []rune(line); len(r) > 80 {
		return string(r[:80])
	}
	return line
}

// priority accepts ntfy's names and any integer.
func priority(s string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return 0, false
	case "min":
		return 1, true
	case "low":
		return 2, true
	case "default":
		return 3, true
	case "high":
		return 4, true
	case "max", "urgent":
		return 5, true
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return n, true
}

func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func setIfPresent(payload map[string]any, key, value string) {
	if value != "" {
		payload[key] = value
	}
}
