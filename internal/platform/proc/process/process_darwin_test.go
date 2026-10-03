//go:build darwin

package process

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseKernProcargs2_EnvUnavailable(t *testing.T) {
	// Build a minimal KERN_PROCARGS2 payload with argc=1, exec_path, argv, but no env.
	argc := uint32(1)
	execPath := []byte("/usr/bin/claude\x00")
	argv0 := []byte("claude\x00")

	var buf []byte
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, argc)
	buf = append(buf, b...)
	buf = append(buf, execPath...)
	buf = append(buf, argv0...)

	argv, env := parseKernProcargs2(buf)
	assert.Equal(t, []string{"claude"}, argv)
	assert.Nil(t, env, "env should be nil when no env entries present")
}

func TestParseKernProcargs2_WithEnv(t *testing.T) {
	argc := uint32(1)
	execPath := []byte("/path/to/node\x00")
	argv0 := []byte("node\x00")
	envEntry := []byte("CLAUDECODE=1\x00")
	terminator := []byte("\x00")

	var buf []byte
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, argc)
	buf = append(buf, b...)
	buf = append(buf, execPath...)
	buf = append(buf, argv0...)
	buf = append(buf, envEntry...)
	buf = append(buf, terminator...)

	argv, env := parseKernProcargs2(buf)
	assert.Equal(t, []string{"node"}, argv)
	assert.Equal(t, map[string]string{"CLAUDECODE": "1"}, env)
}
