package tmuxcc

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// floodLine is one %output line the size tmux sends under a flood, escaped the
// way tmux escapes a colored wall of text.
func floodLine() []byte {
	var line bytes.Buffer
	line.WriteString("%output %1 ")
	for line.Len() < 1400 {
		line.WriteString(`\033[32m0012345\033[0m the quick brown fox jumps over the lazy dog\015\012`)
	}
	return line.Bytes()
}

func BenchmarkDecodeOutput(b *testing.B) {
	_, payload, _ := bytes.Cut(bytes.TrimPrefix(floodLine(), []byte("%output ")), []byte(" "))
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for b.Loop() {
		decodeOutput(payload)
	}
}

func BenchmarkGatewayFeedOutput(b *testing.B) {
	line := floodLine()
	gw := NewGateway(zerolog.Nop(), io.Discard, func(Notification) {})
	b.SetBytes(int64(len(line)))
	b.ReportAllocs()
	for b.Loop() {
		if err := gw.Feed(line); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLineScanner(b *testing.B) {
	stream := bytes.Repeat(append(floodLine(), '\n'), 256)
	b.SetBytes(int64(len(stream)))
	b.ReportAllocs()
	for b.Loop() {
		scanner := newLineScanner(bytes.NewReader(stream), maxLineBytes)
		for {
			if _, err := scanner.next(); err != nil {
				break
			}
		}
	}
}

// BenchmarkBrokerFlood is the number the terminal's flood cost scales with:
// frames per megabyte delivered is how many WebSocket frames, webview tasks and
// term.write calls one megabyte of output costs downstream. The subscriber does
// the small amount of work a loopback socket write takes.
func BenchmarkBrokerFlood(b *testing.B) {
	const notification = 1400
	data := bytes.Repeat([]byte("x"), notification)
	const perSecond = 20000 // what a pane flooding through tmux produces
	interval := time.Second / perSecond

	b.ReportAllocs()
	for b.Loop() {
		broker := newBroker(backlogBounds{bytes: 64 << 20}, nil)
		events, unsubscribe := broker.subscribe()
		const total = 8000
		done := make(chan int)
		go func() {
			frames, delivered := 0, 0
			for ev := range events {
				out, ok := ev.(Output)
				if !ok {
					continue
				}
				frames++
				delivered += len(out.Data)
				if delivered >= total*notification {
					break
				}
				time.Sleep(20 * time.Microsecond)
			}
			done <- frames
		}()
		next := time.Now()
		for range total {
			broker.publish(Output{At: next, WindowID: "@1", PaneID: "%1", Data: data})
			next = next.Add(interval)
			for time.Now().Before(next) {
			}
		}
		frames := <-done
		unsubscribe()
		broker.close()
		b.ReportMetric(float64(frames)/(float64(total*notification)/1e6), "frames/MB")
	}
}
