package relay

import (
	"fmt"
	"maps"
	"time"
)

const (
	stateResolved = "resolved"
	stateDown     = "down"
)

// recordSuccess notes the newest successful ping per key. It is tracked apart
// from the item because a later /start or /log replaces the stored payload.
func (n *nodeState) recordSuccess(key string, it item) {
	state, _ := it.payload["state"].(string)
	ping, _ := it.payload["ping"].(string)
	if state != stateResolved || ping != "" {
		return
	}
	at := pingedAt(it)
	if n.lastSuccess == nil {
		n.lastSuccess = map[string]time.Time{}
	}
	if at.After(n.lastSuccess[key]) {
		n.lastSuccess[key] = at
	}
}

// applyClock marks a check down once period plus grace has passed since its
// last successful ping. A check that never succeeded counts from its newest
// ping instead, so a lone /start is not down on arrival. The down item is
// left alone on later ticks so it emits once; only a new success ping, merged
// through the parser, clears it.
func (n *nodeState) applyClock(checks map[string]Check, now time.Time) {
	for slug, check := range checks {
		it, ok := n.items[slug]
		if !ok {
			continue
		}
		if state, _ := it.payload["state"].(string); state == stateDown {
			continue
		}
		since, ok := n.lastSuccess[slug]
		if !ok {
			since = pingedAt(it)
		}
		late := now.Sub(since)
		if late <= check.Period.Duration()+check.Grace.Duration() {
			continue
		}
		payload := maps.Clone(it.payload)
		payload["state"] = stateDown
		payload["body"] = fmt.Sprintf("no ping for %s", late.Round(time.Second))
		it.payload = payload
		n.items[slug] = it
	}
}

func pingedAt(it item) time.Time {
	if s, ok := it.payload["pinged_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
	}
	return it.receivedAt
}
