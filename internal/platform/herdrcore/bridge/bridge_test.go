package bridge

import (
	"testing"

	"github.com/colonyops/hive/internal/platform/herdrcore"
)

func TestIncompleteTail(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"ab\x1b[92;", 2},
		{"ab\x1b]11;rgb:ff", 2},
		{"ab\x1b]11;rgb:ff\x1b", 2},
		{"\x1b[Aab\x1b[1;5", 5},
		{"ab\x1b", 2},
		{"ab\x1b[A", 5},
		{"\x1b]11;rgb:ff\x1b\\x", 14},
		{"\x1bx", 2},
	}
	for _, c := range cases {
		if got := incompleteTail([]byte(c.in)); got != c.want {
			t.Errorf("incompleteTail(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTakeRepliesReadsDisplayFacts(t *testing.T) {
	var facts herdrcore.Display
	in := "x\x1b]10;rgb:ffff/eeee/dddd\x1b\\\x1b]11;rgb:1010/2020/3030\x07\x1b[6;18;9t\x1b[?62;22cy"
	rest, done := takeReplies([]byte(in), &facts)
	if string(rest) != "xy" || !done {
		t.Fatalf("rest %q done %v", rest, done)
	}
	if *facts.FG != "#ffeedd" || *facts.BG != "#102030" || *facts.Cell != [2]uint32{9, 18} {
		t.Fatalf("facts %+v", facts)
	}
}

func TestShownKeepsFocusReportsOn(t *testing.T) {
	got := string(shown([]byte("\x1b[?9001h\x1b[?1004lhi\x1b[?9001l")))
	if got != "\x1b[?1004hhi" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandQuotes(t *testing.T) {
	got := Command("/Apps/Hive Desktop.app/hive", "/s/it's.sock", "t3")
	want := `'/Apps/Hive Desktop.app/hive' --herdr-attach '/s/it'\''s.sock' 't3'`
	if got != want {
		t.Fatalf("got %s", got)
	}
}
