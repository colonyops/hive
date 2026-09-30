package ptyterm

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRingKeepsEverythingBelowTheCap(t *testing.T) {
	r := newRing(16)
	_, _ = r.Write([]byte("abc"))
	_, _ = r.Write([]byte("def"))
	require.Equal(t, "abcdef", string(r.Snapshot()))
}

func TestRingKeepsTheTailOnceFull(t *testing.T) {
	r := newRing(8)
	_, _ = r.Write([]byte("0123456789"))
	require.Equal(t, "23456789", string(r.Snapshot()))

	_, _ = r.Write([]byte("ab"))
	require.Equal(t, "456789ab", string(r.Snapshot()))
}

func TestRingWrapsAcrossTheEnd(t *testing.T) {
	r := newRing(6)
	_, _ = r.Write([]byte("abcd"))
	_, _ = r.Write([]byte("efgh"))
	require.Equal(t, "cdefgh", string(r.Snapshot()))
}

// The wrap arithmetic is where a ring goes wrong, so it is checked against the
// definition — the last max bytes written — over random chunk sizes.
func TestRingMatchesTheTailOfEverythingWritten(t *testing.T) {
	const max = 64
	r := newRing(max)
	rng := rand.New(rand.NewSource(1))
	var all []byte

	for range 500 {
		chunk := make([]byte, 1+rng.Intn(3*max))
		for i := range chunk {
			chunk[i] = byte('a' + rng.Intn(26))
		}
		_, _ = r.Write(chunk)
		all = append(all, chunk...)

		want := all
		if len(want) > max {
			want = want[len(want)-max:]
		}
		require.True(t, bytes.Equal(want, r.Snapshot()), "after %d bytes: want %q got %q", len(all), want, r.Snapshot())
	}
}
