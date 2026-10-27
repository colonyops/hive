// Package core holds the drive server's accounts, tokens, and policies. It
// reaches storage through the port interfaces in ports.go, which
// store/control implements; core must not import it, because the generated
// sqlc package imports Scope.
package core

// Scope is what an API token may do. push also grants everything read does.
//
// ENUM(read, push)
type Scope string

// Covers reports whether holding s satisfies a requirement for want.
func (s Scope) Covers(want Scope) bool {
	return s == want || s == ScopePush
}
