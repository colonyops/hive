package config

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var sizeUnits = map[string]int64{
	"":    1,
	"b":   1,
	"kb":  1e3,
	"mb":  1e6,
	"gb":  1e9,
	"tb":  1e12,
	"kib": 1 << 10,
	"mib": 1 << 20,
	"gib": 1 << 30,
	"tib": 1 << 40,
}

// Size is a byte count, or no limit at all. The zero value is "unset": it is
// what a missing key decodes to, so Validate can tell it from a real limit.
type Size struct {
	Bytes     int64
	Unlimited bool
}

func (s Size) IsSet() bool { return s.Unlimited || s.Bytes > 0 }

// UnmarshalText accepts "unlimited" or a positive number with an optional
// unit: decimal (KB, MB, GB, TB) or binary (KiB, MiB, GiB, TiB). Zero and
// negative values are errors so that no limit is ever implied.
func (s *Size) UnmarshalText(text []byte) error {
	v := strings.ToLower(strings.TrimSpace(string(text)))
	if v == "unlimited" {
		*s = Size{Unlimited: true}
		return nil
	}

	split := strings.IndexFunc(v, func(r rune) bool { return (r < '0' || r > '9') && r != '.' && r != '-' })
	if split < 0 {
		split = len(v)
	}
	number, unit := v[:split], strings.TrimSpace(v[split:])
	mult, ok := sizeUnits[unit]
	if !ok {
		return fmt.Errorf("size %q: unknown unit %q", text, unit)
	}
	n, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return fmt.Errorf("size %q: not a number", text)
	}
	if n <= 0 {
		return errors.New("size must be greater than zero; use \"unlimited\" for no limit")
	}
	bytes := n * float64(mult)
	if bytes >= math.MaxInt64 {
		return fmt.Errorf("size %q is too large", text)
	}
	*s = Size{Bytes: int64(bytes)}
	return nil
}
