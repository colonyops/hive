package tmux

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/colonyops/hive/internal/core/terminal/classifier"
)

// PaneLister abstracts tmux pane enumeration.
type PaneLister interface {
	// ListAllPanes returns raw pane data for all panes in all tmux sessions.
	ListAllPanes() ([]classifier.PaneInput, error)
}

// TmuxPaneLister calls tmux list-panes -a and parses the output.
type TmuxPaneLister struct{}

// listPanesFormat is the delimited format used for `tmux list-panes -a`.
// tmux replaces literal tab format separators with underscores, so use a
// printable delimiter that survives format rendering.
// Fields: session_name, window_index, window_name, pane_current_path,
// window_activity, pane_id, pane_pid, pane_title, @hive-session.
const listPanesDelimiter = "|||"

const listPanesFormat = "#{session_name}" + listPanesDelimiter + "#{window_index}" + listPanesDelimiter + "#{window_name}" + listPanesDelimiter +
	"#{pane_current_path}" + listPanesDelimiter + "#{window_activity}" + listPanesDelimiter + "#{pane_id}" + listPanesDelimiter +
	"#{pane_pid}" + listPanesDelimiter + "#{pane_title}" + listPanesDelimiter + "#{@hive-session}"

// paneLine is the parsed form of one line of `tmux list-panes` output.
type paneLine struct {
	sessName    string
	winIdx      string
	winName     string
	workDir     string
	activity    int64
	paneID      string
	panePID     int64
	paneTitle   string
	hiveSession string
}

// ListAllPanes returns all tmux panes visible to the current tmux client.
func (TmuxPaneLister) ListAllPanes() ([]classifier.PaneInput, error) {
	output, err := exec.Command("tmux", "list-panes", "-a", "-F", listPanesFormat).Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes failed: %w", err)
	}
	return parsePaneList(string(output)), nil
}

func parsePaneList(output string) []classifier.PaneInput {
	var panes []classifier.PaneInput
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		pl, ok := parsePaneLine(line)
		if !ok {
			continue
		}
		panes = append(panes, paneInputFromLine(pl))
	}
	return panes
}

func paneInputFromLine(pl paneLine) classifier.PaneInput {
	return classifier.PaneInput{
		SessionName: pl.sessName,
		PaneID:      pl.paneID,
		PanePID:     pl.panePID,
		WindowIndex: pl.winIdx,
		WindowName:  pl.winName,
		PaneTitle:   pl.paneTitle,
		WorkDir:     pl.workDir,
		Activity:    pl.activity,
		HiveSession: pl.hiveSession,
	}
}

// parsePaneLine parses one delimited line in listPanesFormat.
func parsePaneLine(line string) (paneLine, bool) {
	parts := strings.SplitN(line, listPanesDelimiter, 9)
	if len(parts) < 6 || line == "" {
		return paneLine{}, false
	}
	pl := paneLine{
		sessName: parts[0],
		winIdx:   parts[1],
		winName:  parts[2],
		workDir:  parts[3],
		paneID:   parts[5],
	}
	_, _ = fmt.Sscanf(parts[4], "%d", &pl.activity)
	if len(parts) >= 7 {
		_, _ = fmt.Sscanf(parts[6], "%d", &pl.panePID)
	}
	if len(parts) >= 8 {
		pl.paneTitle = parts[7]
	}
	if len(parts) >= 9 {
		pl.hiveSession = strings.TrimSpace(parts[8])
	}
	return pl, true
}
