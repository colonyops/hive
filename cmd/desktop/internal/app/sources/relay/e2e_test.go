package relay_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ingest"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay"
	"github.com/colonyops/hive/internal/platform/credentials"
)

const e2eEmail = "e2e@example.com"

type flows []flow.Flow

func (f flows) List() []flow.Flow { return f }

// TestRelayEndToEnd runs the real relay binary and drives sources.relay
// through the desktop's producer, so the relay's stored shape, the client,
// the dialect detector and the ingest path are checked together.
func TestRelayEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the relay binary")
	}
	t.Parallel()

	base, key := startRelay(t)
	ref := credentials.Ref{Provider: relay.Provider, Account: "127.0.0.1-" + e2eEmail}
	creds := credentials.NewMemoryStore()
	require.NoError(t, creds.Set(ref, key))
	instances := relay.NewInstanceStore(filepath.Join(t.TempDir(), "relay-instances.json"))
	require.NoError(t, instances.Set(ref, relay.Binding{URL: base, Email: e2eEmail}))
	fetchers := relay.NewFetchers(instances, creds, "e2e", zerolog.Nop())

	client, err := fetchers.For(ref)
	require.NoError(t, err)
	src, err := client.CreateSource(t.Context(), "e2e", "")
	require.NoError(t, err)
	require.NotEmpty(t, src.IngressURL)

	ntfyID := deliver(t, http.MethodPost, src.IngressURL+"/alerts", "text/plain",
		map[string]string{"X-Title": "Backup failed", "X-Priority": "high", "X-Tags": "warning"}, "Backup of /home failed")
	gotifyID := deliver(t, http.MethodPost, src.IngressURL+"/message?token=x", "application/json",
		nil, `{"title":"Deploy","message":"Deployed v1.2.0","priority":5}`)
	deliver(t, http.MethodGet, src.IngressURL+"/backup/fail", "", nil, "")
	deliver(t, http.MethodPost, src.IngressURL, "application/json",
		nil, `{"id":"deploy-42","title":"Deploy failed","state":"open"}`)

	node := flow.Node{
		ID:   "relay",
		Type: relay.Descriptor.Type,
		Config: flow.NewSourceConfig(relay.Descriptor.Type, &relay.Config{
			Credential: ref.String(),
			Source:     src.ID,
		}),
	}
	resolver := ingest.NewResolver(flows{{ID: "inbox", Enabled: true, Nodes: []flow.Node{node}}},
		map[string]connector.Factory{relay.Descriptor.Type: relay.NewFactory(fetchers)}, zerolog.Nop())

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	st := stores.New(db, stores.Options{})
	producer := ingest.NewProducer(ingest.ProducerDeps{
		Ingester:  st.InboxItems,
		Snapshots: st.EventLog,
		Heads:     st.SourceHeads,
		Sources:   resolver,
		Interval:  time.Hour,
		Logger:    zerolog.Nop(),
	})

	summary := producer.Refresh(t.Context())
	require.Equal(t, 1, summary.Sources)
	require.Zero(t, summary.Failed)

	msgs, _, err := st.EventLog.ReadFrom(t.Context(), 0, 100)
	require.NoError(t, err)
	kinds := map[string]string{}
	states := map[string]any{}
	for _, m := range msgs {
		if m.Key == "" {
			continue
		}
		assert.Equal(t, "source:inbox/relay", m.Topic)
		assert.Equal(t, relay.SourceKind, m.SourceKind)
		var p map[string]any
		require.NoError(t, json.Unmarshal(m.Payload, &p))
		kinds[m.Key], _ = p["kind"].(string)
		states[m.Key] = p["state"]
	}
	assert.Equal(t, map[string]string{
		hash(ntfyID):   "Notification",
		hash(gotifyID): "Notification",
		"backup":       "Check",
		"deploy-42":    "Webhook",
	}, kinds)
	assert.Equal(t, "down", states["backup"])
	assert.Equal(t, "open", states["deploy-42"])
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func deliver(t *testing.T, method, target, contentType string, headers map[string]string, body string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck // test read
	data, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var out struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	return out.ID
}

// startRelay builds cmd/relay and serves it on a temporary data dir. It returns
// the base URL and the API key --mint prints as the first stdout line.
func startRelay(t *testing.T) (string, string) {
	t.Helper()
	root := moduleRoot(t)
	bin := filepath.Join(t.TempDir(), "hive-relay")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "./cmd/relay")
	build.Dir = root
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	port := freePort(t)
	base := "http://127.0.0.1:" + port
	cmd := exec.Command(bin, "serve", "--data-dir", t.TempDir(), "--listen", "127.0.0.1:"+port, "--public-url", base, "--mint", e2eEmail)
	cmd.Env = relayEnv()
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr lockedBuffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		if t.Failed() {
			t.Logf("relay stderr:\n%s", stderr.String())
		}
	})

	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- strings.TrimSpace(scanner.Text())
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	var key string
	select {
	case key = <-lines:
	case <-time.After(10 * time.Second):
		t.Fatalf("relay printed no key; stderr:\n%s", stderr.String())
	}
	require.NotEmpty(t, key)

	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz") //nolint:noctx // readiness probe
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("relay never became healthy; stderr:\n%s", stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	return base, key
}

// relayEnv drops RELAY_* so a developer's shell cannot reconfigure the run.
func relayEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "RELAY_") {
			env = append(env, kv)
		}
	}
	return env
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	require.NoError(t, err)
	gomod := strings.TrimSpace(string(out))
	require.NotEmpty(t, gomod)
	return filepath.Dir(gomod)
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	require.NoError(t, ln.Close())
	return port
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
