package relayformat

import "strings"

var (
	ntfyDetectHeaders = []string{"X-Title", "Title", "X-Priority", "Priority", "X-Tags", "Tags", "X-Message", "Message"}
	ntfyDetectQuery   = []string{"title", "message", "priority", "tags", "click"}
)

// detect picks a dialect for a delivery, most specific shape first. A plain
// text POST to /<topic> with no ntfy headers is indistinguishable from a
// healthchecks ping and reads as one; a node that receives those sets dialect
// ntfy.
func detect(d Delivery) string {
	obj, isObject := decodeObject(d.Body)
	ntfyHeaders := hasHeader(d.Headers, ntfyDetectHeaders...) || query(d.Query, ntfyDetectQuery...) != ""
	gotify := lastSegment(d.Path) == "message" &&
		(hasHeader(d.Headers, "X-Gotify-Key") || d.Query.Has("token") || gotifyBodyHasMessage(d))

	switch {
	case !isObject && !ntfyHeaders && !gotify && healthchecksShape(d.Path):
		return DialectHealthchecks
	case gotify:
		return DialectGotify
	case ntfyHeaders || (field(obj, "topic") != "" && field(obj, "message") != ""):
		return DialectNtfy
	default:
		return DialectGeneric
	}
}

func healthchecksShape(path string) bool {
	segs := pathSegments(path)
	switch len(segs) {
	case 1:
		return true
	case 2:
		return isPingSuffix(segs[1])
	default:
		return false
	}
}

func lastSegment(path string) string {
	segs := pathSegments(path)
	if len(segs) == 0 {
		return ""
	}
	return strings.ToLower(segs[len(segs)-1])
}
