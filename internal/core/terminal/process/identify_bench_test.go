package process

import "testing"

func BenchmarkCandidates_DirectMatch(b *testing.B) {
	reader := fakeReader{procs: map[int]fakeProc{
		100: {tpgid: 200},
		200: {comm: "claude", argv: []string{"/usr/local/bin/claude"}},
	}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Candidates(100, reader); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCandidates_WrapperDepth2(b *testing.B) {
	reader := fakeReader{
		procs: map[int]fakeProc{
			100: {tpgid: 200},
			200: {comm: "sh"},
			201: {comm: "bash"},
			202: {comm: "claude", argv: []string{"claude"}},
		},
		children: map[int][]int{200: {201}, 201: {202}},
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Candidates(100, reader); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCandidates_NoChildren(b *testing.B) {
	reader := fakeReader{
		procs: map[int]fakeProc{
			100: {tpgid: 200},
			200: {comm: "zsh"},
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Candidates(100, reader); err != nil {
			b.Fatal(err)
		}
	}
}
