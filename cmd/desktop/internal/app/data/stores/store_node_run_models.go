package stores

// NodeRunRecord adds persisted EndedAt to the Wails-facing node-run shape.
type NodeRunRecord struct {
	FlowID    string `json:"flowId"`
	NodeID    string `json:"nodeId"`
	OK        bool   `json:"ok"`
	InCount   int    `json:"inCount"`
	OutCount  int    `json:"outCount"`
	DropCount int    `json:"dropCount"`
	Err       string `json:"err"`
	DurMs     int64  `json:"durMs"`
	EndedAt   int64  `json:"endedAt"`
}
