package relay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/credentials"
)

const testKey = "rk_test"

// fakeRelay serves the slice of the /v1 API the connector uses, honoring
// after, limit and more the way the relay's feed does.
type fakeRelay struct {
	*httptest.Server

	mu         sync.Mutex
	deliveries []DeliveryView
	limits     Limits
	email      string
	status     int
	afters     []string
	userAgents []string
}

func newFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	f := &fakeRelay{email: "me@example.com"}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeRelay) add(views ...DeliveryView) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deliveries = append(f.deliveries, views...)
}

func (f *fakeRelay) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userAgents = append(f.userAgents, r.Header.Get("User-Agent"))

	if r.URL.Path == "/v1/auth/methods" {
		_, _ = w.Write([]byte(`{"email_code":false,"password":false,"api_key":true}`))
		return
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"kind":"unauthenticated","message":"bad key"}`))
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+testKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/v1/me":
		_ = json.NewEncoder(w).Encode(Me{Email: f.email, Limits: f.limits, Version: "test"})
	case "/v1/sources/src/deliveries":
		after := r.URL.Query().Get("after")
		f.afters = append(f.afters, after)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 200
		}
		start := 0
		for start < len(f.deliveries) && f.deliveries[start].ID <= after {
			start++
		}
		end := min(start+limit, len(f.deliveries))
		page := Page{Deliveries: f.deliveries[start:end], More: end < len(f.deliveries)}
		if end > start {
			page.Next = f.deliveries[end-1].ID
		}
		_ = json.NewEncoder(w).Encode(page)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func deliveryID(n int) string { return fmt.Sprintf("d%05d", n) }

func jsonDelivery(n int, body string, at time.Time) DeliveryView {
	return DeliveryView{ID: deliveryID(n), ReceivedAt: at, Method: "POST", Path: "/hook", Body: body}
}

// connected returns fetchers over a store that holds the fake relay's binding
// and key.
func connected(t *testing.T, relay *fakeRelay) (*Fetchers, credentials.Ref) {
	t.Helper()
	ref := credentials.Ref{Provider: Provider, Account: "relay.test-me@example.com"}
	creds := credentials.NewMemoryStore()
	require.NoError(t, creds.Set(ref, testKey))
	instances := NewInstanceStore(filepath.Join(t.TempDir(), "relay-instances.json"))
	require.NoError(t, instances.Set(ref, Binding{URL: relay.URL, Email: "me@example.com"}))
	return NewFetchers(instances, creds, "1.2.3", zerolog.Nop()), ref
}
