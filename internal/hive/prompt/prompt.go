package prompt

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/domain/terminal/assess"
)

// AgentState is read from one capture, without the TUI's debounce.
//
// ENUM(active, approval, question, ready, unknown, missing)
type AgentState string

var ErrAgentPaneNotFound = errors.New("no agent pane found for session")

type PaneInput interface {
	CapturePane(ctx context.Context, target multiplexer.Target, opts multiplexer.CaptureOptions) (string, error)
	SendLiteral(ctx context.Context, target multiplexer.Target, text string) error
	SendKey(ctx context.Context, target multiplexer.Target, key multiplexer.NamedKey) error
	Paste(ctx context.Context, target multiplexer.Target, text []byte, opts multiplexer.PasteOptions) error
}

type AgentPaneFinder interface {
	RefreshAll(ctx context.Context)
	DiscoverSession(ctx context.Context, slug string, metadata map[string]string) (*terminal.SessionInfo, terminal.Integration, error)
}

type SubmitTiming struct {
	Poll      time.Duration
	SettleMin time.Duration
	SettleMax time.Duration
	// Redraw is how long to wait for the screen to change after keys.
	Redraw time.Duration
}

var DefaultSubmitTiming = SubmitTiming{
	Poll:      150 * time.Millisecond,
	SettleMin: 500 * time.Millisecond,
	SettleMax: 5 * time.Second,
	Redraw:    3 * time.Second,
}

// Service reads and types into a session's agent pane, found by
// discovery rather than the active window, which is usually the shell. It
// reports what the screen shows and leaves every decision to the caller.
type Service struct {
	panes  func() AgentPaneFinder
	input  PaneInput
	engine *assess.Engine
	timing SubmitTiming
	sleep  func(context.Context, time.Duration) error
}

// NewService takes the pane finder as a getter because the engine rebuilds
// its terminal manager on a config reload. A nil finder means no terminal
// integration runs, and every call reports ErrAgentPaneNotFound.
func NewService(panes func() AgentPaneFinder, input PaneInput) *Service {
	return &Service{
		panes:  panes,
		input:  input,
		engine: assess.NewEngine(),
		timing: DefaultSubmitTiming,
		sleep:  sleepContext,
	}
}

type SendOptions struct {
	NoSubmit bool
	// Lines is how much of the screen to return; zero means 20.
	Lines int
}

type ContextUsage struct {
	Used    string `json:"used"`
	Total   string `json:"total"`
	Percent int    `json:"percent"`
}

// SessionPeek is one capture of the agent pane. State is a hint from the
// status engine; Tail is what the screen shows.
type SessionPeek struct {
	Pane    string        `json:"pane,omitempty"`
	Tool    string        `json:"tool,omitempty"`
	State   AgentState    `json:"state"`
	Context *ContextUsage `json:"context,omitempty"`
	Tail    string        `json:"tail,omitempty"`
}

type agentPane struct {
	target multiplexer.Target
	tool   string
}

func (p *Service) Peek(ctx context.Context, sess session.Session, lines int) (SessionPeek, error) {
	pane, err := p.findPane(ctx, sess)
	if errors.Is(err, ErrAgentPaneNotFound) {
		return SessionPeek{State: AgentStateMissing}, nil
	}
	if err != nil {
		return SessionPeek{}, err
	}
	screen, err := p.capture(ctx, pane)
	if err != nil {
		return SessionPeek{}, err
	}
	return p.peekOf(pane, screen, lines), nil
}

// Send types text, waits for the agent to finish rendering it, presses Enter
// once, and returns the screen after. Agents drop an Enter that arrives while
// they still render the text; the wait is what fixes that.
func (p *Service) Send(ctx context.Context, sess session.Session, text string, opts SendOptions) (SessionPeek, error) {
	if strings.TrimSpace(text) == "" {
		return SessionPeek{}, errors.New("prompt text is empty")
	}
	pane, err := p.findPane(ctx, sess)
	if err != nil {
		return SessionPeek{}, err
	}
	before, err := p.capture(ctx, pane)
	if err != nil {
		return SessionPeek{}, err
	}
	if err := p.typeText(ctx, pane, text); err != nil {
		return SessionPeek{}, err
	}
	typed, err := p.settle(ctx, pane, before)
	if err != nil {
		return SessionPeek{}, err
	}
	if !opts.NoSubmit {
		if typed, err = p.press(ctx, pane, typed, "Enter"); err != nil {
			return SessionPeek{}, err
		}
	}
	return p.peekOf(pane, typed, opts.Lines), nil
}

// SendKeys presses tmux named keys in order and returns the screen after.
func (p *Service) SendKeys(ctx context.Context, sess session.Session, keys []string, lines int) (SessionPeek, error) {
	if len(keys) == 0 {
		return SessionPeek{}, errors.New("no keys to send")
	}
	for i, k := range keys {
		if _, err := multiplexer.NewNamedKey(k); err != nil {
			return SessionPeek{}, fmt.Errorf("key %d: %w", i, err)
		}
	}
	pane, err := p.findPane(ctx, sess)
	if err != nil {
		return SessionPeek{}, err
	}
	screen, err := p.capture(ctx, pane)
	if err != nil {
		return SessionPeek{}, err
	}
	for i, k := range keys {
		if i > 0 {
			if err := p.sleep(ctx, p.timing.Poll); err != nil {
				return SessionPeek{}, err
			}
		}
		if screen, err = p.press(ctx, pane, screen, k); err != nil {
			return SessionPeek{}, err
		}
	}
	return p.peekOf(pane, screen, lines), nil
}

// press sends one key and waits up to Redraw for the screen to change and
// then hold still, so a clear-and-redraw is not returned half drawn.
func (p *Service) press(ctx context.Context, pane agentPane, before, key string) (string, error) {
	if err := p.input.SendKey(ctx, pane.target, multiplexer.NamedKey(key)); err != nil {
		return "", fmt.Errorf("press %s: %w", key, err)
	}
	prev := before
	for waited := time.Duration(0); waited < p.timing.Redraw; waited += p.timing.Poll {
		if err := p.sleep(ctx, p.timing.Poll); err != nil {
			return "", err
		}
		next, err := p.capture(ctx, pane)
		if err != nil {
			return "", err
		}
		if next != before && next == prev {
			return next, nil
		}
		prev = next
	}
	return prev, nil
}

func (p *Service) peekOf(pane agentPane, screen string, lines int) SessionPeek {
	if lines <= 0 {
		lines = 20
	}
	return SessionPeek{
		Pane:    pane.target.Pane,
		Tool:    pane.tool,
		State:   p.classify(screen, pane.tool),
		Context: parseContextUsage(screen),
		Tail:    tailLines(screen, lines),
	}
}

func (p *Service) findPane(ctx context.Context, sess session.Session) (agentPane, error) {
	metadata := sess.Metadata
	if sess.Path != "" {
		metadata = make(map[string]string, len(sess.Metadata)+1)
		maps.Copy(metadata, sess.Metadata)
		metadata[terminal.SessionPathKey] = sess.Path
	}
	// A refresh racing another caller's leaves a stale cache, so retry.
	for attempt := range discoverAttempts {
		if attempt > 0 {
			if err := p.sleep(ctx, p.timing.SettleMin); err != nil {
				return agentPane{}, err
			}
		}
		panes := p.panes()
		if panes == nil {
			return agentPane{}, ErrAgentPaneNotFound
		}
		panes.RefreshAll(ctx)
		info, _, err := panes.DiscoverSession(ctx, sess.Slug, metadata)
		if err != nil {
			return agentPane{}, fmt.Errorf("discover agent pane: %w", err)
		}
		if info != nil && info.PaneID != "" {
			return agentPane{target: multiplexer.Target{Pane: info.PaneID}, tool: info.DetectedTool}, nil
		}
	}
	return agentPane{}, ErrAgentPaneNotFound
}

const discoverAttempts = 3

func (p *Service) capture(ctx context.Context, pane agentPane) (string, error) {
	screen, err := p.input.CapturePane(ctx, pane.target, multiplexer.CaptureOptions{})
	if err != nil {
		return "", fmt.Errorf("capture agent pane: %w", err)
	}
	return screen, nil
}

// typeText pastes multi-line text because a typed newline submits the line.
func (p *Service) typeText(ctx context.Context, pane agentPane, text string) error {
	if strings.ContainsAny(text, "\r\n") {
		if err := p.input.Paste(ctx, pane.target, []byte(text), multiplexer.PasteOptions{Bracketed: true}); err != nil {
			return fmt.Errorf("paste prompt: %w", err)
		}
		return nil
	}
	if err := p.input.SendLiteral(ctx, pane.target, text); err != nil {
		return fmt.Errorf("type prompt: %w", err)
	}
	return nil
}

func (p *Service) settle(ctx context.Context, pane agentPane, before string) (string, error) {
	if err := p.sleep(ctx, p.timing.SettleMin); err != nil {
		return "", err
	}
	prev, err := p.capture(ctx, pane)
	if err != nil {
		return "", err
	}
	for waited := p.timing.SettleMin; waited < p.timing.SettleMax; waited += p.timing.Poll {
		if err := p.sleep(ctx, p.timing.Poll); err != nil {
			return "", err
		}
		next, err := p.capture(ctx, pane)
		if err != nil {
			return "", err
		}
		if settledSignature(next) != settledSignature(before) && settledSignature(next) == settledSignature(prev) {
			return next, nil
		}
		prev = next
	}
	return prev, nil
}

// settledSignature compares only the input box when there is one, because a
// working agent's spinner never stops changing the rest of the screen.
func settledSignature(screen string) string {
	if pending, ok := assess.PromptInput(screen); ok {
		return pending
	}
	return screen
}

func (p *Service) classify(screen, tool string) AgentState {
	switch p.engine.Assess(assess.Snapshot{Content: screen, Tool: strings.ToLower(tool)}).State {
	case assess.StateWorking:
		return AgentStateActive
	case assess.StateApproval:
		return AgentStateApproval
	case assess.StateQuestion:
		return AgentStateQuestion
	case assess.StateIdle:
		return AgentStateReady
	default:
		return AgentStateUnknown
	}
}

var contextRatio = regexp.MustCompile(`(?i)^\s*(\d+(?:\.\d+)?[km]?)\s*/\s*(\d+(?:\.\d+)?[km]?)(?:\s+\d+%)?\s*$`)

// parseContextUsage matches "·"-separated fields one at a time because branch
// names contain slashes too.
func parseContextUsage(screen string) *ContextUsage {
	lines := strings.Split(strings.TrimRight(terminal.StripANSI(screen), "\n"), "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-12; i-- {
		if !strings.Contains(lines[i], "·") {
			continue
		}
		for field := range strings.SplitSeq(lines[i], "·") {
			m := contextRatio.FindStringSubmatch(field)
			if m == nil {
				continue
			}
			used, okUsed := parseTokenCount(m[1])
			total, okTotal := parseTokenCount(m[2])
			if !okUsed || !okTotal || total <= 0 {
				continue
			}
			return &ContextUsage{Used: m[1], Total: m[2], Percent: int(used/total*100 + 0.5)}
		}
	}
	return nil
}

func parseTokenCount(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	scale := 1.0
	switch {
	case strings.HasSuffix(s, "k"):
		scale, s = 1_000, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		scale, s = 1_000_000, strings.TrimSuffix(s, "m")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return n * scale, true
}

func tailLines(screen string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(terminal.StripANSI(screen), "\n "), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
