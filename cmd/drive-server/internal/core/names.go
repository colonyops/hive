package core

import "strings"

// EmailName is the name shown for an account with no display name: the part
// of its email before the "@".
func EmailName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	return local
}
