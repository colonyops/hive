package commands

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func collectShortFlags(cmd *cli.Command, path string, out map[string]map[string][]string) {
	for _, f := range cmd.Flags {
		names := f.Names()
		if len(names) == 0 {
			continue
		}
		long := names[0]
		for _, n := range names {
			if len(n) > len(long) {
				long = n
			}
		}
		for _, n := range names {
			if len(n) != 1 {
				continue
			}
			if out[n] == nil {
				out[n] = map[string][]string{}
			}
			out[n][long] = append(out[n][long], path)
		}
	}
	for _, sub := range cmd.Commands {
		collectShortFlags(sub, path+" "+sub.Name, out)
	}
}

// A script that works in one command must not do something else in another
// because the same letter names a different flag.
func TestShortFlagsHaveOneMeaning(t *testing.T) {
	shorts := map[string]map[string][]string{}
	collectShortFlags(commandTree(t), "hive", shorts)

	letters := make([]string, 0, len(shorts))
	for l := range shorts {
		letters = append(letters, l)
	}
	sort.Strings(letters)

	for _, l := range letters {
		if len(shorts[l]) < 2 {
			continue
		}
		var parts []string
		for long, cmds := range shorts[l] {
			parts = append(parts, "--"+long+" ("+strings.Join(cmds, ", ")+")")
		}
		sort.Strings(parts)
		assert.Failf(t, "short flag with several meanings", "-%s: %s", l, strings.Join(parts, " | "))
	}
}
