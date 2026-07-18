// Package desktop holds code that exists purely for the Hive desktop app.
// Subpackages implement the desktop's service backends (auth, feed); the
// desktop/ main package is thin Wails wiring over them. Anything reusable
// beyond the desktop (the GitHub client, session/core logic) does not
// belong here.
package desktop

import "os"

// EnvMockMode selects deterministic offline backends instead of live
// GitHub: "feed" starts authenticated with fixture data (the e2e default),
// "onboarding" starts signed out with a self-granting fake device flow.
const EnvMockMode = "HIVE_DESKTOP_MOCK"

// MockMode returns the requested mock mode, or "" for live backends.
func MockMode() string {
	return os.Getenv(EnvMockMode)
}
