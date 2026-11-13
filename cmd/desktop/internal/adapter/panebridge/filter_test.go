package panebridge

import "testing"

func TestDropRepliesKeepsKeysAndDropsAnswers(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"text", "ls -la\r", "ls -la\r"},
		{"arrows and a modified F3", "\x1b[A\x1b[1;2R\x1b[1;5D", "\x1b[A\x1b[1;2R\x1b[1;5D"},
		{"device attributes", "a\x1b[?62;22cb\x1b[>1;10;0cc", "abc"},
		{"cursor position report", "x\x1b[24;80Ry", "xy"},
		{"status report", "\x1b[0n!", "!"},
		{"mode report", "\x1b[?2026;2$y.", "."},
		{"kitty flags", "\x1b[?0u.", "."},
		{"cell size report", "\x1b[6;18;9t.", "."},
		{"color report BEL", "\x1b]11;rgb:1e1e/1e1e/2e2e\x07.", "."},
		{"color report ST", "\x1b]10;rgb:ffff/ffff/ffff\x1b\\.", "."},
		{"xtversion", "\x1bP>|ghostty 1.3\x1b\\.", "."},
		{"bracketed paste is input", "\x1b[200~hi\x1b[201~", "\x1b[200~hi\x1b[201~"},
		{"focus reports are input", "\x1b[I\x1b[O", "\x1b[I\x1b[O"},
	}
	for _, c := range cases {
		if got := string(dropReplies([]byte(c.in))); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestIncompleteTailHoldsACutSequence(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"ab\x1b[92;", 2},
		{"ab\x1b", 2},
		{"ab\x1b[A", 5},
		{"\x1b]11;rgb:ff", 0},
		{"\x1bx", 2},
	}
	for _, c := range cases {
		if got := incompleteTail([]byte(c.in)); got != c.want {
			t.Errorf("incompleteTail(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
