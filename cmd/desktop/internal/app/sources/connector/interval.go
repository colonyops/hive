package connector

import "fmt"

// IntervalDoc is the prose every pull connector's `interval` field documents
// itself with. Struct tags take literals only, so the tag text is still
// repeated per config; this exists so the docs and any prose about the field
// quote one sentence rather than seven paraphrases.
const IntervalDoc = "Shortest time between fetches. The source still only runs on a poll tick, so the real cadence rounds up to the next one; empty fetches on every tick."

// ValidateInterval rejects a negative cadence floor. Zero is not an error: it
// is what a config that omits the field carries, and it means every tick.
//
// Nothing here enforces the floor. A connector only declares it, as its
// Instance's MinInterval; the poll producer is what skips a source that is not
// due, and a manual refresh overrides it.
func ValidateInterval(connectorName string, interval Duration) error {
	if interval < 0 {
		return fmt.Errorf("%s: interval must not be negative", connectorName)
	}
	return nil
}
