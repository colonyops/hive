package herdrcore

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(encodeFrame(7, KindResize, sizePayload(120, 40)))
	f, err := readFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if f.channel != 7 || f.kind != KindResize || !bytes.Equal(f.payload, []byte{120, 0, 40, 0}) {
		t.Fatalf("got %+v", f)
	}
}

func TestSocketPath(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		vars map[string]string
		want string
	}{
		{map[string]string{"HC_SOCKET": "/x.sock"}, "/x.sock"},
		{map[string]string{"HOME": "/h", "HC_SESSION": "s"}, "/h/.local/state/herdr-core/s/s.sock"},
		{map[string]string{"XDG_STATE_HOME": "/st"}, "/st/herdr-core/default/default.sock"},
		{map[string]string{"XDG_RUNTIME_DIR": "/run/u", "HC_SESSION": "s"}, "/run/u/herdr-core/s.sock"},
	}
	for _, c := range cases {
		got, err := SocketPath(env(c.vars))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%v: got %s, want %s", c.vars, got, c.want)
		}
	}
	if _, err := SocketPath(env(map[string]string{"HC_SESSION": "a/b"})); err == nil {
		t.Error("a session with a slash must fail")
	}
}
