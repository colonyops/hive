package stores

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/data/db"
)

// KVStore implements kv.KV using SQLite.
type KVStore struct {
	db *db.DB
}

var _ kv.KV = (*KVStore)(nil)

// NewKVStore creates a new SQLite-backed KV store.
func NewKVStore(db *db.DB) *KVStore {
	return &KVStore{db: db}
}

// Get retrieves and deserializes a value by key.
// Returns an error wrapping sql.ErrNoRows if the key does not exist.
// Expired entries are lazily deleted and treated as missing.
func (s *KVStore) Get(ctx context.Context, key string, dest any) error {
	row, err := s.db.Queries().KVGet(ctx, key)
	if err != nil {
		return fmt.Errorf("kv get %q: %w", key, err)
	}

	if s.isExpired(row) {
		_ = s.db.Queries().KVDelete(ctx, key)
		return fmt.Errorf("kv get %q: %w", key, sql.ErrNoRows)
	}

	if err := json.Unmarshal(row.Value, dest); err != nil {
		return fmt.Errorf("kv get %q unmarshal: %w", key, err)
	}

	return nil
}

// Set stores a value with no expiry.
func (s *KVStore) Set(ctx context.Context, key string, value any) error {
	return s.set(ctx, key, value, sql.NullInt64{})
}

// SetTTL stores a value that expires after the given duration.
func (s *KVStore) SetTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl).UnixNano()
	return s.set(ctx, key, value, sql.NullInt64{Int64: expiresAt, Valid: true})
}

// Delete removes a key.
func (s *KVStore) Delete(ctx context.Context, key string) error {
	if err := s.db.Queries().KVDelete(ctx, key); err != nil {
		return fmt.Errorf("kv delete %q: %w", key, err)
	}
	return nil
}

// Has returns whether a key exists (and is not expired).
func (s *KVStore) Has(ctx context.Context, key string) (bool, error) {
	count, err := s.db.Queries().KVHas(ctx, key)
	if err != nil {
		return false, fmt.Errorf("kv has %q: %w", key, err)
	}
	if count == 0 {
		return false, nil
	}

	// Check expiry via lazy delete
	row, err := s.db.Queries().KVGet(ctx, key)
	if err != nil {
		return false, fmt.Errorf("kv has %q get: %w", key, err)
	}
	if s.isExpired(row) {
		_ = s.db.Queries().KVDelete(ctx, key)
		return false, nil
	}

	return true, nil
}

// ListKeys returns all non-expired keys in sorted order.
func (s *KVStore) ListKeys(ctx context.Context) ([]string, error) {
	now := sql.NullInt64{Int64: time.Now().UnixNano(), Valid: true}
	keys, err := s.db.Queries().KVListKeys(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("kv list keys: %w", err)
	}
	return keys, nil
}

// GetRaw retrieves a raw KV entry with metadata.
// Returns an error wrapping sql.ErrNoRows if the key does not exist.
func (s *KVStore) GetRaw(ctx context.Context, key string) (kv.Entry, error) {
	row, err := s.db.Queries().KVGetRaw(ctx, key)
	if err != nil {
		return kv.Entry{}, fmt.Errorf("kv get raw %q: %w", key, err)
	}

	if s.isExpired(row) {
		_ = s.db.Queries().KVDelete(ctx, key)
		return kv.Entry{}, fmt.Errorf("kv get raw %q: %w", key, sql.ErrNoRows)
	}

	entry := kv.Entry{
		Key:       row.Key,
		Value:     json.RawMessage(row.Value),
		CreatedAt: time.Unix(0, row.CreatedAt),
		UpdatedAt: time.Unix(0, row.UpdatedAt),
	}

	if row.ExpiresAt.Valid {
		t := time.Unix(0, row.ExpiresAt.Int64)
		entry.ExpiresAt = &t
	}

	return entry, nil
}

// SweepExpired deletes all entries whose TTL has passed.
func (s *KVStore) SweepExpired(ctx context.Context) error {
	now := sql.NullInt64{Int64: time.Now().UnixNano(), Valid: true}
	if err := s.db.Queries().KVSweepExpired(ctx, now); err != nil {
		return fmt.Errorf("kv sweep expired: %w", err)
	}
	return nil
}

func (s *KVStore) set(ctx context.Context, key string, value any, expiresAt sql.NullInt64) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("kv set %q marshal: %w", key, err)
	}

	now := time.Now().UnixNano()
	if err := s.db.Queries().KVSet(ctx, db.KVSetParams{
		Key:       key,
		Value:     data,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("kv set %q: %w", key, err)
	}

	return nil
}

func (s *KVStore) isExpired(row db.KvStore) bool {
	return row.ExpiresAt.Valid && row.ExpiresAt.Int64 < time.Now().UnixNano()
}
