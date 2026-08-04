package assess

import (
	"strings"

	"github.com/colonyops/hive/internal/core/terminal"
)

// rule is one prioritized detection rule.
type rule struct {
	id    string
	state State
	hold  bool // transient screen: caller should hold the currently published state
	match func(r regions, title string) (Signal, bool)
}

type ruleSet []rule

// Engine assesses snapshots. Stateless and safe for concurrent use.
type Engine struct {
	ruleSets map[string]ruleSet
	fallback ruleSet
}

// NewEngine builds an Engine with the built-in per-tool rule sets. Tools
// without a dedicated set (including unclassified/"agent"/"shell") fall back
// to the generic rule set.
func NewEngine() *Engine {
	return &Engine{
		ruleSets: map[string]ruleSet{
			"claude": claudeRules,
			"codex":  codexRules,
		},
		fallback: genericRules,
	}
}

// Assess normalizes snap.Content exactly once (StripANSI + NBSP→space),
// computes regions, and runs the tool's rule set in priority order with
// early exit. No rule matched → StateUnknown (never a silent idle). The
// returned Assessment always carries AboveBox, including on no-match, so
// Stage 2 can churn-hash it regardless of classification outcome.
func (e *Engine) Assess(snap Snapshot) Assessment {
	r := computeRegions(normalizeContent(snap.Content))
	aboveBox := r.abovePromptBox()

	rules, ok := e.ruleSets[snap.Tool]
	if !ok {
		rules = e.fallback
	}

	for _, rl := range rules {
		signal, matched := rl.match(r, snap.Title)
		if !matched {
			continue
		}
		return Assessment{
			State:    rl.state,
			Hold:     rl.hold,
			RuleID:   rl.id,
			Signals:  []Signal{signal},
			AboveBox: aboveBox,
		}
	}

	return Assessment{
		State:    StateUnknown,
		AboveBox: aboveBox,
	}
}

// normalizeContent is the engine's single normalization pass: ANSI stripped
// via the existing terminal.StripANSI, then NBSP folded to a regular space so
// downstream exact-match rules and substring phrase checks behave the same
// regardless of which whitespace byte the source terminal used. Rules never
// re-strip; this runs once at entry (and once for the DumpRegions diagnostic
// path, which must see exactly what rules see).
func normalizeContent(content string) string {
	stripped := terminal.StripANSI(content)
	return strings.ReplaceAll(stripped, "\u00a0", " ")
}
