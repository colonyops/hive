package terminal

import (
	"fmt"
	"io"
	"sort"
	"strconv"
)

// PrintPaneTree renders a list of panes grouped by tmux session and window.
// Output is deterministic: sessions sorted alphabetically, windows sorted
// numerically (so "10" follows "9" instead of "1"), panes in input order.
func PrintPaneTree(w io.Writer, panes []PaneDetail) {
	type windowKey struct{ session, windowIndex string }
	windowPanes := make(map[windowKey][]PaneDetail)
	windowNames := make(map[windowKey]string)
	var sessionOrder []string
	sessionSeen := make(map[string]bool)

	for _, p := range panes {
		key := windowKey{p.TmuxSession, p.WindowIndex}
		windowPanes[key] = append(windowPanes[key], p)
		windowNames[key] = p.WindowName
		if !sessionSeen[p.TmuxSession] {
			sessionSeen[p.TmuxSession] = true
			sessionOrder = append(sessionOrder, p.TmuxSession)
		}
	}
	sort.Strings(sessionOrder)

	for _, sess := range sessionOrder {
		_, _ = fmt.Fprintf(w, "%s\n", sess)

		var keys []windowKey
		for k := range windowPanes {
			if k.session == sess {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			return windowIndexLess(keys[i].windowIndex, keys[j].windowIndex)
		})

		for ki, wk := range keys {
			isLastWindow := ki == len(keys)-1
			winConnector := "├─"
			if isLastWindow {
				winConnector = "└─"
			}
			_, _ = fmt.Fprintf(w, "  %s window %s (%s)\n", winConnector, wk.windowIndex, windowNames[wk])

			panePfx := "│ "
			if isLastWindow {
				panePfx = "  "
			}

			wPanes := windowPanes[wk]
			for pi, p := range wPanes {
				isLastPane := pi == len(wPanes)-1
				paneConnector := "├─"
				if isLastPane {
					paneConnector = "└─"
				}

				hive := p.HiveSession
				if hive == "" {
					hive = "(none)"
				}
				tool := p.Tool
				if tool == "" {
					tool = "?"
				}
				fgPID := "-"
				if p.FgPID > 0 {
					fgPID = strconv.FormatInt(p.FgPID, 10)
				}
				panePID := "-"
				if p.PanePID > 0 {
					panePID = strconv.FormatInt(p.PanePID, 10)
				}

				_, _ = fmt.Fprintf(w, "  %s  %s pane %s  hive:%-16s  pid:%-7s  fg:%-7s  tool:%s\n",
					panePfx, paneConnector, p.PaneID, hive, panePID, fgPID, tool)
			}
		}
	}
}

// windowIndexLess sorts numerically when both inputs parse as ints, falling
// back to lexicographic order otherwise. tmux window indices are usually
// integers but can be any string.
func windowIndexLess(a, b string) bool {
	ai, aerr := strconv.Atoi(a)
	bi, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil {
		return ai < bi
	}
	return a < b
}
