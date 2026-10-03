package kv

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is wrapped by Get and GetRaw when the key is missing or expired.
var ErrNotFound = errors.New("kv: key not found")

// Entry represents a raw KV entry with metadata.
type Entry struct {
	Key       string
	Value     json.RawMessage
	ExpiresAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// KV is the interface for a persistent key-value store.
// Keys are strings, values are JSON-serializable.
// Get on a missing key returns an error wrapping ErrNotFound.
type KV interface {
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value any) error
	SetTTL(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Has(ctx context.Context, key string) (bool, error)
	ListKeys(ctx context.Context) ([]string, error)
	GetRaw(ctx context.Context, key string) (Entry, error)
}
