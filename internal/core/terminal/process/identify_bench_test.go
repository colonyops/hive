package process

import "testing"

var benchTools = []string{"claude", "gemini", "aider", "codex", "cursor", "opencode", "cline", "pi"}

func BenchmarkIdentifyWith_DirectMatch(b *testing.B) {
	reader := fakeReader{procs: map[int]fakeProc{
		100: {tpgid: 200},
		200: {comm: "claude", argv: []string{"/usr/local/bin/claude"}, env: map[string]string{envClaudeCode: "1"}},
	}}
	b.ReportAllocs()
	for b.Loop() {
		proc, err := IdentifyWith(100, reader, benchTools)
		if err != nil || proc == nil || proc.Tool != "claude" {
			b.Fatalf("IdentifyWith() = %#v, %v", proc, err)
		}
	}
}

func BenchmarkIdentifyWith_WrapperDepth2(b *testing.B) {
	reader := fakeReader{
		procs: map[int]fakeProc{
			100: {tpgid: 200},
			200: {comm: "sh", argv: []string{"sh"}},
			201: {comm: "bash", argv: []string{"bash"}},
			202: {comm: "claude", argv: []string{"claude"}},
		},
		children: map[int][]int{200: {201}, 201: {202}},
	}
	b.ReportAllocs()
	for b.Loop() {
		proc, err := IdentifyWith(100, reader, benchTools)
		if err != nil || proc == nil || proc.Tool != "claude" {
			b.Fatalf("IdentifyWith() = %#v, %v", proc, err)
		}
	}
}

func BenchmarkIdentifyWith_NoMatch(b *testing.B) {
	reader := fakeReader{
		procs: map[int]fakeProc{
			100: {tpgid: 200},
			200: {comm: "zsh", argv: []string{"zsh"}},
			201: {comm: "sleep", argv: []string{"sleep", "600"}},
		},
		children: map[int][]int{200: {201}},
	}
	b.ReportAllocs()
	for b.Loop() {
		proc, err := IdentifyWith(100, reader, benchTools)
		if err != nil || proc == nil || proc.Tool != ToolShell {
			b.Fatalf("IdentifyWith() = %#v, %v", proc, err)
		}
	}
}
