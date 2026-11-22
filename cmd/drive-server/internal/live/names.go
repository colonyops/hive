package live

import (
	"hash/fnv"

	"github.com/colonyops/hive/internal/platform/driveapi"
)

var (
	adjectives = []string{
		"Amber", "Brave", "Calm", "Clever", "Curious", "Daring", "Eager", "Gentle",
		"Happy", "Jolly", "Keen", "Lively", "Lucky", "Merry", "Nimble", "Quiet",
		"Quick", "Sunny", "Swift", "Witty",
	}
	animals = []string{
		"Badger", "Beaver", "Crane", "Falcon", "Ferret", "Fox", "Gecko", "Heron",
		"Koala", "Lemur", "Lynx", "Marten", "Otter", "Owl", "Panda", "Puffin",
		"Raven", "Seal", "Stoat", "Wren",
	}
)

// generatedName is a readable name for an anonymous viewer, stable for one
// seed.
func generatedName(seed string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	n := h.Sum32()
	return adjectives[n%uint32(len(adjectives))] + " " + animals[(n/uint32(len(adjectives)))%uint32(len(animals))]
}

// ColorFor is a presence color index that stays the same for one seed, such
// as an account id.
func ColorFor(seed string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return int(h.Sum32() % driveapi.PresenceColors)
}
