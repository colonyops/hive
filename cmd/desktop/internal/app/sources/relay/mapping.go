package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
)

type mapField struct {
	name    string
	pointer string
}

func (m *Map) fields() []mapField {
	return []mapField{
		{"key", m.Key}, {"title", m.Title}, {"url", m.URL}, {"kind", m.Kind}, {"state", m.State},
	}
}

// Apply overlays the node's map on a parsed item. A pointer that resolves to
// nothing, or to an object or array, keeps the parser's value.
func Apply(m *Map, it relayformat.Item) relayformat.Item {
	if m == nil {
		return it
	}
	payload := maps.Clone(it.Payload)
	for _, f := range m.fields() {
		value, ok := lookup(f.pointer, it.Payload)
		if !ok {
			continue
		}
		switch f.name {
		case "key":
			if strings.TrimSpace(value) == "" {
				continue
			}
			it.Key = value
			payload["id"] = value
		case "state":
			if mapped, ok := m.States[value]; ok {
				value = mapped
			}
			payload["state"] = value
		default:
			payload[f.name] = value
		}
	}
	it.Payload = payload
	return it
}

// lookup resolves against the parser's payload, not the one being rewritten,
// so a title pointer at /id reads the parsed id rather than a mapped key.
func lookup(pointer string, parsed map[string]any) (string, bool) {
	if pointer == "" {
		return "", false
	}
	tokens, err := parsePointer(pointer)
	if err != nil {
		return "", false
	}
	v, ok := resolve(parsed, tokens)
	if !ok {
		return "", false
	}
	return scalarString(v)
}

func scalarString(v any) (string, bool) {
	switch v := v.(type) {
	case string:
		return v, true
	case json.Number:
		return v.String(), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int, int64:
		return fmt.Sprint(v), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		return "", false
	}
}

// parsePointer splits an RFC 6901 JSON Pointer into unescaped reference
// tokens. The empty pointer is valid and means unset here.
func parsePointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("JSON Pointer %q must start with /", pointer)
	}
	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		token, err := unescapeToken(part)
		if err != nil {
			return nil, fmt.Errorf("JSON Pointer %q: %w", pointer, err)
		}
		parts[i] = token
	}
	return parts, nil
}

// unescapeToken decodes in one left-to-right pass, so "~01" becomes "~1" as
// RFC 6901 requires.
func unescapeToken(s string) (string, error) {
	if !strings.Contains(s, "~") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			return "", errors.New("~ must be followed by 0 or 1")
		}
		switch s[i+1] {
		case '0':
			b.WriteByte('~')
		case '1':
			b.WriteByte('/')
		default:
			return "", errors.New("~ must be followed by 0 or 1")
		}
		i++
	}
	return b.String(), nil
}

func resolve(doc any, tokens []string) (any, bool) {
	cur := doc
	for _, token := range tokens {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[token]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, ok := arrayIndex(token)
			if !ok || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func arrayIndex(token string) (int, bool) {
	if token == "" || (len(token) > 1 && token[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(token)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
