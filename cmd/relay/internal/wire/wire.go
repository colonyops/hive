// Package wire holds the /v1 JSON shapes and input rules. The HTTP API and the
// MCP server both answer with these, so a tool returns the same document the
// matching endpoint does.
package wire

import (
	"time"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

type Source struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	HasSecret  bool      `json:"has_secret"`
	CreatedAt  time.Time `json:"created_at"`
	Deliveries int       `json:"deliveries"`
	Bytes      int64     `json:"bytes"`
}

func SourceFrom(v relay.SourceView) Source {
	return Source{
		ID:         v.ID,
		Name:       v.Name,
		HasSecret:  v.SecretHash != "",
		CreatedAt:  v.CreatedAt,
		Deliveries: v.Deliveries,
		Bytes:      v.Bytes,
	}
}

type Sources struct {
	Sources []Source `json:"sources"`
}

func SourcesFrom(views []relay.SourceView) Sources {
	out := make([]Source, 0, len(views))
	for _, v := range views {
		out = append(out, SourceFrom(v))
	}
	return Sources{Sources: out}
}

// CreatedSource is the only answer that carries the ingress URL besides a
// rotation: the relay stores its hash, not the token.
type CreatedSource struct {
	Source
	IngressURL string `json:"ingress_url"`
}

type IngressURL struct {
	IngressURL string `json:"ingress_url"`
}

type Token struct {
	ID         string          `json:"id"`
	Kind       relay.TokenKind `json:"kind"`
	Name       string          `json:"name"`
	CreatedAt  time.Time       `json:"created_at"`
	LastUsedAt *time.Time      `json:"last_used_at"`
	// Current marks the token this request authenticated with, so a client
	// can tell before revoking itself.
	Current bool `json:"current"`
}

type Tokens struct {
	Tokens []Token `json:"tokens"`
}

func TokensFrom(toks []relay.Token, currentID string) Tokens {
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		v := Token{ID: t.ID, Kind: t.Kind, Name: t.Device, CreatedAt: t.CreatedAt, Current: t.ID == currentID}
		if !t.LastUsedAt.IsZero() {
			v.LastUsedAt = &t.LastUsedAt
		}
		out = append(out, v)
	}
	return Tokens{Tokens: out}
}

type CreatedToken struct {
	Token     string          `json:"token"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Kind      relay.TokenKind `json:"kind"`
	CreatedAt time.Time       `json:"created_at"`
}

func CreatedTokenFrom(plaintext string, tok relay.Token) CreatedToken {
	return CreatedToken{Token: plaintext, ID: tok.ID, Name: tok.Device, Kind: tok.Kind, CreatedAt: tok.CreatedAt}
}

type Me struct {
	Email      string        `json:"email"`
	Encryption bool          `json:"encryption"`
	Limits     Limits        `json:"limits"`
	Usage      Usage         `json:"usage"`
	Methods    relay.Methods `json:"methods"`
	Version    string        `json:"version"`
}

type Limits struct {
	BodyBytes           int64 `json:"body_bytes"`
	HeaderBytes         int   `json:"header_bytes"`
	DeliveriesPerSource int   `json:"deliveries_per_source"`
	BytesPerAccount     int64 `json:"bytes_per_account"`
	SourcesPerAccount   int   `json:"sources_per_account"`
	RetentionSeconds    int64 `json:"retention_seconds"`
}

type Usage struct {
	Bytes   int64 `json:"bytes"`
	Sources int   `json:"sources"`
}

func MeFrom(acct relay.Account, sources int, l relay.Limits, methods relay.Methods, version string) Me {
	return Me{
		Email:      acct.Email,
		Encryption: acct.Encryption,
		Limits: Limits{
			BodyBytes:           l.BodyBytes,
			HeaderBytes:         l.HeaderBytes,
			DeliveriesPerSource: l.DeliveriesPerSource,
			BytesPerAccount:     l.BytesPerAccount,
			SourcesPerAccount:   l.SourcesPerAccount,
			RetentionSeconds:    int64(l.Retention.Seconds()),
		},
		Usage:   Usage{Bytes: acct.BytesUsed, Sources: sources},
		Methods: methods,
		Version: version,
	}
}
