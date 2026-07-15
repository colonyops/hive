package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests that exercise the override set it explicitly with t.Setenv.
	_ = os.Unsetenv(EnvDefaultAgent)
	os.Exit(m.Run())
}
