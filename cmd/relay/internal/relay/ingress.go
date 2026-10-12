package relay

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/randid"
)

type Ingress struct {
	store  Store
	limits Limits
	now    func() time.Time
	newID  func() string
}

func NewIngress(store Store, limits Limits) *Ingress {
	return &Ingress{store: store, limits: limits, now: time.Now, newID: randid.UUIDv7}
}

// Accept stores rec under the source the token names. It reads nothing from
// rec.Body and makes no cap decision of its own: DeliveryStore.Append decides
// and its KindOverCap passes through. Errors: KindNotFound (token),
// KindUnauthorized (secret), KindOverCap.
func (i *Ingress) Accept(ctx context.Context, token, presentedSecret string, rec Record) (string, error) {
	src, err := i.ResolveSource(ctx, token, presentedSecret)
	if err != nil {
		return "", err
	}
	return i.AcceptSource(ctx, src, rec)
}

// AcceptSource is the half the HTTP handler calls after its own
// ResolveSource, so the per-source limiter sits between the two.
func (i *Ingress) AcceptSource(ctx context.Context, src Source, rec Record) (string, error) {
	raw, err := json.Marshal(rec)
	if err != nil {
		return "", fmt.Errorf("encode record: %w", err)
	}
	acct, err := i.store.Accounts().Get(ctx, src.Email)
	if errors.Is(err, ErrNotFound) {
		return "", NewError(KindNotFound, "unknown ingress token")
	}
	if err != nil {
		return "", fmt.Errorf("look up account: %w", err)
	}
	d := Delivery{
		ID:         i.newID(),
		SourceID:   src.ID,
		ReceivedAt: i.now(),
		Record:     raw,
	}
	if acct.Encryption {
		if d.Record, err = i.sealFor(ctx, src.Email, raw); err != nil {
			return "", err
		}
		d.Enc = true
	}
	d.Size = int64(len(d.Record))
	err = i.store.Deliveries().Append(ctx, src.Email, d, i.limits.BytesPerAccount, i.limits.DeliveriesPerSource)
	switch {
	case err == nil:
		return d.ID, nil
	case KindOf(err) != "":
		return "", err
	case errors.Is(err, ErrNotFound):
		// The source or its account went away after the lookup.
		return "", NewError(KindNotFound, "unknown ingress token")
	default:
		return "", fmt.Errorf("append delivery: %w", err)
	}
}

// sealFor encrypts raw to the account's recipients. SetEncryption and
// RemoveRecipient never leave encryption on with none, so an empty list is a
// broken invariant, not a caller error.
func (i *Ingress) sealFor(ctx context.Context, email string, raw []byte) ([]byte, error) {
	rs, err := i.store.Recipients().ListByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	if len(rs) == 0 {
		return nil, fmt.Errorf("account %s has encryption on and no recipients", email)
	}
	keys := make([]string, len(rs))
	for j, r := range rs {
		keys[j] = r.Recipient
	}
	return seal(raw, keys)
}

// ResolveSource looks the source up by ingress token and checks the secret.
// The HTTP layer calls it before the per-source rate limiter so an unknown
// token never consumes a bucket.
func (i *Ingress) ResolveSource(ctx context.Context, token, presentedSecret string) (Source, error) {
	if token == "" {
		return Source{}, NewError(KindNotFound, "unknown ingress token")
	}
	src, err := i.store.Sources().GetByIngressHash(ctx, HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return Source{}, NewError(KindNotFound, "unknown ingress token")
	}
	if err != nil {
		return Source{}, fmt.Errorf("look up source: %w", err)
	}
	if src.SecretHash != "" &&
		subtle.ConstantTimeCompare([]byte(HashToken(presentedSecret)), []byte(src.SecretHash)) != 1 {
		return Source{}, NewError(KindUnauthorized, "missing or wrong source secret")
	}
	return src, nil
}

var redactedHeaders = []string{"Authorization", "Cookie", "X-Hive-Secret", "X-Gotify-Key"}

// RedactHeaders drops credentials and keeps headers, in key order, until the
// next value would push the sum of key and value lengths past limit.
func RedactHeaders(h http.Header, limit int) map[string][]string {
	out := map[string][]string{}
	total := 0
	for _, key := range slices.Sorted(maps.Keys(h)) {
		if slices.ContainsFunc(redactedHeaders, func(r string) bool { return strings.EqualFold(key, r) }) {
			continue
		}
		for _, v := range h[key] {
			cost := len(v)
			if _, seen := out[key]; !seen {
				cost += len(key)
			}
			if total+cost > limit {
				return out
			}
			total += cost
			out[key] = append(out[key], v)
		}
	}
	return out
}
