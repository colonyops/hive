package app

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hay-kot/appkit/plugs"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func TestRunServesTheFlow(t *testing.T) {
	cfg := config.Config{
		Listen:   "127.0.0.1:0",
		DataDir:  t.TempDir(),
		MintFor:  "dev@example.com",
		Limits:   relay.DefaultLimits(),
		LogLevel: "info",
	}

	stdoutR, stdoutW := io.Pipe()
	addrs := make(chan net.Addr, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cfg, zerolog.Nop(), Options{
			// WithSignals() with no signals does not disable handling:
			// signal.NotifyContext then relays every signal, including the
			// runtime's SIGURG preemption, which stops the server mid-test.
			Plugs:    []plugs.Option{plugs.WithSignals(os.Interrupt)},
			Stdout:   stdoutW,
			OnListen: func(a net.Addr) { addrs <- a },
			NoReload: true,
		})
	}()

	key, err := bufio.NewReader(stdoutR).ReadString('\n')
	require.NoError(t, err)
	key = strings.TrimSpace(key)
	require.NotEmpty(t, key)
	base := "http://" + (<-addrs).String()

	call := func(method, url, body string, auth bool) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
		require.NoError(t, err)
		if auth {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		out, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, out
	}

	status, body := call(http.MethodPost, base+"/v1/sources", `{"name":"ci"}`, true)
	require.Equal(t, http.StatusCreated, status, string(body))
	var src struct {
		ID         string `json:"id"`
		IngressURL string `json:"ingress_url"`
	}
	require.NoError(t, json.Unmarshal(body, &src))
	require.True(t, strings.HasPrefix(src.IngressURL, base+"/in/"), src.IngressURL)

	payload := `{"id":"build-1","title":"tests failed"}`
	status, body = call(http.MethodPost, src.IngressURL, payload, false)
	require.Equal(t, http.StatusOK, status, string(body))

	status, body = call(http.MethodGet, base+"/v1/sources/"+src.ID+"/deliveries", "", true)
	require.Equal(t, http.StatusOK, status, string(body))
	var page relay.Page
	require.NoError(t, json.Unmarshal(body, &page))
	require.Len(t, page.Deliveries, 1)
	assert.Equal(t, payload, page.Deliveries[0].Body)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestOpenStoreCreatesDataDir(t *testing.T) {
	cfg := config.Config{DataDir: filepath.Join(t.TempDir(), "nested", "relay")}
	store, err := OpenStore(t.Context(), cfg)
	require.NoError(t, err)
	require.NoError(t, store.Close())
	assert.FileExists(t, cfg.SQLitePath())
}

func TestRunSignsInWithTheLogSender(t *testing.T) {
	cfg := config.Config{
		Listen:        "127.0.0.1:0",
		DataDir:       t.TempDir(),
		Limits:        relay.DefaultLimits(),
		LogLevel:      "info",
		AllowedEmails: []string{"*@example.com"},
		SMTP:          config.SMTP{URL: config.SMTPLog},
	}

	stdoutR, stdoutW := io.Pipe()
	codes := make(chan string, 1)
	go func() {
		re := regexp.MustCompile(`^\s*(\d{8})\s*$`)
		sc := bufio.NewScanner(stdoutR)
		for sc.Scan() {
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				codes <- m[1]
			}
		}
	}()
	addrs := make(chan net.Addr, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cfg, zerolog.Nop(), Options{
			Plugs:    []plugs.Option{plugs.WithSignals(os.Interrupt)},
			Stdout:   stdoutW,
			OnListen: func(a net.Addr) { addrs <- a },
			NoReload: true,
		})
	}()
	base := "http://" + (<-addrs).String()

	post := func(path, body string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(body))
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		out, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, out
	}

	status, body := post("/v1/auth/start", `{"email":"dev@example.com"}`)
	require.Equal(t, http.StatusAccepted, status, string(body))

	var code string
	select {
	case code = <-codes:
	case <-time.After(15 * time.Second):
		t.Fatal("no sign-in code on stdout")
	}

	status, body = post("/v1/auth/verify", `{"email":"dev@example.com","code":"`+code+`","device":"test"}`)
	require.Equal(t, http.StatusOK, status, string(body))
	var signedIn struct {
		Token string `json:"token"`
		Kind  string `json:"kind"`
	}
	require.NoError(t, json.Unmarshal(body, &signedIn))
	assert.Equal(t, "device", signedIn.Kind)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/me", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+signedIn.Token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
