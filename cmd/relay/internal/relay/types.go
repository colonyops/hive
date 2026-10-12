// Package relay is the relay's core: the models, the store port, and the
// services that accept, cap, and serve deliveries. It knows nothing about HTTP.
package relay

import (
	"net/url"
	"time"
)

type Account struct {
	Email      string
	Encryption bool
	BytesUsed  int64
	CreatedAt  time.Time
}

type TokenKind string

const (
	TokenDevice TokenKind = "device"
	TokenAPI    TokenKind = "api"
)

type Token struct {
	ID         string
	Email      string
	Hash       string // hex sha256 of the bearer value
	Kind       TokenKind
	Device     string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// Recipient is an age X25519 public key a desktop registered, in its
// "age1..." form.
type Recipient struct {
	ID        string
	Email     string
	Recipient string
	Device    string
	CreatedAt time.Time
}

type Source struct {
	ID          string // uuid v4
	Email       string
	Name        string
	IngressHash string // hex sha256 of the ingress token
	SecretHash  string // "" when no secret
	CreatedAt   time.Time
}

// Record is a delivery as the sender made it. The relay stores it without
// reading Body.
type Record struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"` // suffix after the token, "" or "/..."
	Query       url.Values          `json:"query"`
	Headers     map[string][]string `json:"headers"` // redacted
	Body        []byte              `json:"body"`
	ContentType string              `json:"content_type"`
	Remote      string              `json:"remote"`
}

type Delivery struct {
	ID         string // uuid v7; the cursor
	SourceID   string
	ReceivedAt time.Time
	Enc        bool
	Size       int64  // bytes of Record as stored, counted against the account
	Record     []byte // JSON of Record, or age ciphertext when Enc
}

type Limits struct {
	BodyBytes           int64         // 256 KiB
	HeaderBytes         int           // 64 KiB
	DeliveriesPerSource int           // 1000
	BytesPerAccount     int64         // 64 MiB
	SourcesPerAccount   int           // 50
	Retention           time.Duration // 14d
	IngressPerSource    Rate          // 60/min burst 30
	IngressPerIP        Rate          // 600/min
	AuthPerEmail        Rate          // 3/hour
	AuthPerIP           Rate          // 10/hour
	Concurrency         int           // 256
}

type Rate struct {
	Per   time.Duration
	Count int
	Burst int
}

// DefaultLimits are the caps the design doc fixes for v1.
func DefaultLimits() Limits {
	return Limits{
		BodyBytes:           256 << 10,
		HeaderBytes:         64 << 10,
		DeliveriesPerSource: 1000,
		BytesPerAccount:     64 << 20,
		SourcesPerAccount:   50,
		Retention:           14 * 24 * time.Hour,
		IngressPerSource:    Rate{Per: time.Minute, Count: 60, Burst: 30},
		IngressPerIP:        Rate{Per: time.Minute, Count: 600, Burst: 600},
		AuthPerEmail:        Rate{Per: time.Hour, Count: 3, Burst: 3},
		AuthPerIP:           Rate{Per: time.Hour, Count: 10, Burst: 10},
		Concurrency:         256,
	}
}
