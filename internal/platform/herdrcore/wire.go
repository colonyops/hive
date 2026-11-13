package herdrcore

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Frame kinds of herdr-core's framed mode: `[channel u32 LE][kind u8][len u32 LE][payload]`.
const (
	KindData           byte = 0
	KindCredit         byte = 1
	KindClose          byte = 2
	KindResize         byte = 3
	KindFocus          byte = 4
	KindFacts          byte = 5
	KindAutomaticInput byte = 6
)

const (
	headerSize = 9
	maxPayload = 16 << 20
)

type frame struct {
	channel uint32
	kind    byte
	payload []byte
}

func encodeFrame(channel uint32, kind byte, payload []byte) []byte {
	out := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint32(out[0:4], channel)
	out[4] = kind
	binary.LittleEndian.PutUint32(out[5:9], uint32(len(payload)))
	copy(out[headerSize:], payload)
	return out
}

func readFrame(r io.Reader) (frame, error) {
	var h [headerSize]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return frame{}, err
	}
	n := binary.LittleEndian.Uint32(h[5:9])
	if n > maxPayload {
		return frame{}, fmt.Errorf("herdrcore: frame of %d bytes exceeds %d", n, maxPayload)
	}
	f := frame{channel: binary.LittleEndian.Uint32(h[0:4]), kind: h[4], payload: make([]byte, n)}
	if _, err := io.ReadFull(r, f.payload); err != nil {
		return frame{}, err
	}
	return f, nil
}

func sizePayload(cols, rows uint16) []byte {
	p := make([]byte, 4)
	binary.LittleEndian.PutUint16(p[0:2], cols)
	binary.LittleEndian.PutUint16(p[2:4], rows)
	return p
}

func creditPayload(n int) []byte {
	p := make([]byte, 4)
	binary.LittleEndian.PutUint32(p, uint32(n))
	return p
}
