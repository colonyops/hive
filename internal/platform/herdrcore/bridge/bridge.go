// Package bridge puts the terminal it runs in onto a herdr-core terminal:
// raw bytes both ways, like `hc attach`, but with no detach key. A native
// terminal surface runs it as its command, so libghostty draws and answers
// for the program herdr-core keeps alive.
package bridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/colonyops/hive/internal/platform/herdrcore"
)

// askDisplay asks the outer terminal for its colors and cell size; the DA1
// reply marks the end of the answers.
const askDisplay = "\x1b]10;?\x1b\\\x1b]11;?\x1b\\\x1b]12;?\x1b\\\x1b[16t\x1b[c"

// reset undoes modes the program may have left on in the outer terminal
// (from herdr-core's attach.rs).
const reset = "\x1b[?2026l\x1b[?1049h\x1b[=0;1u\x1b[?1049l\x1b[?1047l\x1b[?47l\x1b[=0;1u\x1b[>4;0m" +
	"\x1b7\x1b[4l\x1b[20l\x1b[?1l\x1b[?5l\x1b[?6l\x1b[?66l\x1b[?67l\x1b[?69l\x1b[?9l\x1b[?1000l\x1b[?1001l\x1b[?1002l" +
	"\x1b[?1003l\x1b[?1004l\x1b[?1005l\x1b[?1006l\x1b[?1015l\x1b[?1016l\x1b[?2004l\x1b[?2031l\x1b[?2033l" +
	"\x1b[?2048l\x1b[?7h\x1b[?25h\x1b[?1007h\x1b[r\x1b8\x1b[0 q\x1b(B\x1b[0m" +
	"\x1b]104\x1b\\\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\\x1b]22;\x1b\\\r\n"

// escapeTime is how long a sequence cut off at the end of a read waits for
// its rest: nothing more by then means a lone Esc, which is a key.
const escapeTime = 50 * time.Millisecond

// Options name the core and terminal to attach to.
type Options struct {
	Socket   string
	Terminal string
	// History is how many scrollback lines the first paint carries.
	History int
}

// Run attaches stdin/stdout to the terminal until it ends or stdin closes.
// It returns the exit code to use: the program's when it exited.
func Run(ctx context.Context, opts Options) int {
	in, out := os.Stdin, os.Stdout
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		fmt.Fprintln(os.Stderr, "hive: the herdr attach bridge needs a terminal")
		return 2
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hive:", err)
		return 2
	}
	defer func() {
		_, _ = io.WriteString(out, reset)
		_ = term.Restore(fd, old)
	}()

	input := make(chan []byte, 64)
	go readInput(in, input)

	facts, early := askFacts(out, input)

	c, err := herdrcore.Dial(ctx, opts.Socket)
	if err != nil {
		_, _ = fmt.Fprintf(out, "\r\nhive: cannot reach herdr-core at %s: %v\r\n", opts.Socket, err)
		return 1
	}
	defer func() { _ = c.Close() }()

	cols, rows, _ := term.GetSize(int(out.Fd()))
	s, _, err := c.Attach(ctx, herdrcore.AttachParams{
		Terminal: opts.Terminal, Cols: uint16(max(cols, 1)), Rows: uint16(max(rows, 1)),
		History: opts.History, Display: facts,
	})
	if err != nil {
		_, _ = fmt.Fprintf(out, "\r\nhive: cannot attach %s: %v\r\n", opts.Terminal, err)
		return 1
	}
	// Focus reports pick the active viewer; the core passes them on to the
	// program only if it asked.
	_, _ = io.WriteString(out, "\x1b[?1004h")

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64<<10)
		for {
			n, err := s.Read(buf)
			if n > 0 {
				_, _ = out.Write(shown(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)

	if len(early) > 0 {
		_, _ = s.Write(early)
	}
	var carry []byte
	flush := time.NewTimer(time.Hour)
	flush.Stop()
	for {
		select {
		case <-flush.C:
			// Nothing completed the sequence in time: a lone Esc is a key.
			if len(carry) > 0 {
				_, _ = s.Write(carry)
				carry = nil
			}
		case <-ctx.Done():
			_ = s.Close()
			return 0
		case <-done:
			return exitCode(s.Ended())
		case <-winch:
			if cols, rows, err := term.GetSize(int(out.Fd())); err == nil {
				_ = s.Resize(uint16(cols), uint16(rows))
			}
		case chunk, ok := <-input:
			if !ok {
				_ = s.Close()
				return 0
			}
			flush.Stop()
			data := slices.Concat(carry, chunk)
			cut := incompleteTail(data)
			carry = append([]byte(nil), data[cut:]...)
			data = takeFocus(data[:cut], s)
			if len(data) > 0 {
				_, _ = s.Write(data)
			}
			if len(carry) > 0 {
				flush.Reset(escapeTime)
			}
		}
	}
}

func readInput(in io.Reader, ch chan<- []byte) {
	buf := make([]byte, 32<<10)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			ch <- append([]byte(nil), buf[:n]...)
		}
		if err != nil {
			close(ch)
			return
		}
	}
}

// askFacts asks the outer terminal for its display facts and waits up to
// 500ms for the answers. Input that arrives meanwhile is returned for the
// program.
func askFacts(out io.Writer, input <-chan []byte) (*herdrcore.Display, []byte) {
	_, _ = io.WriteString(out, askDisplay)
	var facts herdrcore.Display
	var rest, carry []byte
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case <-deadline:
			return &facts, append(rest, carry...)
		case chunk, ok := <-input:
			if !ok {
				return &facts, append(rest, carry...)
			}
			data := slices.Concat(carry, chunk)
			cut := incompleteTail(data)
			carry = append([]byte(nil), data[cut:]...)
			r, finished := takeReplies(data[:cut], &facts)
			rest = append(rest, r...)
			if finished {
				return &facts, append(rest, carry...)
			}
		}
	}
}

// takeReplies pulls answers to askDisplay out of input into facts. It
// returns the remaining bytes and whether the closing DA1 reply arrived.
func takeReplies(input []byte, facts *herdrcore.Display) ([]byte, bool) {
	var rest []byte
	done := false
	for i := 0; i < len(input); {
		tail := input[i:]
		if which := oscColor(tail); which >= 0 {
			if end, n := stringEnd(tail); end > 0 {
				if color := parseRGB(string(tail[5:end])); color != "" {
					switch which {
					case 0:
						facts.FG = &color
					case 1:
						facts.BG = &color
					default:
						facts.Cursor = &color
					}
				}
				i += n
				continue
			}
		}
		if body, ok := bytes.CutPrefix(tail, []byte("\x1b[6;")); ok {
			if t := bytes.IndexByte(body, 't'); t >= 0 {
				parts := strings.Split(string(body[:t]), ";")
				if len(parts) == 2 {
					h, errH := strconv.ParseUint(parts[0], 10, 32)
					w, errW := strconv.ParseUint(parts[1], 10, 32)
					if errH == nil && errW == nil {
						facts.Cell = &[2]uint32{uint32(w), uint32(h)}
					}
				}
				i += 4 + t + 1
				continue
			}
		}
		if body, ok := bytes.CutPrefix(tail, []byte("\x1b[?")); ok {
			c := bytes.IndexFunc(body, func(r rune) bool { return (r < '0' || r > '9') && r != ';' })
			if c >= 0 && body[c] == 'c' {
				done = true
				i += 3 + c + 1
				continue
			}
		}
		rest = append(rest, input[i])
		i++
	}
	return rest, done
}

func oscColor(b []byte) int {
	for i, p := range []string{"\x1b]10;", "\x1b]11;", "\x1b]12;"} {
		if bytes.HasPrefix(b, []byte(p)) {
			return i
		}
	}
	return -1
}

// stringEnd finds the ST or BEL that ends an OSC: where its body ends and
// the sequence's whole length.
func stringEnd(b []byte) (int, int) {
	st := bytes.Index(b, []byte("\x1b\\"))
	bel := bytes.IndexByte(b, 0x07)
	switch {
	case st >= 0 && (bel < 0 || st < bel):
		return st, st + 2
	case bel >= 0:
		return bel, bel + 1
	}
	return -1, 0
}

// parseRGB turns `rgb:RRRR/GGGG/BBBB` into `#rrggbb`.
func parseRGB(s string) string {
	s, ok := strings.CutPrefix(s, "rgb:")
	if !ok {
		return ""
	}
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return ""
	}
	out := "#"
	for _, p := range parts {
		if len(p) < 2 {
			return ""
		}
		if _, err := strconv.ParseUint(p[:2], 16, 8); err != nil {
			return ""
		}
		out += strings.ToLower(p[:2])
	}
	return out
}

// takeFocus turns the outer terminal's focus reports into FOCUS frames.
func takeFocus(data []byte, s *herdrcore.Stream) []byte {
	for _, f := range []struct {
		seq     string
		focused bool
	}{{"\x1b[I", true}, {"\x1b[O", false}} {
		for {
			at := bytes.Index(data, []byte(f.seq))
			if at < 0 {
				break
			}
			data = append(data[:at:at], data[at+3:]...)
			_ = s.Focus(f.focused)
		}
	}
	return data
}

var focusOff = []byte("\x1b[?1004l")

// shown is program output as the outer terminal gets it: focus reports stay
// on because they are the bridge's, and ConPTY's win32-input-mode request
// is dropped.
func shown(p []byte) []byte {
	if !bytes.Contains(p, []byte("\x1b[?9001")) && !bytes.Contains(p, focusOff) {
		return p
	}
	p = bytes.ReplaceAll(p, []byte("\x1b[?9001h"), nil)
	p = bytes.ReplaceAll(p, []byte("\x1b[?9001l"), nil)
	return bytes.ReplaceAll(p, focusOff, []byte("\x1b[?1004h"))
}

// incompleteTail is where a trailing escape sequence that has not fully
// arrived starts (len(input) when none).
func incompleteTail(input []byte) int {
	at := 0
	for {
		found := bytes.IndexByte(input[at:], 0x1b)
		if found < 0 {
			return len(input)
		}
		esc := at + found
		rest := input[esc+1:]
		n := -1
		switch {
		case len(rest) == 0:
		case rest[0] == '[':
			if p := bytes.IndexFunc(rest[1:], func(r rune) bool { return r >= 0x40 && r <= 0x7e }); p >= 0 {
				n = p + 2
			}
		case rest[0] == ']' || rest[0] == 'P' || rest[0] == '_':
			if _, l := stringEnd(rest); l > 0 {
				n = l
			} else if len(rest) > 512 {
				n = len(rest)
			}
		case rest[0] == 'O':
			if len(rest) >= 2 {
				n = 2
			}
		default:
			n = 1
		}
		if n < 0 {
			return esc
		}
		at = esc + 1 + n
	}
}

func exitCode(r *herdrcore.CloseReason) int {
	if r != nil && r.ExitCode != nil {
		return int(*r.ExitCode)
	}
	return 0
}

// Command is the command line a surface runs to attach to terminal through
// the core at socket, using the executable at exe in bridge mode.
func Command(exe, socket, terminal string) string {
	return strings.Join([]string{shellQuote(exe), Flag, shellQuote(socket), shellQuote(terminal)}, " ")
}

// Flag is the first argument that starts the executable in bridge mode:
// `<exe> --herdr-attach <socket> <terminal>`.
const Flag = "--herdr-attach"

// Main runs the bridge when args select it, and reports whether they did.
func Main(args []string) (int, bool) {
	if len(args) != 4 || args[1] != Flag {
		return 0, false
	}
	return Run(context.Background(), Options{Socket: args[2], Terminal: args[3], History: 5000}), true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
