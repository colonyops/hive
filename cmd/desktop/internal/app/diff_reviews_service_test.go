package app

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/hive"
)

type fakeServerReviews struct {
	reviews   map[string][]review.DiffReview
	dismissed []string
	err       error
}

func (f *fakeServerReviews) DiffReviews(_ context.Context, id string) ([]review.DiffReview, error) {
	return f.reviews[id], f.err
}

func (f *fakeServerReviews) DismissDiffComment(_ context.Context, commentID string) error {
	f.dismissed = append(f.dismissed, "comment:"+commentID)
	return f.err
}

func (f *fakeServerReviews) DismissDiffReview(_ context.Context, reviewID string) error {
	f.dismissed = append(f.dismissed, "review:"+reviewID)
	return f.err
}

func serverReviewsService(servers map[string]*fakeServerReviews) *DiffReviewsService {
	return newDiffReviewsService(DiffReviewsDeps{
		Engine: func() *hive.Engine { return nil },
		Servers: func(name string) (serverDiffReviews, error) {
			if srv, ok := servers[name]; ok {
				return srv, nil
			}
			return nil, Errorf(KindNotFound, "server %q is not under servers: in config.yaml", name)
		},
		Logger: zerolog.Nop(),
	})
}

func TestDiffReviewsOfAServerSessionComeFromTheServer(t *testing.T) {
	srv := &fakeServerReviews{reviews: map[string][]review.DiffReview{"s1": {{ID: "r1", SessionID: "s1", Author: "claude"}}}}
	svc := serverReviewsService(map[string]*fakeServerReviews{"box": srv})

	got, err := svc.List(t.Context(), "box:s1")
	require.NoError(t, err)
	assert.Equal(t, []review.DiffReview{{ID: "r1", SessionID: "s1", Author: "claude"}}, got)

	require.NoError(t, svc.Dismiss(t.Context(), "box:s1", "c1"))
	require.NoError(t, svc.DismissReview(t.Context(), "box:s1", "r1"))
	assert.Equal(t, []string{"comment:c1", "review:r1"}, srv.dismissed)

	_, err = svc.List(t.Context(), "nowhere:s1")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestDiffReviewsOnAServerClassifyItsFailures(t *testing.T) {
	srv := &fakeServerReviews{err: review.ErrDiffReviewNotFound}
	svc := serverReviewsService(map[string]*fakeServerReviews{"box": srv})
	assert.Equal(t, KindNotFound, KindOf(svc.Dismiss(t.Context(), "box:s1", "c1")), "a comment dismissed elsewhere is gone, not a fault")

	srv.err = errors.New("ssh: connection refused")
	_, err := svc.List(t.Context(), "box:s1")
	assert.Equal(t, KindUnavailable, KindOf(err))
	assert.ErrorContains(t, err, "connection refused", "the server's cause reaches the user")
}

func TestDiffReviewsOfALocalSessionStayOffTheServers(t *testing.T) {
	svc := newDiffReviewsService(DiffReviewsDeps{
		Engine: func() *hive.Engine { return nil },
		Servers: func(string) (serverDiffReviews, error) {
			t.Fatal("asked a server for a local session")
			return nil, nil
		},
		Logger: zerolog.Nop(),
	})
	_, err := svc.List(t.Context(), "s1")
	assert.Equal(t, KindUnavailable, KindOf(err), "the closed engine answers, not a server")
}
