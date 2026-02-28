// Package randid provides random ID generation utilities.
package randid

import "crypto/rand"

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
