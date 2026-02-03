package components

import (
	"strings"
	"sync"
)

// Cache for padding strings to avoid strings.Repeat allocations.
// Supports padding widths from 0 to 200 characters.
var paddingCache = [201]string{
	0:   "",
	1:   " ",
	2:   "  ",
	3:   "   ",
	4:   "    ",
	5:   "     ",
	6:   "      ",
	7:   "       ",
	8:   "        ",
	9:   "         ",
	10:  "          ",
	20:  "                    ",
	40:  "                                        ",
	80:  "                                                                                ",
	100: "                                                                                                    ",
	200: "                                                                                                                                                                                        ",
}

var paddingOnce sync.Once

// initPaddingCache initializes all padding entries from 0-200.
func initPaddingCache() {
	for i := 0; i <= 200; i++ {
		if paddingCache[i] == "" && i > 0 {
			paddingCache[i] = strings.Repeat(" ", i)
		}
	}
}

// Pad returns a string of n spaces, using a cache for efficiency.
func Pad(n int) string {
	if n <= 0 {
		return ""
	}
	if n <= 200 {
		paddingOnce.Do(initPaddingCache)
		return paddingCache[n]
	}
	return strings.Repeat(" ", n)
}
