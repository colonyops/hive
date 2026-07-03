package connectorpicker

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"
	"github.com/sahilm/fuzzy"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/styles"
)

// Fixed dialog sizing, mirroring components.InfoDialog: the modal's overall
// width/height are a deterministic function of the terminal size (capped),
// never of the current item/detail content, so navigating the list or
// loading a longer/shorter detail body never resizes or shifts the dialog.
// Content that doesn't fit the fixed content area scrolls instead.
const (
	connectorPickerMaxModalHeight = 36
	connectorPickerModalMargin    = 2
	connectorPickerMinModalWidth  = 72
	// connectorPickerChrome counts the fixed rows View renders around the
	// scrollable content area: ModalStyle border (2), ModalStyle vertical
	// padding (2), title (1), the blank joiners above and below the body
	// (2), and the help line including ModalHelpStyle's MarginTop (2). The
	// search input row lives inside the body (contentHeight), not here.
	// View's rendered height is contentHeight + connectorPickerChrome, so
	// this must match the actual frame or the modal overflows the terminal.
	connectorPickerChrome = 9
)

// connectorPickerGen issues a unique generation token per picker instance.
// Async search/detail results carry the token of the picker that issued
// them, so a late result from a closed picker (possibly for a different
// scope) can never populate a newer picker's cache.
var connectorPickerGen atomic.Int64

// connectorItemsSource implements fuzzy.Source over item titles, for
// local-mode filtering (mirrors commandEntries in command_palette.go).
type connectorItemsSource []connectors.Item

func (c connectorItemsSource) String(i int) string { return c[i].Title }
func (c connectorItemsSource) Len() int            { return len(c) }

// Picker is a two-pane modal: a searchable/filterable item list on
// the left and a detail pane on the right. It reuses the textinput +
// sahilm/fuzzy pattern from CommandPalette for local-mode filtering rather
// than a hand-rolled substring loop; remote-mode search debounces a real
// Connector.Search call per manifest configuration.
type Picker struct {
	conn     connectors.Connector
	manifest connectors.Manifest
	scope    string

	// gen identifies this picker instance in async result messages; see
	// connectorPickerGen.
	gen int64

	input textinput.Model

	// loaded holds the full result set from the last completed Search call;
	// items is the currently displayed set (loaded, or a local-filtered
	// subset of it).
	loaded []connectors.Item
	items  []connectors.Item

	cursor       int
	scrollOffset int

	// searchMode is true while "/" has focused the query input; in the
	// default navigate mode j/k move the cursor and typing is swallowed.
	searchMode bool

	searchInFlight bool
	searchedOnce   bool
	searchErr      error

	// detailCache/detailErr/detailPending are keyed by item ID. Reads and
	// writes to these maps are safe from value-receiver methods because a
	// map is a reference type; only the map header itself must not be
	// reassigned, and it never is here.
	detailCache   map[string]connectors.Detail
	detailErr     map[string]error
	detailPending map[string]bool

	cancelled bool
	selected  bool

	// width/height are the caller's terminal dimensions; modalWidth/
	// modalHeight/listWidth/detailWidth/contentHeight are derived from them
	// once (in New/SetSize) via computePickerDims and
	// never recomputed from content, so the dialog's frame stays a fixed
	// size regardless of how many items match or how long a detail body is.
	width, height                         int
	modalWidth, modalHeight               int
	listWidth, detailWidth, contentHeight int

	// detailVP renders the right-pane detail content within a fixed
	// contentHeight/detailWidth viewport; content longer than that scrolls
	// instead of growing the dialog.
	detailVP viewport.Model
}

// Result is the item and detail selected by the user, if any.
type Result struct {
	Item   connectors.Item
	Detail connectors.Detail
}

// New constructs a picker for conn, using manifest to decide
// layout/columns/search behavior and scope to constrain Search/FetchDetail
// calls (e.g. a GitHub "owner/name" repo). width/height are the caller's
// current terminal dimensions (mirrors NewRepoPicker(repos, currentRepo,
// width, height)) so the picker renders at the real size instead of a fixed
// default that can overflow a small terminal/tmux pane.
func New(conn connectors.Connector, manifest connectors.Manifest, scope string, width, height int) Picker {
	input := textinput.New()
	input.Placeholder = "search..."
	input.Prompt = "/ "
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Focused.Prompt = styles.TextPrimaryStyle
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)

	modalWidth, modalHeight, listWidth, detailWidth, contentHeight := computePickerDims(width, height, manifest)
	input.SetWidth(connectorInputWidth(listWidth))

	p := Picker{
		gen:           connectorPickerGen.Add(1),
		conn:          conn,
		manifest:      manifest,
		scope:         scope,
		input:         input,
		detailCache:   make(map[string]connectors.Detail),
		detailErr:     make(map[string]error),
		detailPending: make(map[string]bool),
		width:         width,
		height:        height,
		modalWidth:    modalWidth,
		modalHeight:   modalHeight,
		listWidth:     listWidth,
		detailWidth:   detailWidth,
		contentHeight: contentHeight,
		detailVP:      viewport.New(viewport.WithWidth(detailWidth), viewport.WithHeight(contentHeight)),
	}
	p.refreshDetailViewport()
	return p
}

// computePickerDims derives the picker's fixed modal/pane
// dimensions from the caller's terminal size and the manifest's column
// layout. The result depends only on width/height/manifest — never on the
// current item count or detail content — so the dialog never resizes while
// the user browses.
func computePickerDims(width, height int, manifest connectors.Manifest) (modalWidth, modalHeight, listWidth, detailWidth, contentHeight int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	modalWidth = min(max(int(float64(width)*0.92), connectorPickerMinModalWidth), max(width-connectorPickerModalMargin, connectorPickerMinModalWidth))
	modalHeight = min(max(height-connectorPickerModalMargin, connectorPickerChrome+3), connectorPickerMaxModalHeight)
	contentHeight = max(modalHeight-connectorPickerChrome, 3)

	// available excludes the ModalStyle border (2) and horizontal padding
	// (4) plus the gap between panes.
	available := max(modalWidth-8, 20)

	if manifest.Picker.HidePreview {
		// Single-pane mode: the list/table gets the full modal width and
		// no detail pane is rendered.
		return modalWidth, modalHeight, available, 0, contentHeight
	}

	listRatio := 34
	if manifest.Picker.Layout == connectors.LayoutModeList {
		// List-layout issue pickers use two-line cards, so give the scan
		// column enough room for title + metadata while preserving preview
		// space on the right.
		listRatio = 42
	}
	listWidth = max(available*listRatio/100, minListWidthForManifest(manifest))
	detailWidth = available - listWidth
	if detailWidth < 20 {
		detailWidth = 20
		listWidth = max(available-detailWidth, 10)
	}
	return modalWidth, modalHeight, listWidth, detailWidth, contentHeight
}

// Init issues the initial (empty-query) Search call to populate the list.
// It takes a pointer receiver so the searchInFlight flag it sets survives
// on the stored picker (a value receiver would mutate a discarded copy and
// the list would render blank instead of "searching..." until the first
// result lands).
func (p *Picker) Init() tea.Cmd {
	return p.startSearch("")
}

// connectorInputWidth sizes the search input to fit inside the left pane
// (accounting for the "/ " prompt) so long queries scroll within the input
// instead of widening the pane and shifting the layout.
func connectorInputWidth(listWidth int) int {
	return max(listWidth-4, 10)
}

// startSearch marks a search in flight and returns the Cmd that performs it.
func (p *Picker) startSearch(query string) tea.Cmd {
	p.searchInFlight = true
	p.searchErr = nil
	return connectorSearchCmd(p.gen, p.conn, p.scope, query)
}

// SetSize updates the picker's rendering dimensions, recomputing the fixed
// modal/pane sizes and resizing the detail viewport to match.
func (p *Picker) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.modalWidth, p.modalHeight, p.listWidth, p.detailWidth, p.contentHeight = computePickerDims(width, height, p.manifest)
	p.input.SetWidth(connectorInputWidth(p.listWidth))
	p.detailVP.SetWidth(p.detailWidth)
	p.detailVP.SetHeight(p.contentHeight)
	p.refreshDetailViewport()
}

// Update handles key events and connector RPC result messages.
func (p Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		return p.handleKey(m)
	case connectorSearchResultMsg:
		return p.handleSearchResult(m)
	case connectorSearchErrorMsg:
		return p.handleSearchError(m)
	case connectorSearchDebounceMsg:
		return p.handleDebounce(m)
	case connectorDetailResultMsg:
		return p.handleDetailResult(m)
	case connectorDetailErrorMsg:
		return p.handleDetailError(m)
	}
	return p, nil
}

// handleKey dispatches navigation/selection keys, then falls through to the
// text input for query editing.
func (p Picker) handleKey(msg tea.KeyPressMsg) (Picker, tea.Cmd) {
	// Keys shared by both modes: arrows always navigate, enter always
	// selects, and detail scrolling stays available while typing.
	switch msg.String() {
	case "esc":
		if p.searchMode {
			// Leave search mode, keeping the current query/filter; a
			// second esc (in navigate mode) dismisses the picker.
			p.searchMode = false
			p.input.Blur()
			return p, nil
		}
		p.cancelled = true
		return p, nil
	case "enter":
		if len(p.items) > 0 && p.cursor < len(p.items) {
			p.selected = true
		}
		return p, nil
	case "up", "ctrl+k":
		return p.moveCursor(-1)
	case "down", "ctrl+j":
		return p.moveCursor(1)
	case "pgdown", "ctrl+d":
		p.detailVP.HalfPageDown()
		return p, nil
	case "pgup", "ctrl+u":
		p.detailVP.HalfPageUp()
		return p, nil
	}

	if !p.searchMode {
		return p.handleNavigateKey(msg)
	}

	before := p.input.Value()
	var inputCmd tea.Cmd
	p.input, inputCmd = p.input.Update(msg)
	if p.input.Value() == before {
		// Non-editing keys (left/right/home/end, etc.) must not reset the
		// list cursor or re-issue a remote search for an unchanged query.
		return p, inputCmd
	}
	return p.handleQueryChanged(inputCmd)
}

// handleNavigateKey handles keys in navigate mode (the default): j/k move
// the cursor, "/" enters search mode, "O" opens the highlighted item's URL
// in the browser. Anything else is swallowed so stray typing can't mutate
// the query.
func (p Picker) handleNavigateKey(msg tea.KeyPressMsg) (Picker, tea.Cmd) {
	switch msg.String() {
	case "j":
		return p.moveCursor(1)
	case "k":
		return p.moveCursor(-1)
	case "/":
		p.searchMode = true
		return p, p.input.Focus()
	case "O":
		return p, p.openCurrentItemURL()
	}
	return p, nil
}

// moveCursor moves the highlight by delta, keeping it in range, and
// triggers the lazy detail fetch for the newly highlighted item.
func (p Picker) moveCursor(delta int) (Picker, tea.Cmd) {
	next := p.cursor + delta
	if next >= 0 && next < len(p.items) {
		p.cursor = next
		p.clampScroll()
		p.refreshDetailViewport()
	}
	return p, p.triggerDetailFetch()
}

// openCurrentItemURL returns a Cmd that opens the highlighted item's URI in
// the OS browser, or nil when there is no item or it carries no URI.
func (p Picker) openCurrentItemURL() tea.Cmd {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return nil
	}
	uri := p.items[p.cursor].URI
	if uri == "" {
		return nil
	}
	return func() tea.Msg {
		if err := browserOpenCmd(uri).Run(); err != nil {
			log.Debug().Err(err).Str("uri", uri).Msg("connector picker: open url failed")
		}
		return nil
	}
}

// browserOpenCmd builds the OS-specific command to open uri in the default
// browser.
func browserOpenCmd(uri string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", uri)
	default:
		return exec.Command("xdg-open", uri)
	}
}

// handleQueryChanged reacts to a query edit according to the manifest's
// search mode: local mode re-filters already-loaded items in memory; remote
// mode schedules a debounced Search call.
func (p Picker) handleQueryChanged(inputCmd tea.Cmd) (Picker, tea.Cmd) {
	query := p.input.Value()

	if p.manifest.Picker.Search.Mode != connectors.SearchModeRemote {
		p.applyLocalFilter(query)
		return p, tea.Batch(inputCmd, p.triggerDetailFetch())
	}

	delay := connectorDebounceDelay(p.manifest)
	return p, tea.Batch(inputCmd, connectorDebounceCmd(p.gen, query, delay))
}

// applyLocalFilter narrows p.items to fuzzy matches of query against
// p.loaded. An empty query resets to the full loaded set.
func (p *Picker) applyLocalFilter(query string) {
	if query == "" {
		p.items = p.loaded
	} else {
		matches := fuzzy.FindFrom(query, connectorItemsSource(p.loaded))
		items := make([]connectors.Item, len(matches))
		for i, match := range matches {
			items[i] = p.loaded[match.Index]
		}
		p.items = items
	}
	p.cursor = 0
	p.scrollOffset = 0
	p.refreshDetailViewport()
}

// handleDebounce issues the actual remote Search call once the debounce
// delay elapses, but only if the query hasn't changed since the debounce
// was scheduled (otherwise this tick is stale and is dropped).
func (p Picker) handleDebounce(msg connectorSearchDebounceMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen || msg.Query != p.input.Value() {
		return p, nil
	}
	return p, p.startSearch(msg.Query)
}

// handleSearchResult applies a completed Search response, discarding it if
// the query it answers no longer matches the current input (a stale/
// superseded response).
func (p Picker) handleSearchResult(msg connectorSearchResultMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen || msg.Query != p.input.Value() {
		return p, nil
	}
	p.searchInFlight = false
	p.searchedOnce = true
	p.searchErr = nil
	p.loaded = msg.Items
	p.items = msg.Items
	p.cursor = 0
	p.scrollOffset = 0
	p.refreshDetailViewport()
	return p, p.triggerDetailFetch()
}

// handleSearchError records a Search failure, discarding it if stale.
func (p Picker) handleSearchError(msg connectorSearchErrorMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen || msg.Query != p.input.Value() {
		return p, nil
	}
	p.searchInFlight = false
	p.searchedOnce = true
	p.searchErr = msg.Err
	p.loaded = nil
	p.items = nil
	p.cursor = 0
	p.scrollOffset = 0
	p.refreshDetailViewport()
	return p, nil
}

// handleDetailResult caches a completed FetchDetail response. The viewport
// only refreshes (resetting scroll to top) when this result is for the
// currently highlighted item; a response for an item the user has since
// moved past must not yank their scroll position on the item they're
// reading now.
func (p Picker) handleDetailResult(msg connectorDetailResultMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen {
		return p, nil
	}
	delete(p.detailPending, msg.ID)
	delete(p.detailErr, msg.ID)
	p.detailCache[msg.ID] = msg.Detail
	if p.isCurrentItemID(msg.ID) {
		p.refreshDetailViewport()
	}
	return p, nil
}

// handleDetailError records a FetchDetail failure for the detail pane, same
// current-item guard as handleDetailResult.
func (p Picker) handleDetailError(msg connectorDetailErrorMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen {
		return p, nil
	}
	delete(p.detailPending, msg.ID)
	p.detailErr[msg.ID] = msg.Err
	if p.isCurrentItemID(msg.ID) {
		p.refreshDetailViewport()
	}
	return p, nil
}

// isCurrentItemID reports whether id is the ID of the currently highlighted
// item.
func (p Picker) isCurrentItemID(id string) bool {
	return p.cursor >= 0 && p.cursor < len(p.items) && p.items[p.cursor].ID == id
}

// triggerDetailFetch issues a lazy FetchDetail call for the currently
// highlighted item if the connector supports detail, the item doesn't
// already carry inline detail, and it isn't already cached or in flight.
func (p Picker) triggerDetailFetch() tea.Cmd {
	if !p.manifest.Capabilities.FetchDetail {
		return nil
	}
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return nil
	}

	item := p.items[p.cursor]
	if item.Detail.Kind() != connectors.DetailKindNone {
		return nil
	}
	if _, cached := p.detailCache[item.ID]; cached {
		return nil
	}
	if p.detailPending[item.ID] {
		return nil
	}

	p.detailPending[item.ID] = true
	return connectorFetchDetailCmd(p.gen, p.conn, p.scope, item)
}

// clampScroll keeps the cursor within the visible scroll window.
func (p *Picker) clampScroll() {
	visible := p.visibleCount()
	if p.cursor < p.scrollOffset {
		p.scrollOffset = p.cursor
	} else if p.cursor >= p.scrollOffset+visible {
		p.scrollOffset = p.cursor - visible + 1
	}
	maxOffset := max(len(p.items)-visible, 0)
	p.scrollOffset = min(max(p.scrollOffset, 0), maxOffset)
}

// visibleCount bounds how many rows the left pane renders at once; the
// remainder scrolls with the cursor. It is fixed to p.contentHeight (set
// once from terminal size in computePickerDims), not to the
// current item count, so the list pane's rendered height never changes as
// results come and go — renderList pads short lists to this height.
func (p Picker) visibleCount() int {
	if p.manifest.Picker.Layout == connectors.LayoutModeList {
		return min(len(p.items), max((p.listHeight()+1)/3, 1))
	}
	return min(len(p.items), p.listHeight())
}

func (p Picker) listHeight() int {
	// The left pane body is input + blank separator + rows. Keep the whole
	// pane exactly p.contentHeight rows so navigating items cannot resize the
	// modal.
	return max(p.contentHeight-2, 1)
}

// minListWidthForManifest returns the minimum left-pane width needed to
// render the manifest's table columns without truncation, or a sane
// default for list layout.
func minListWidthForManifest(manifest connectors.Manifest) int {
	if manifest.Picker.Layout != connectors.LayoutModeTable || len(manifest.Picker.Columns) == 0 {
		return 28
	}
	total := 2 // leading cursor column
	for i, col := range manifest.Picker.Columns {
		width := col.Width
		if width <= 0 {
			width = 12
		}
		total += width
		if i > 0 {
			total++ // inter-column space
		}
	}
	return total
}

// Selected returns the highlighted item and its best-known detail, if the
// user pressed enter on a non-empty list.
func (p Picker) Selected() (Result, bool) {
	if !p.selected || p.cursor < 0 || p.cursor >= len(p.items) {
		return Result{}, false
	}
	item := p.items[p.cursor]
	detail := item.Detail
	if detail.Kind() == connectors.DetailKindNone {
		if cached, ok := p.detailCache[item.ID]; ok {
			detail = cached
		}
	}
	return Result{Item: item, Detail: detail}, true
}

// Cancelled reports whether the user dismissed the picker.
func (p Picker) Cancelled() bool {
	return p.cancelled
}

// View renders the two-pane picker: item list on the left, detail on the
// right. The overall frame size (modalWidth/modalHeight) and both pane
// sizes are fixed — derived only from terminal size, never from the current
// item count or detail content — so browsing never shifts the dialog.
func (p Picker) View() string {
	title := styles.ModalTitleStyle.Render(p.manifest.DisplayName)

	left := lipgloss.JoinVertical(lipgloss.Left,
		p.input.View(),
		"",
		p.renderList(p.listWidth),
	)

	body := lipgloss.NewStyle().Width(p.listWidth).Height(p.contentHeight).MaxHeight(p.contentHeight).Render(left)
	if !p.manifest.Picker.HidePreview {
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			body,
			renderPickerDivider(p.contentHeight),
			lipgloss.NewStyle().Width(p.detailWidth).Height(p.contentHeight).MaxHeight(p.contentHeight).Padding(0, 0, 0, 2).Render(p.detailVP.View()),
		)
	}

	help := styles.ModalHelpStyle.Render(p.helpText())

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help)
	return styles.ModalStyle.Width(p.modalWidth).Render(content)
}

// helpText returns the mode-appropriate key hints for the help line. Kept
// short enough for an 80-column terminal; less-common keys (ctrl+u/d
// detail scrolling, pgup/pgdn) stay undocumented rather than pushing
// "esc cancel" past the modal edge.
func (p Picker) helpText() string {
	if p.searchMode {
		return "type to search  ↑/↓ navigate  enter select  esc done"
	}
	return "j/k navigate  / search  O open  enter select  esc cancel"
}

// renderPickerDivider renders the vertical divider between the
// list and detail panes at the given height.
func renderPickerDivider(height int) string {
	if height <= 0 {
		return ""
	}
	return styles.TextMutedStyle.Render(strings.Repeat("│\n", height-1) + "│")
}

// renderList renders the left-pane item rows, honoring the manifest's
// layout (list vs table) and current search/loading state. Output is
// always p.listHeight() lines regardless of how many items match, so the
// list pane never changes height as results come and go.
func (p Picker) renderList(width int) string {
	rowHeight := p.listHeight()
	lines := make([]string, 0, rowHeight)
	appendStatus := func(line string) string {
		lines = append(lines, line)
		for len(lines) < rowHeight {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}

	if p.searchErr != nil {
		return appendStatus(styles.TextErrorStyle.Render("search failed: " + p.searchErr.Error()))
	}
	if p.searchInFlight {
		return appendStatus(styles.TextMutedStyle.Render("searching..."))
	}
	if p.searchedOnce && len(p.items) == 0 {
		return appendStatus(styles.TextMutedStyle.Render("no results"))
	}

	visible := p.visibleCount()
	for i := range visible {
		idx := i + p.scrollOffset
		if idx >= len(p.items) {
			break
		}
		for _, line := range strings.Split(p.renderRow(p.items[idx], idx == p.cursor, width), "\n") {
			if len(lines) >= rowHeight {
				break
			}
			lines = append(lines, line)
		}
		if p.manifest.Picker.Layout == connectors.LayoutModeList && idx < len(p.items)-1 && len(lines) < rowHeight {
			lines = append(lines, "")
		}
	}
	for len(lines) < rowHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderRow renders a single item row, using table columns when the
// manifest declares a table layout, or title/subtitle otherwise.
func (p Picker) renderRow(item connectors.Item, selected bool, width int) string {
	cursor := "  "
	rowStyle := styles.TextForegroundStyle
	if selected {
		cursor = styles.TextPrimaryStyle.Render(styles.IconSelector + " ")
		rowStyle = styles.TextPrimaryBoldStyle
	}

	if p.manifest.Picker.Layout == connectors.LayoutModeTable && len(p.manifest.Picker.Columns) > 0 {
		line := cursor + renderConnectorTableRow(item, p.manifest.Picker.Columns, max(width-2, 1), selected)
		return lipgloss.NewStyle().MaxWidth(width).Render(line)
	}

	title := rowStyle.Render(item.Title)
	row1 := lipgloss.NewStyle().MaxWidth(width).Render(cursor + title)

	meta := renderConnectorListMeta(item, max(width-2, 1))
	if meta == "" {
		return row1
	}
	return row1 + "\n" + lipgloss.NewStyle().MaxWidth(width).Render("  "+meta)
}

// connectorFlexColumnMinWidth is the floor a Flex column can be squeezed
// to before the row simply truncates at the pane edge.
const connectorFlexColumnMinWidth = 8

// resolveConnectorColumnWidths assigns single-line rendering widths to
// columns within total: declared fixed widths are kept as-is (defaulting
// to 12 when a column declares neither Width nor Flex), and Flex columns
// share whatever space remains proportionally to their weights.
func resolveConnectorColumnWidths(columns []connectors.Column, total int) []int {
	widths := make([]int, len(columns))
	remaining := total - max(len(columns)-1, 0) // inter-column spaces
	flexTotal := 0
	for i, col := range columns {
		switch {
		case col.Flex > 0:
			flexTotal += col.Flex
		case col.Width > 0:
			widths[i] = col.Width
			remaining -= col.Width
		default:
			widths[i] = 12
			remaining -= 12
		}
	}
	if flexTotal == 0 {
		return widths
	}
	for i, col := range columns {
		if col.Flex > 0 {
			widths[i] = max(remaining*col.Flex/flexTotal, connectorFlexColumnMinWidth)
		}
	}
	return widths
}

// renderConnectorTableRow renders an item's Fields as a single row: fixed
// columns at their declared widths, Flex columns sharing the remaining
// space. Cell content is truncated (never wrapped) so a long title cannot
// turn one table row into several lines and break the picker's fixed-height
// layout.
func renderConnectorTableRow(item connectors.Item, columns []connectors.Column, width int, selected bool) string {
	widths := resolveConnectorColumnWidths(columns, width)
	cells := make([]string, 0, len(columns))
	for i, col := range columns {
		w := max(widths[i], 1)
		value := connectorFieldString(item, col.Key)
		// Number columns render as "#42", matching the list layout's
		// metadata convention.
		if col.Key == "number" && value != "" {
			value = "#" + value
		}
		style := tableCellStyle(col.Key, value)
		if selected {
			// The highlighted row stays a uniform primary-bold bar so the
			// cursor position is scannable; semantic colors show on the
			// rest of the table.
			style = styles.TextPrimaryBoldStyle
		}
		value = ansi.Truncate(statusIcon(value)+value, w, "…")
		cells = append(cells, lipgloss.NewStyle().Width(w).MaxHeight(1).Render(style.Render(value)))
	}
	return strings.Join(cells, " ")
}

// statusIcon returns a glyph prefix for CI status vocabulary so pass/fail
// state is scannable without reading the words. Styled by the same
// tableCellStyle color as the text.
func statusIcon(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "passing":
		return "✓ "
	case "failing":
		return "✗ "
	case "pending":
		return "● "
	}
	return ""
}

// tableCellStyle picks a semantic style for a table cell from its column
// key and value. Identifier columns render in the primary accent, authors
// muted, and well-known status vocabulary (shared by issues/PRs and usable
// by external connectors emitting the same terms) gets state colors. This
// is a presentation concern, so it lives here rather than in manifests.
func tableCellStyle(key, value string) lipgloss.Style {
	switch key {
	case "number", "id", "index":
		return styles.TextPrimaryStyle
	case "author":
		return styles.TextMutedStyle
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved", "open", "success", "passing":
		return styles.TextSuccessStyle
	case "changes requested", "closed", "failure", "failed", "failing":
		return styles.TextErrorStyle
	case "review required", "pending":
		return styles.TextWarningStyle
	case "draft":
		return styles.TextMutedStyle
	case "merged":
		return styles.TextSecondaryStyle
	}
	return styles.TextForegroundStyle
}

// connectorFieldString resolves a column key against an item's well-known
// fields (id/title/subtitle) or its Fields map, returning "" if absent.
func connectorFieldString(item connectors.Item, key string) string {
	switch key {
	case "id":
		return item.ID
	case "title":
		return item.Title
	case "subtitle":
		return item.Subtitle
	}
	if item.Fields == nil {
		return ""
	}
	if v, ok := item.Fields[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func connectorFieldStringSlice(item connectors.Item, key string) []string {
	if item.Fields == nil {
		return nil
	}
	value, ok := item.Fields[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return nonEmptyStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, v := range typed {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				values = append(values, s)
			}
		}
		return values
	default:
		if s := strings.TrimSpace(fmt.Sprintf("%v", value)); s != "" {
			return []string{s}
		}
		return nil
	}
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func renderConnectorListMeta(item connectors.Item, width int) string {
	parts := connectorMetadataParts(item)
	if len(parts) == 0 {
		if item.Subtitle == "" {
			return ""
		}
		return styles.TextMutedStyle.Render(item.Subtitle)
	}
	return lipgloss.NewStyle().MaxWidth(max(width, 1)).Render(strings.Join(parts, "  "))
}

func renderConnectorDetailMeta(item connectors.Item, width int) string {
	parts := connectorMetadataParts(item)
	if len(parts) == 0 {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(max(width, 1)).Render(strings.Join(parts, "  "))
}

func connectorMetadataParts(item connectors.Item) []string {
	parts := make([]string, 0, 6)
	if number := connectorFieldString(item, "number"); number != "" {
		parts = append(parts, styles.TextPrimaryStyle.Render("#"+number))
	}
	if author := connectorFieldString(item, "author"); author != "" {
		parts = append(parts, styles.TextMutedStyle.Render("@"+author))
	}
	for _, label := range connectorFieldStringSlice(item, "labels") {
		parts = append(parts, styles.TextSecondaryStyle.Render("["+label+"]"))
	}
	return parts
}

// detailContent computes the detail pane text for the currently highlighted
// item, honoring inline item detail, cache, in-flight, and error states in
// that priority order. It does not itself bound the output to any
// height/width — that's the detail viewport's job (see refreshDetailViewport).
func (p Picker) detailContent() string {
	if len(p.items) == 0 || p.cursor < 0 || p.cursor >= len(p.items) {
		return ""
	}

	item := p.items[p.cursor]
	title := lipgloss.NewStyle().MaxWidth(p.detailWidth).Render(styles.TextForegroundBoldStyle.Render(item.Title))
	meta := renderConnectorDetailMeta(item, p.detailWidth)

	var body string
	if item.Detail.Kind() != connectors.DetailKindNone {
		body = renderConnectorDetail(item.Detail, p.detailWidth)
	} else if err, ok := p.detailErr[item.ID]; ok {
		body = styles.TextErrorStyle.Render("detail failed: " + err.Error())
	} else if detail, ok := p.detailCache[item.ID]; ok {
		body = renderConnectorDetail(detail, p.detailWidth)
	} else if p.detailPending[item.ID] {
		body = styles.TextMutedStyle.Render("loading detail...")
	} else {
		body = renderConnectorDetail(connectors.Detail{}, p.detailWidth)
	}

	header := title
	if meta != "" {
		header += "\n" + meta
	}
	if strings.TrimSpace(body) == "" {
		return header
	}
	return header + "\n\n" + body
}

// refreshDetailViewport recomputes the detail pane content for the
// currently highlighted item and loads it into the fixed-size detail
// viewport, resetting scroll to the top. Content longer than
// p.contentHeight scrolls within the viewport instead of growing the
// dialog.
func (p *Picker) refreshDetailViewport() {
	if p.manifest.Picker.HidePreview {
		return
	}
	p.detailVP.SetContent(p.detailContent())
	p.detailVP.GotoTop()
}

// connectorDebounceDelay resolves the remote-search debounce delay from the
// manifest, falling back to defaultConnectorSearchDebounce when unset.
func connectorDebounceDelay(manifest connectors.Manifest) time.Duration {
	if manifest.Picker.Search.DebounceMS <= 0 {
		return defaultConnectorSearchDebounce
	}
	return time.Duration(manifest.Picker.Search.DebounceMS) * time.Millisecond
}
