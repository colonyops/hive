package cliengine

import (
	"encoding/json"
	"fmt"
	"time"
)

// TimeNow is overridable in tests so age formatting is deterministic.
var TimeNow = time.Now

// ShortAge renders a compact age like "3w", "5d", "2mo", or "1y" relative to
// TimeNow. It returns "" for a zero or future timestamp.
func ShortAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := TimeNow().Sub(t)
	switch {
	case d < 0:
		return ""
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

// DecodeList unmarshals a CLI's JSON array stdout into T entries.
func DecodeList[T any](out []byte) ([]T, error) {
	var entries []T
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// DecodeJSON unmarshals a CLI's JSON object stdout into dest.
func DecodeJSON(out []byte, dest any) error {
	return json.Unmarshal(out, dest)
}
