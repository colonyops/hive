package classifier_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testToolAgent  = "agent"
	testToolAider  = "aider"
	testToolClaude = "claude"
	testToolCodex  = "codex"
	testToolGemini = "gemini"
	testToolShell  = "shell"
)

func TestClassify_Tier1_TitleMatch(t *testing.T) {
	tests := []struct {
		name      string
		titles    []classifier.TitlePattern
		paneTitle string
		wantHit   bool
		wantTool  string
	}{
		{name: "exact_match", titles: titlePatterns(testToolClaude, testToolClaude), paneTitle: testToolClaude, wantHit: true, wantTool: testToolClaude},
		{name: "case_insensitive_match", titles: titlePatterns("(?i)claude", testToolClaude), paneTitle: "Claude Code", wantHit: true, wantTool: testToolClaude},
		{name: "multiple_patterns", titles: append(titlePatterns(testToolCodex, testToolCodex), titlePattern(testToolGemini, testToolGemini)), paneTitle: testToolGemini, wantHit: true, wantTool: testToolGemini},
		{name: "no_match_falls_through", titles: titlePatterns(testToolClaude, testToolClaude), paneTitle: testToolShell, wantHit: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeReader{}
			cls := classifier.New(tt.titles, reader, nil, nil)
			got := cls.Classify(context.Background(), classifier.PaneInput{PaneTitle: tt.paneTitle, PanePID: 0})
			assertClassifiedAt(t, got)
			assert.Equal(t, tt.wantHit, got.IsAgent)
			if tt.wantHit {
				assert.Equal(t, tt.wantTool, got.Tool)
				assert.Equal(t, classifier.ConfidenceHigh, got.Confidence)
				assert.Equal(t, 1, got.Tier)
				assert.Zero(t, reader.tpgidCalls)
			} else {
				assert.Equal(t, 0, got.Tier)
			}
		})
	}
}

func TestClassify_Tier1_DoesNotMatchWindowName(t *testing.T) {
	cls := classifier.New(titlePatterns(testToolClaude, testToolClaude), &fakeReader{}, nil, nil)
	got := cls.Classify(context.Background(), classifier.PaneInput{WindowName: testToolClaude, PaneTitle: testToolShell})
	assert.False(t, got.IsAgent)
	assert.Equal(t, 0, got.Tier)
}

func TestClassify_Tier2_ProcessTree(t *testing.T) {
	errPermission := errors.New("permission denied")
	tests := []struct {
		name     string
		reader   *fakeReader
		wantHit  bool
		wantTool string
	}{
		{
			name:     "direct_claude_binary",
			reader:   processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: testToolClaude, argv: []string{testToolClaude}, env: map[string]string{"CLAUDECODE": "1"}}}),
			wantHit:  true,
			wantTool: testToolClaude,
		},
		{
			name: "node_wrapper_child_claude",
			reader: processReaderWithChildren(
				map[int]fakeProc{100: {tpgid: 200}, 200: {comm: "node", argv: []string{"node", "wrapper.js"}}},
				map[int][]int{200: {201}},
				map[int]fakeProc{201: {comm: testToolClaude, argv: []string{testToolClaude}}},
			),
			wantHit:  true,
			wantTool: testToolClaude,
		},
		{
			name: "sh_bash_claude_depth_2",
			reader: processReaderWithChildren(
				map[int]fakeProc{100: {tpgid: 200}, 200: {comm: "sh", argv: []string{"sh"}}},
				map[int][]int{200: {201}, 201: {202}},
				map[int]fakeProc{201: {comm: "bash", argv: []string{"bash"}}, 202: {comm: testToolClaude, argv: []string{testToolClaude}}},
			),
			wantHit:  true,
			wantTool: testToolClaude,
		},
		{
			name:     "python_module_aider",
			reader:   processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: "python3", argv: []string{"python3", "-m", testToolAider}}}),
			wantHit:  true,
			wantTool: testToolAider,
		},
		{name: "shell_fallthrough", reader: processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: "zsh", argv: []string{"zsh"}}}), wantHit: false},
		{
			name:     "nil_env_argv_fallback",
			reader:   processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: "node", argv: []string{"node", "/opt/claude"}, env: nil}}),
			wantHit:  true,
			wantTool: testToolClaude,
		},
		{
			name:     "tpgid_error_fallback",
			reader:   &fakeReader{procs: map[int]fakeProc{100: {comm: testToolCodex, argv: []string{testToolCodex}}}, tpgidErr: errPermission},
			wantHit:  true,
			wantTool: testToolCodex,
		},
		{name: "empty_comm", reader: processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: "", argv: nil}}), wantHit: false},
		{
			name:     "cmdline_error_falls_back_to_comm",
			reader:   processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: testToolGemini, argvErr: errPermission}}),
			wantHit:  true,
			wantTool: testToolGemini,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &fakeCapture{}
			cls := classifier.New(nil, tt.reader, capture, nil)
			got := cls.Classify(context.Background(), classifier.PaneInput{PaneID: "%1", PanePID: 100, WindowName: testToolShell})
			assertClassifiedAt(t, got)
			assert.Equal(t, tt.wantHit, got.IsAgent)
			if tt.wantHit {
				assert.Equal(t, tt.wantTool, got.Tool)
				assert.Equal(t, classifier.ConfidenceHigh, got.Confidence)
				assert.Equal(t, 2, got.Tier)
			} else {
				assert.Equal(t, 0, got.Tier)
			}
			assert.Zero(t, capture.calls)
		})
	}
}

func TestClassify_Tier3_ContentAnalysis(t *testing.T) {
	tests := []struct {
		name     string
		capture  *fakeCapture
		scorer   *fakeScorer
		wantHit  bool
		wantTool string
	}{
		{name: "agent_content_above_threshold", capture: &fakeCapture{content: testToolAgent}, scorer: &fakeScorer{score: 6, categories: 3, tool: testToolClaude}, wantHit: true, wantTool: testToolClaude},
		{name: "agent_content_default_tool", capture: &fakeCapture{content: testToolAgent}, scorer: &fakeScorer{score: 9, categories: 3}, wantHit: true, wantTool: testToolAgent},
		{name: "below_score_threshold", capture: &fakeCapture{content: testToolShell}, scorer: &fakeScorer{score: 5, categories: 3}, wantHit: false},
		{name: "below_category_threshold", capture: &fakeCapture{content: "repl"}, scorer: &fakeScorer{score: 6, categories: 2}, wantHit: false},
		{name: "capture_error", capture: &fakeCapture{err: errors.New("pane gone")}, scorer: &fakeScorer{score: 6, categories: 3}, wantHit: false},
		{name: "empty_content", capture: &fakeCapture{content: ""}, scorer: &fakeScorer{score: 6, categories: 3}, wantHit: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cls := classifier.New(nil, shellReader(), tt.capture, tt.scorer)
			got := cls.Classify(context.Background(), classifier.PaneInput{PaneID: "%1", PanePID: 100})
			assertClassifiedAt(t, got)
			assert.Equal(t, tt.wantHit, got.IsAgent)
			if tt.wantHit {
				assert.Equal(t, tt.wantTool, got.Tool)
				assert.Equal(t, classifier.ConfidenceMedium, got.Confidence)
				assert.Equal(t, 3, got.Tier)
			} else {
				assert.Equal(t, 0, got.Tier)
			}
		})
	}
}

func TestClassify_Tier3_Disabled(t *testing.T) {
	capture := &fakeCapture{content: testToolAgent}
	cls := classifier.New(nil, shellReader(), capture, nil)
	got := cls.Classify(context.Background(), classifier.PaneInput{PaneID: "%1", PanePID: 100})
	assert.False(t, got.IsAgent)
	assert.Zero(t, capture.calls)
}

func TestClassify_CascadeOrder(t *testing.T) {
	t.Run("tier1_short_circuits_process_and_content", func(t *testing.T) {
		reader := &fakeReader{}
		capture := &fakeCapture{content: testToolAgent}
		scorer := &fakeScorer{score: 6, categories: 3}
		cls := classifier.New(titlePatterns(testToolClaude, testToolClaude), reader, capture, scorer)
		got := cls.Classify(context.Background(), classifier.PaneInput{PaneID: "%1", PanePID: 100, PaneTitle: testToolClaude})
		assert.True(t, got.IsAgent)
		assert.Equal(t, 1, got.Tier)
		assert.Zero(t, reader.tpgidCalls)
		assert.Zero(t, capture.calls)
		assert.Zero(t, scorer.calls)
	})

	t.Run("tier2_short_circuits_content", func(t *testing.T) {
		reader := processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: testToolClaude, argv: []string{testToolClaude}}})
		capture := &fakeCapture{content: testToolAgent}
		scorer := &fakeScorer{score: 6, categories: 3}
		cls := classifier.New(nil, reader, capture, scorer)
		got := cls.Classify(context.Background(), classifier.PaneInput{PaneID: "%1", PanePID: 100})
		assert.True(t, got.IsAgent)
		assert.Equal(t, 2, got.Tier)
		assert.NotZero(t, reader.tpgidCalls)
		assert.Zero(t, capture.calls)
		assert.Zero(t, scorer.calls)
	})
}

func TestClassify_AllTiersMiss(t *testing.T) {
	cls := classifier.New(titlePatterns(testToolClaude, testToolClaude), shellReader(), &fakeCapture{content: testToolShell}, &fakeScorer{score: 1, categories: 1})
	got := cls.Classify(context.Background(), classifier.PaneInput{PaneID: "%1", PanePID: 100, WindowName: testToolShell})
	assert.False(t, got.IsAgent)
	assert.Empty(t, got.Tool)
	assert.Empty(t, got.Confidence)
	assert.Equal(t, 0, got.Tier)
	assertClassifiedAt(t, got)
}

func TestClassify_ConfidenceLevels(t *testing.T) {
	tests := []struct {
		name  string
		cls   *classifier.Classifier
		input classifier.PaneInput
		want  classifier.Confidence
	}{
		{name: "tier1_high", cls: classifier.New(titlePatterns(testToolClaude, testToolClaude), &fakeReader{}, nil, nil), input: classifier.PaneInput{PaneTitle: testToolClaude}, want: classifier.ConfidenceHigh},
		{name: "tier2_high", cls: classifier.New(nil, processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: testToolClaude, argv: []string{testToolClaude}}}), nil, nil), input: classifier.PaneInput{PanePID: 100}, want: classifier.ConfidenceHigh},
		{name: "tier3_medium", cls: classifier.New(nil, shellReader(), &fakeCapture{content: testToolAgent}, &fakeScorer{score: 6, categories: 3}), input: classifier.PaneInput{PaneID: "%1", PanePID: 100}, want: classifier.ConfidenceMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cls.Classify(context.Background(), tt.input)
			require.True(t, got.IsAgent)
			assert.Equal(t, tt.want, got.Confidence)
		})
	}
}

func TestClassify_WindowNamePiDoesNotMatchNonAgentPane(t *testing.T) {
	cls := classifier.New(titlePatterns("\\bpi\\b", "pi"), shellReader(), nil, nil)
	got := cls.Classify(context.Background(), classifier.PaneInput{
		PaneID:     "%1",
		PanePID:    100,
		WindowName: "pi",
		PaneTitle:  "zsh",
	})

	assert.False(t, got.IsAgent)
	assert.Equal(t, 0, got.Tier)
}

func TestTitlePatternsFromConfig(t *testing.T) {
	patterns := classifier.TitlePatternsFromConfig([]string{"(?i)claude", "[", "worker", "\\bpi\\b", "pipeline", "π"})
	require.Len(t, patterns, 5)
	assert.Equal(t, testToolClaude, patterns[0].Tool)
	assert.True(t, patterns[0].Pattern.MatchString("Claude"))
	assert.True(t, patterns[1].Pattern.MatchString("WORKER"))
	assert.Equal(t, testToolAgent, patterns[1].Tool)
	assert.Equal(t, "pi", patterns[2].Tool)
	assert.Equal(t, testToolAgent, patterns[3].Tool)
	assert.Equal(t, "pi", patterns[4].Tool)
}

func assertClassifiedAt(t *testing.T, got classifier.Result) {
	t.Helper()
	assert.False(t, got.ClassifiedAt.IsZero())
}

func titlePatterns(pattern, tool string) []classifier.TitlePattern {
	return []classifier.TitlePattern{titlePattern(pattern, tool)}
}

func titlePattern(pattern, tool string) classifier.TitlePattern {
	return classifier.TitlePattern{Pattern: regexp.MustCompile(pattern), Tool: tool}
}

type fakeReader struct {
	procs         map[int]fakeProc
	children      map[int][]int
	tpgidErr      error
	tpgidCalls    int
	commCalls     int
	cmdlineCalls  int
	environCalls  int
	childrenCalls int
}

type fakeProc struct {
	tpgid   int
	comm    string
	argv    []string
	env     map[string]string
	argvErr error
}

func processReader(procs map[int]fakeProc) *fakeReader {
	return &fakeReader{procs: procs}
}

func processReaderWithChildren(procs map[int]fakeProc, children map[int][]int, childProcs map[int]fakeProc) *fakeReader {
	for pid, proc := range childProcs {
		procs[pid] = proc
	}
	return &fakeReader{procs: procs, children: children}
}

func shellReader() *fakeReader {
	return processReader(map[int]fakeProc{100: {tpgid: 200}, 200: {comm: "zsh", argv: []string{"zsh"}}})
}

func (f *fakeReader) TPGID(pid int) (int, error) {
	f.tpgidCalls++
	if f.tpgidErr != nil {
		return 0, f.tpgidErr
	}
	return f.procs[pid].tpgid, nil
}

func (f *fakeReader) Comm(pid int) string {
	f.commCalls++
	return f.procs[pid].comm
}

func (f *fakeReader) Cmdline(pid int) ([]string, error) {
	f.cmdlineCalls++
	proc := f.procs[pid]
	if proc.argvErr != nil {
		return nil, proc.argvErr
	}
	return proc.argv, nil
}

func (f *fakeReader) Environ(pid int) map[string]string {
	f.environCalls++
	return f.procs[pid].env
}

func (f *fakeReader) Children(pid int) ([]int, error) {
	f.childrenCalls++
	return f.children[pid], nil
}

type fakeCapture struct {
	content string
	err     error
	calls   int
}

func (f *fakeCapture) CapturePane(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.content, f.err
}

type fakeScorer struct {
	score      int
	categories int
	tool       string
	calls      int
}

func (f *fakeScorer) Score(_ string) (int, int, string) {
	f.calls++
	return f.score, f.categories, f.tool
}
