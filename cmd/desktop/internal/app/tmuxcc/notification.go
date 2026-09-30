package tmuxcc

import (
	"bytes"
	"errors"
	"fmt"
)

// Notification is a parsed %-line the gateway dispatches to the controller.
type Notification interface{ isNotification() }

type (
	// OutputNotification carries already-decoded pane bytes.
	OutputNotification struct {
		Pane string
		Data []byte
	}
	WindowAddNotification     struct{ Window string }
	WindowCloseNotification   struct{ Window string }
	WindowRenamedNotification struct{ Window, Name string }
	WindowPaneChanged         struct{ Window, Pane string }
	SessionChanged            struct{ Session, Name string }
	SessionWindowChanged      struct{ Session, Window string }
	// LayoutChanged carries the window's new pane tree and whether its active
	// pane is zoomed over it. Layout is the zero value when the string could
	// not be read; the reconcile the same notification triggers carries the
	// tree authoritatively.
	LayoutChanged struct {
		Window string
		Layout Layout
		Zoomed bool
	}
	PauseNotification    struct{ Pane string }
	ContinueNotification struct{ Pane string }
	ExitNotification     struct{ Reason string }
)

func (OutputNotification) isNotification()        {}
func (WindowAddNotification) isNotification()     {}
func (WindowCloseNotification) isNotification()   {}
func (WindowRenamedNotification) isNotification() {}
func (WindowPaneChanged) isNotification()         {}
func (SessionChanged) isNotification()            {}
func (SessionWindowChanged) isNotification()      {}
func (LayoutChanged) isNotification()             {}
func (PauseNotification) isNotification()         {}
func (ContinueNotification) isNotification()      {}
func (ExitNotification) isNotification()          {}

var (
	// errUnknownNotification marks a %-line this client does not model. tmux
	// gains notifications over time, so it is a debug-level drop.
	errUnknownNotification = errors.New("tmuxcc: unknown notification")
	errMalformed           = errors.New("tmuxcc: malformed notification")
)

// parseNotification decodes one %-line. line must not be retained by the
// caller; every field the result carries is copied out of it.
func parseNotification(line []byte) (Notification, error) {
	name, rest, hasArgs := bytes.Cut(line, []byte(" "))
	switch string(name) {
	case "%output":
		pane, data, ok := bytes.Cut(rest, []byte(" "))
		if !ok || !isPaneID(pane) {
			return nil, fmt.Errorf("%w: %%output", errMalformed)
		}
		return OutputNotification{Pane: string(pane), Data: decodeOutput(data)}, nil

	case "%extended-output":
		return parseExtendedOutput(rest)

	case "%window-add":
		if !isWindowID(rest) {
			return nil, fmt.Errorf("%w: %%window-add", errMalformed)
		}
		return WindowAddNotification{Window: string(rest)}, nil

	// tmux picks between the two by whether the window is still linked into the
	// client's session when the *deferred* notification fires — and for
	// kill-window it no longer is, so a window we track dies as
	// %unlinked-window-close. Both mean the same thing to us; a window we never
	// tracked produces no event either way.
	case "%window-close", "%unlinked-window-close":
		if !isWindowID(rest) {
			return nil, fmt.Errorf("%w: %%window-close", errMalformed)
		}
		return WindowCloseNotification{Window: string(rest)}, nil

	case "%window-renamed":
		win, name, ok := bytes.Cut(rest, []byte(" "))
		if !ok || !isWindowID(win) {
			return nil, fmt.Errorf("%w: %%window-renamed", errMalformed)
		}
		return WindowRenamedNotification{Window: string(win), Name: string(name)}, nil

	case "%window-pane-changed":
		win, pane, ok := bytes.Cut(rest, []byte(" "))
		if !ok || !isWindowID(win) || !isPaneID(pane) {
			return nil, fmt.Errorf("%w: %%window-pane-changed", errMalformed)
		}
		return WindowPaneChanged{Window: string(win), Pane: string(pane)}, nil

	case "%session-changed":
		sess, name, ok := bytes.Cut(rest, []byte(" "))
		if !ok || !isSessionID(sess) {
			return nil, fmt.Errorf("%w: %%session-changed", errMalformed)
		}
		return SessionChanged{Session: string(sess), Name: string(name)}, nil

	case "%session-window-changed":
		sess, win, ok := bytes.Cut(rest, []byte(" "))
		if !ok || !isSessionID(sess) || !isWindowID(win) {
			return nil, fmt.Errorf("%w: %%session-window-changed", errMalformed)
		}
		return SessionWindowChanged{Session: string(sess), Window: string(win)}, nil

	case "%layout-change":
		win, args, _ := bytes.Cut(rest, []byte(" "))
		if !isWindowID(win) {
			return nil, fmt.Errorf("%w: %%layout-change", errMalformed)
		}
		layout, zoomed := parseLayoutChange(args)
		return LayoutChanged{Window: string(win), Layout: layout, Zoomed: zoomed}, nil

	case "%pause":
		if !isPaneID(rest) {
			return nil, fmt.Errorf("%w: %%pause", errMalformed)
		}
		return PauseNotification{Pane: string(rest)}, nil

	case "%continue":
		if !isPaneID(rest) {
			return nil, fmt.Errorf("%w: %%continue", errMalformed)
		}
		return ContinueNotification{Pane: string(rest)}, nil

	case "%exit":
		if !hasArgs {
			return ExitNotification{}, nil
		}
		return ExitNotification{Reason: string(rest)}, nil
	}

	return nil, errUnknownNotification
}

// parseExtendedOutput handles `%extended-output %<pane> <age> [unknown...] : <escaped>`.
// The payload starts after the first " : " past the age field — not the last,
// because an unescaped " : " inside the payload itself is legal.
func parseExtendedOutput(rest []byte) (Notification, error) {
	pane, tail, ok := bytes.Cut(rest, []byte(" "))
	if !ok || !isPaneID(pane) {
		return nil, fmt.Errorf("%w: %%extended-output", errMalformed)
	}
	age, tail, ok := bytes.Cut(tail, []byte(" "))
	if !ok || !isDigits(age) {
		return nil, fmt.Errorf("%w: %%extended-output age", errMalformed)
	}
	if bytes.HasPrefix(tail, []byte(": ")) {
		return OutputNotification{Pane: string(pane), Data: decodeOutput(tail[2:])}, nil
	}
	_, payload, ok := bytes.Cut(tail, []byte(" : "))
	if !ok {
		return nil, fmt.Errorf("%w: %%extended-output separator", errMalformed)
	}
	return OutputNotification{Pane: string(pane), Data: decodeOutput(payload)}, nil
}

// parseLayoutChange reads `<layout> <visible-layout> <flags>`. The first
// layout is the window's unzoomed tree, which is what the renderer lays panes
// out from; the visible one differs only under zoom, which the `Z` flag says
// outright. An unreadable layout yields the zero value rather than an error,
// because the reconcile the same notification triggers carries the tree
// authoritatively.
func parseLayoutChange(args []byte) (Layout, bool) {
	fields := bytes.Fields(args)
	if len(fields) == 0 {
		return Layout{}, false
	}
	layout, err := ParseLayout(string(fields[0]))
	if err != nil {
		return Layout{}, false
	}
	zoomed := len(fields) >= 3 && bytes.ContainsRune(fields[2], 'Z')
	return layout, zoomed
}

func atoi(b []byte) int {
	if !isDigits(b) || len(b) > 6 {
		return 0
	}
	n := 0
	for _, c := range b {
		n = n*10 + int(c-'0')
	}
	return n
}

// decodeOutput folds tmux's octal escapes back into raw bytes. tmux escapes
// every byte below 0x20 and the backslash itself, so a bare control byte in
// the payload is line-driver noise and is dropped, and a malformed escape
// yields '?' — the same recovery iTerm2 uses.
func decodeOutput(escaped []byte) []byte {
	out := make([]byte, 0, len(escaped))
	for i := 0; i < len(escaped); i++ {
		c := escaped[i]
		if c < 0x20 {
			continue
		}
		if c != '\\' {
			out = append(out, c)
			continue
		}
		value, digits := 0, 0
		for digits < 3 && i+1+digits < len(escaped) && isOctal(escaped[i+1+digits]) {
			value = value<<3 | int(escaped[i+1+digits]-'0')
			digits++
		}
		if digits == 3 {
			out = append(out, byte(value))
		} else {
			out = append(out, '?')
		}
		i += digits
	}
	return out
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

func isDigits(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func hasIDForm(b []byte, sigil byte) bool {
	return len(b) > 1 && b[0] == sigil && isDigits(b[1:])
}

func isWindowID(b []byte) bool  { return hasIDForm(b, '@') }
func isPaneID(b []byte) bool    { return hasIDForm(b, '%') }
func isSessionID(b []byte) bool { return hasIDForm(b, '$') }

// validWindowID and validPaneID gate ids that reach a tmux command line. Both
// arrive from the frontend, so anything but @<digits> or %<digits> is refused
// rather than interpolated into a command.
func validWindowID(id string) bool { return hasIDForm([]byte(id), '@') }
func validPaneID(id string) bool   { return hasIDForm([]byte(id), '%') }
