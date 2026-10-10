package wailsui

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

type ClientLogService struct {
	clientLog *app.ClientLogService
}

func NewClientLogService(clientLog *app.ClientLogService) *ClientLogService {
	return &ClientLogService{clientLog: clientLog}
}

// ClientLogEntry is one webview log line. AtUnixMs is Date.now() at the call;
// zero leaves only the time the backend wrote it.
type ClientLogEntry struct {
	AtUnixMs int64          `json:"atUnixMs"`
	Level    string         `json:"level"`
	Name     string         `json:"name,omitempty"`
	Message  string         `json:"message"`
	Error    string         `json:"error,omitempty"`
	Kind     string         `json:"kind,omitempty"`
	Stack    string         `json:"stack,omitempty"`
	Window   string         `json:"window,omitempty"`
	Route    string         `json:"route,omitempty"`
	Count    int            `json:"count,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

func (s *ClientLogService) Level(ctx context.Context) string {
	return s.clientLog.Level(ctx)
}

func (s *ClientLogService) Write(ctx context.Context, entries []ClientLogEntry) {
	converted := make([]app.ClientLogEntry, 0, len(entries))
	for _, in := range entries {
		var at time.Time
		if in.AtUnixMs > 0 {
			at = time.UnixMilli(in.AtUnixMs).UTC()
		}
		converted = append(converted, app.ClientLogEntry{
			At:      at,
			Level:   in.Level,
			Name:    in.Name,
			Message: in.Message,
			Error:   in.Error,
			Kind:    in.Kind,
			Stack:   in.Stack,
			Window:  in.Window,
			Route:   in.Route,
			Count:   in.Count,
			Fields:  in.Fields,
		})
	}
	s.clientLog.Write(ctx, converted)
}
