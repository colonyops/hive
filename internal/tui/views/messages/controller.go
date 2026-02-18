package messages

import (
	"strings"

	"github.com/colonyops/hive/internal/core/messaging"
)

// Controller manages message data and filtering.
// It contains pure data logic with no Bubble Tea dependencies.
type Controller struct {
	messages   []messaging.Message // accumulated, chronological order
	displayed  []messaging.Message // reversed (newest first)
	cursor     int
	offset     int
	filtering  bool
	filter     string
	filterBuf  strings.Builder
	filteredAt []int // indices into displayed matching filter
}

// NewController creates a new messages controller.
func NewController() *Controller {
	return &Controller{
		filteredAt: make([]int, 0),
	}
}

// Append adds new messages and rebuilds the displayed slice.
func (c *Controller) Append(msgs []messaging.Message) {
	if len(msgs) == 0 {
		return
	}
	c.messages = append(c.messages, msgs...)
	c.displayed = make([]messaging.Message, len(c.messages))
	for i, msg := range c.messages {
		c.displayed[len(c.messages)-1-i] = msg
	}
	c.applyFilter()
}

// StartFilter begins filter input mode.
func (c *Controller) StartFilter() {
	c.filtering = true
	c.filterBuf.Reset()
}

// StopFilter ends filter input mode without clearing the filter text.
func (c *Controller) StopFilter() {
	c.filtering = false
}

// CancelFilter cancels filtering and clears the filter.
func (c *Controller) CancelFilter() {
	c.filtering = false
	c.filter = ""
	c.filterBuf.Reset()
	c.applyFilter()
}

// IsFiltering returns true if filter input is active.
func (c *Controller) IsFiltering() bool {
	return c.filtering
}

// AddFilterRune adds a rune to the filter.
func (c *Controller) AddFilterRune(r rune) {
	c.filterBuf.WriteRune(r)
	c.filter = c.filterBuf.String()
	c.applyFilter()
}

// DeleteFilterRune removes the last rune from the filter.
func (c *Controller) DeleteFilterRune() {
	runes := []rune(c.filterBuf.String())
	if len(runes) > 0 {
		runes = runes[:len(runes)-1]
		c.filterBuf.Reset()
		c.filterBuf.WriteString(string(runes))
		c.filter = string(runes)
		c.applyFilter()
	}
}

// ConfirmFilter confirms the filter and exits filter mode.
func (c *Controller) ConfirmFilter() {
	c.filtering = false
	c.applyFilter()
}

// Filter returns the current filter text.
func (c *Controller) Filter() string {
	return c.filter
}

// MoveUp moves the cursor up one position.
func (c *Controller) MoveUp(visibleLines int) {
	if c.cursor > 0 {
		c.cursor--
		c.clampOffset(visibleLines)
	}
}

// MoveDown moves the cursor down one position.
func (c *Controller) MoveDown(visibleLines int) {
	if c.cursor < len(c.filteredAt)-1 {
		c.cursor++
		c.clampOffset(visibleLines)
	}
}

// Selected returns the currently selected message, or nil if none.
func (c *Controller) Selected() *messaging.Message {
	if len(c.filteredAt) == 0 || c.cursor >= len(c.filteredAt) {
		return nil
	}
	idx := c.filteredAt[c.cursor]
	if idx >= len(c.displayed) {
		return nil
	}
	return &c.displayed[idx]
}

// Displayed returns the displayed (reversed) messages.
func (c *Controller) Displayed() []messaging.Message {
	return c.displayed
}

// FilteredAt returns the indices of displayed messages matching the filter.
func (c *Controller) FilteredAt() []int {
	return c.filteredAt
}

// Cursor returns the current cursor position.
func (c *Controller) Cursor() int {
	return c.cursor
}

// Offset returns the current scroll offset.
func (c *Controller) Offset() int {
	return c.offset
}

// Len returns the total number of accumulated messages.
func (c *Controller) Len() int {
	return len(c.messages)
}

// SetSize clamps the offset after a size change.
func (c *Controller) SetSize(visibleLines int) {
	c.clampOffset(visibleLines)
}

func (c *Controller) applyFilter() {
	c.filteredAt = c.filteredAt[:0]
	filter := strings.ToLower(c.filter)

	for i := range c.displayed {
		if filter == "" || matchesFilter(&c.displayed[i], filter) {
			c.filteredAt = append(c.filteredAt, i)
		}
	}

	if c.cursor >= len(c.filteredAt) {
		c.cursor = 0
	}
}

func matchesFilter(msg *messaging.Message, filter string) bool {
	return strings.Contains(strings.ToLower(msg.Topic), filter) ||
		strings.Contains(strings.ToLower(msg.Sender), filter) ||
		strings.Contains(strings.ToLower(msg.Payload), filter)
}

func (c *Controller) clampOffset(visible int) {
	total := len(c.filteredAt)

	if c.cursor < c.offset {
		c.offset = c.cursor
	} else if c.cursor >= c.offset+visible {
		c.offset = c.cursor - visible + 1
	}

	if c.offset < 0 {
		c.offset = 0
	}
	maxOffset := max(total-visible, 0)
	if c.offset > maxOffset {
		c.offset = maxOffset
	}
}
