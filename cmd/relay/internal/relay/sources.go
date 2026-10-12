package relay

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Sources struct {
	store     Store
	limits    Limits
	publicURL string
	now       func() time.Time
}

func NewSources(store Store, limits Limits, publicURL string) *Sources {
	return &Sources{
		store:     store,
		limits:    limits,
		publicURL: strings.TrimRight(publicURL, "/"),
		now:       time.Now,
	}
}

type SourceView struct {
	Source
	Deliveries int
	Bytes      int64
}

func (s *Sources) Create(ctx context.Context, email, name, secret string) (Source, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Source{}, "", NewError(KindInvalid, "name is required")
	}
	n, err := s.store.Sources().CountByEmail(ctx, email)
	if err != nil {
		return Source{}, "", fmt.Errorf("count sources: %w", err)
	}
	if n >= s.limits.SourcesPerAccount {
		return Source{}, "", NewError(KindInvalid, fmt.Sprintf("an account holds at most %d sources", s.limits.SourcesPerAccount))
	}

	now := s.now()
	if err := ensureAccount(ctx, s.store, email, now); err != nil {
		return Source{}, "", err
	}
	token, err := NewToken()
	if err != nil {
		return Source{}, "", err
	}
	src := Source{
		ID:          uuid.NewString(),
		Email:       email,
		Name:        name,
		IngressHash: HashToken(token),
		SecretHash:  secretHash(secret),
		CreatedAt:   now,
	}
	if err := s.store.Sources().Put(ctx, src); err != nil {
		return Source{}, "", fmt.Errorf("store source: %w", err)
	}
	return src, token, nil
}

func (s *Sources) Get(ctx context.Context, email, id string) (SourceView, error) {
	src, err := s.get(ctx, email, id)
	if err != nil {
		return SourceView{}, err
	}
	return s.view(ctx, src)
}

// Update changes the fields that are non-nil. An empty secret clears it.
func (s *Sources) Update(ctx context.Context, email, id string, name, secret *string) (Source, error) {
	src, err := s.get(ctx, email, id)
	if err != nil {
		return Source{}, err
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return Source{}, NewError(KindInvalid, "name is required")
		}
		src.Name = trimmed
	}
	if secret != nil {
		src.SecretHash = secretHash(*secret)
	}
	if err := s.store.Sources().Put(ctx, src); err != nil {
		return Source{}, fmt.Errorf("store source: %w", err)
	}
	return src, nil
}

// Rotate replaces the ingress token; the old one stops resolving at once.
func (s *Sources) Rotate(ctx context.Context, email, id string) (string, error) {
	src, err := s.get(ctx, email, id)
	if err != nil {
		return "", err
	}
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	src.IngressHash = HashToken(token)
	if err := s.store.Sources().Put(ctx, src); err != nil {
		return "", fmt.Errorf("store source: %w", err)
	}
	return token, nil
}

func (s *Sources) Delete(ctx context.Context, email, id string) error {
	err := s.store.Sources().Delete(ctx, email, id)
	if errors.Is(err, ErrNotFound) {
		return NewError(KindNotFound, "source not found")
	}
	if err != nil {
		return fmt.Errorf("delete source: %w", err)
	}
	return nil
}

func (s *Sources) List(ctx context.Context, email string) ([]SourceView, error) {
	srcs, err := s.store.Sources().ListByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	slices.SortFunc(srcs, func(x, y Source) int {
		return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), cmp.Compare(x.ID, y.ID))
	})
	out := make([]SourceView, 0, len(srcs))
	for _, src := range srcs {
		v, err := s.view(ctx, src)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Sources) IngressURL(token string) string {
	return s.publicURL + "/in/" + token
}

func (s *Sources) get(ctx context.Context, email, id string) (Source, error) {
	src, err := s.store.Sources().Get(ctx, email, id)
	if errors.Is(err, ErrNotFound) {
		return Source{}, NewError(KindNotFound, "source not found")
	}
	if err != nil {
		return Source{}, fmt.Errorf("look up source: %w", err)
	}
	return src, nil
}

func (s *Sources) view(ctx context.Context, src Source) (SourceView, error) {
	n, b, err := s.store.Deliveries().Stats(ctx, src.ID)
	if err != nil {
		return SourceView{}, fmt.Errorf("source stats: %w", err)
	}
	return SourceView{Source: src, Deliveries: n, Bytes: b}, nil
}

func secretHash(secret string) string {
	if secret == "" {
		return ""
	}
	return HashToken(secret)
}
