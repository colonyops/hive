package panebridge

import "bytes"

// dropReplies removes the answers libghostty writes back for terminal queries
// in the pane's output. tmux already answered those queries for the pane, so
// a second answer would reach the program as typed garbage (ADR
// the-embedded-emulator-never-answers-terminal-queries-on-a-tmux-pane).
//
// The surface writes keys and replies into the same PTY, so replies are told
// apart by shape. Only shapes no key encodes are dropped, with one known
// overlap: a cursor position report `CSI row;col R` has the shape of a
// modified F3 in the legacy encoding (`CSI 1;mod R`), so a report for row 1
// with col 2..16 is let through rather than eat that key.
func dropReplies(input []byte) []byte {
	out := input[:0:0]
	for i := 0; i < len(input); {
		if n := replyLen(input[i:]); n > 0 {
			i += n
			continue
		}
		out = append(out, input[i])
		i++
	}
	return out
}

// replyLen is the length of the reply input starts with, or 0.
func replyLen(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b {
		return 0
	}
	switch b[1] {
	case '[':
		return csiReplyLen(b)
	case ']':
		// OSC color and palette reports: ESC ] Ps ; rgb:... ST
		end, n := stringEnd(b)
		if end > 0 && bytes.Contains(b[2:end], []byte(";rgb:")) {
			return n
		}
	case 'P':
		// DCS reports: XTVERSION (>|), DECRQSS ($r), XTGETTCAP (+r).
		end, n := stringEnd(b)
		if end > 0 {
			body := b[2:end]
			if bytes.HasPrefix(body, []byte(">|")) || bytes.Contains(body, []byte("$r")) || bytes.Contains(body, []byte("+r")) {
				return n
			}
		}
	}
	return 0
}

func csiReplyLen(b []byte) int {
	i := 2
	for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
		i++
	}
	if i >= len(b) {
		return 0
	}
	params, final := b[2:i], b[i]
	n := i + 1
	switch {
	case final == 'c' && len(params) > 0 && (params[0] == '?' || params[0] == '>' || params[0] == '='):
		return n // device attributes
	case final == 'n' && (string(params) == "0" || string(params) == "3"):
		return n // status report
	case final == 'y' && bytes.HasSuffix(params, []byte("$")):
		return n // mode report (DECRPM)
	case final == 'u' && len(params) > 0 && params[0] == '?':
		return n // kitty keyboard flags
	case final == 't' && (bytes.HasPrefix(params, []byte("4;")) || bytes.HasPrefix(params, []byte("6;")) || bytes.HasPrefix(params, []byte("8;"))):
		return n // window and cell size reports
	case final == 'R' && isCursorReport(params):
		return n
	}
	return 0
}

func isCursorReport(params []byte) bool {
	row, col, ok := bytes.Cut(params, []byte(";"))
	if !ok || len(row) == 0 || len(col) == 0 || !allDigits(row) || !allDigits(col) {
		return false
	}
	if string(row) == "1" && len(col) <= 2 && col[0] >= '2' && (len(col) == 1 || string(col) <= "16") {
		return false // a modified F3
	}
	return true
}

func allDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// stringEnd finds the ST or BEL ending an OSC or DCS: where its body ends
// and the sequence's whole length.
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

// incompleteTail is where a trailing escape sequence that has not fully
// arrived starts (len(input) when none). A lone trailing ESC counts as
// incomplete; the caller flushes it after a short wait.
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
