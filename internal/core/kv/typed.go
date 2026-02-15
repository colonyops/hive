package kv

import (
	"context"
	"time"
)

// TypedKV provides type-safe access to a KV store for a specific type T.
type TypedKV[T any] struct {
	store  KV
	prefix string
}

// Scoped returns a TypedKV[T] that prefixes all keys with "namespace:".
func Scoped[T any](store KV, namespace string) *TypedKV[T] {
	return &TypedKV[T]{
		store:  store,
		prefix: namespace + ":",
	}
}

// Get retrieves and deserializes a value by key.
func (t *TypedKV[T]) Get(ctx context.Context, key string) (T, error) {
	var v T
	if err := t.store.Get(ctx, t.prefix+key, &v); err != nil {
		return v, err
	}
	return v, nil
}

// Set stores a value with no expiry.
func (t *TypedKV[T]) Set(ctx context.Context, key string, value T) error {
	return t.store.Set(ctx, t.prefix+key, value)
}

// SetTTL stores a value that expires after the given duration.
func (t *TypedKV[T]) SetTTL(ctx context.Context, key string, value T, ttl time.Duration) error {
	return t.store.SetTTL(ctx, t.prefix+key, value, ttl)
}

// Delete removes a key.
func (t *TypedKV[T]) Delete(ctx context.Context, key string) error {
	return t.store.Delete(ctx, t.prefix+key)
}

// Has returns whether a key exists.
func (t *TypedKV[T]) Has(ctx context.Context, key string) (bool, error) {
	return t.store.Has(ctx, t.prefix+key)
}
