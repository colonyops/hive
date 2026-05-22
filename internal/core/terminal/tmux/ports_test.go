package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLsofListeningPorts(t *testing.T) {
	output := `COMMAND   PID USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
node     1234 hayden 21u  IPv6 0xabc            0t0  TCP *:3000 (LISTEN)
python   1234 hayden 22u  IPv4 0xdef            0t0  TCP 127.0.0.1:8000 (LISTEN)
postgres 5678 hayden 10u  IPv6 0xghi            0t0  TCP [::1]:5432 (LISTEN)
node     1234 hayden 23u  IPv6 0xabc            0t0  TCP *:3000 (LISTEN)
`

	got := parseLsofListeningPorts(output)

	assert.Equal(t, []int{3000, 8000}, got[1234])
	assert.Equal(t, []int{5432}, got[5678])
}

func TestUniquePositiveInts(t *testing.T) {
	assert.Equal(t, []int{1, 2, 3}, uniquePositiveInts([]int{3, 1, 2, 2, 0, -1}))
}
