// Package randid provides random ID generation utilities.
package randid

import (
	"crypto/rand"

	"github.com/google/uuid"
)

// UUIDv7 creates a time-ordered UUID and panics if the system random source fails.
func UUIDv7() string {
	return uuid.Must(uuid.NewV7()).String()
}

// Generate creates a random alphanumeric ID of the specified length.
func Generate(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("randid: crypto/rand.Read failed: " + err.Error())
	}
	for i := range b {
		b[i] = chars[b[i]%byte(len(chars))]
	}
	return string(b)
}
