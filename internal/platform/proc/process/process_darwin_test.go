//go:build darwin

package process

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseKernProcargs2_StopsAtArgc(t *testing.T) {
	var buf []byte
	buf = binary.LittleEndian.AppendUint32(buf, 1)
	buf = append(buf, "/path/to/node\x00"...)
	buf = append(buf, "\x00\x00"...)
	buf = append(buf, "node\x00"...)
	buf = append(buf, "SOME_VAR=1\x00\x00"...)

	assert.Equal(t, []string{"node"}, parseKernProcargs2(buf))
}
