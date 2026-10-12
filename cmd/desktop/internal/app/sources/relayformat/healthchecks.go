package relayformat

import (
	"strconv"
	"strings"
	"time"
)

// Healthchecks pings are one item per check, keyed by slug, so the item's
// state is the check's current state. A plain ping or exit status 0 resolves
// it; /fail or a non-zero exit status marks it down. /start and /log say
// nothing about the outcome: their item has no state and carries "ping"
// ("start" or "log") instead, and the relay connector keeps the stored state
// when it merges one. "pinged_at" is the delivery's receive time, which the
// connector's missed-ping clock reads.
const (
	pingStart = "start"
	pingLog   = "log"
	pingFail  = "fail"
)

type hcPing struct {
	slug   string
	suffix string
}

// healthchecksPath accepts /<slug> with an optional /start, /fail, /log or
// /<exit-status>. Longer paths such as /ping/<key>/<slug> take the slug from
// the segment before the suffix.
func healthchecksPath(path string) (hcPing, bool) {
	segs := pathSegments(path)
	if len(segs) == 0 {
		return hcPing{}, false
	}
	last := segs[len(segs)-1]
	if len(segs) >= 2 && isPingSuffix(last) {
		return hcPing{slug: segs[len(segs)-2], suffix: last}, true
	}
	return hcPing{slug: last}, true
}

func isPingSuffix(s string) bool {
	switch s {
	case pingStart, pingFail, pingLog:
		return true
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

func parseHealthchecks(d Delivery) (Item, bool) {
	ping, ok := healthchecksPath(d.Path)
	if !ok {
		return Item{}, false
	}
	payload := map[string]any{
		"id":    ping.slug,
		"kind":  "Check",
		"title": ping.slug,
		"repo":  "healthchecks",
	}
	switch ping.suffix {
	case "":
		payload["state"] = "resolved"
	case pingFail:
		payload["state"] = "down"
	case pingStart, pingLog:
		payload["ping"] = ping.suffix
	default:
		if code, _ := strconv.Atoi(ping.suffix); code == 0 {
			payload["state"] = "resolved"
		} else {
			payload["state"] = "down"
		}
	}
	setIfPresent(payload, "body", strings.TrimSpace(string(d.Body)))
	if !d.ReceivedAt.IsZero() {
		payload["pinged_at"] = d.ReceivedAt.UTC().Format(time.RFC3339Nano)
	}
	payload["raw"] = raw(d)
	return Item{Key: ping.slug, Payload: payload}, true
}
