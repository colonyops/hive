package app

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	"github.com/colonyops/hive/pkg/logutils"
)

const ClientLogComponent = "webview"

const (
	clientLogMaxMessage    = 2 << 10
	clientLogMaxStack      = 8 << 10
	clientLogMaxName       = 64
	clientLogMaxFields     = 32
	clientLogMaxFieldKey   = 64
	clientLogMaxFieldValue = 2 << 10

	clientLogRate  = 20
	clientLogBurst = 200
)

// Caller fields may not shadow these.
var clientLogReservedKeys = map[string]bool{
	zerolog.LevelFieldName:     true,
	zerolog.TimestampFieldName: true,
	zerolog.MessageFieldName:   true,
	zerolog.ErrorFieldName:     true,
	logutils.ComponentKey:      true,
	logutils.ServiceNameKey:    true,
	"window":                   true,
	"route":                    true,
	"client_time":              true,
	"stack":                    true,
	"kind":                     true,
	"count":                    true,
	"trace_id":                 true,
	"span_id":                  true,
}

// Input is untrusted in size and rate (ADR
// webview-logs-and-uncaught-errors-are-written-to-hive-log-through-the-root-logger).
type ClientLogService struct {
	logger  zerolog.Logger
	limiter *rate.Limiter

	mu         sync.Mutex
	suppressed int
}

func newClientLogService(logger zerolog.Logger) *ClientLogService {
	return &ClientLogService{
		logger:  logger,
		limiter: rate.NewLimiter(clientLogRate, clientLogBurst),
	}
}

type ClientLogEntry struct {
	At      time.Time
	Level   string
	Name    string
	Message string
	Error   string
	Kind    string
	Stack   string
	Window  string
	Route   string
	Count   int
	Fields  map[string]any
}

func (s *ClientLogService) Level(_ context.Context) string {
	return s.logger.GetLevel().String()
}

func (s *ClientLogService) Write(_ context.Context, entries []ClientLogEntry) {
	for _, entry := range entries {
		if !s.admit() {
			continue
		}
		s.write(entry)
	}
}

func (s *ClientLogService) admit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.limiter.Allow() {
		s.suppressed++
		return false
	}
	if s.suppressed > 0 {
		s.logger.Warn().
			Str(logutils.ComponentKey, ClientLogComponent).
			Int("suppressed", s.suppressed).
			Msg("webview log entries suppressed by the rate limit")
		s.suppressed = 0
	}
	return true
}

func (s *ClientLogService) write(entry ClientLogEntry) {
	event := s.logger.WithLevel(clientLogLevel(entry.Level))
	if event == nil {
		return
	}
	event = event.Str(logutils.ComponentKey, clientLogComponent(entry.Name))
	if !entry.At.IsZero() {
		event = event.Time("client_time", entry.At)
	}
	event = optionalStr(event, "window", truncate(entry.Window, clientLogMaxName))
	event = optionalStr(event, "route", truncate(entry.Route, clientLogMaxFieldValue))
	event = optionalStr(event, zerolog.ErrorFieldName, truncate(entry.Error, clientLogMaxMessage))
	event = optionalStr(event, "kind", truncate(entry.Kind, clientLogMaxName))
	event = optionalStr(event, "stack", truncate(entry.Stack, clientLogMaxStack))
	if entry.Count > 1 {
		event = event.Int("count", entry.Count)
	}
	for _, key := range clientLogFieldKeys(entry.Fields) {
		event = event.Interface(key, clientLogFieldValue(entry.Fields[key]))
	}
	event.Msg(truncate(entry.Message, clientLogMaxMessage))
}

// Fatal and panic mean process death on the Go side, so the webview tops out at error.
func clientLogLevel(level string) zerolog.Level {
	parsed, err := zerolog.ParseLevel(level)
	switch {
	case err != nil, parsed == zerolog.NoLevel, parsed == zerolog.Disabled:
		return zerolog.InfoLevel
	case parsed > zerolog.ErrorLevel:
		return zerolog.ErrorLevel
	case parsed < zerolog.DebugLevel:
		return zerolog.DebugLevel
	default:
		return parsed
	}
}

func clientLogComponent(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return -1
		}
	}, name)
	name = strings.Trim(truncate(name, clientLogMaxName), ".")
	if name == "" {
		return ClientLogComponent
	}
	return ClientLogComponent + "." + name
}

func clientLogFieldKeys(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if key == "" || len(key) > clientLogMaxFieldKey || clientLogReservedKeys[key] {
			continue
		}
		keys = append(keys, key)
		if len(keys) == clientLogMaxFields {
			break
		}
	}
	return keys
}

func clientLogFieldValue(value any) any {
	switch v := value.(type) {
	case string:
		return truncate(v, clientLogMaxFieldValue)
	case nil, bool, float64, int, int64:
		return v
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	if len(encoded) <= clientLogMaxFieldValue {
		return value
	}
	return truncate(string(encoded), clientLogMaxFieldValue)
}

func optionalStr(event *zerolog.Event, key, value string) *zerolog.Event {
	if value == "" {
		return event
	}
	return event.Str(key, value)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit], "") + "…"
}
