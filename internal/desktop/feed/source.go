package feed

// SourceDef is one GitHub fetch request. It is no longer a config entry — the
// github-source flow node embeds these fields — but it survives as the value
// type LiveProvider.SourceItems takes and caches on (keyed by kind+query+limit
// via sourceKey), so any number of source nodes requesting the same data
// share one API request.
type SourceDef struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // "search" | "notifications"
	Query string `json:"query,omitempty"`
	// Limit bounds items per fetch. 0 uses defaultSourceLimit.
	Limit int `json:"limit,omitempty"`
}

const defaultSourceLimit = 50

// effectiveLimit resolves the configured limit against the default.
func (s SourceDef) effectiveLimit() int {
	if s.Limit > 0 {
		return s.Limit
	}
	return defaultSourceLimit
}
