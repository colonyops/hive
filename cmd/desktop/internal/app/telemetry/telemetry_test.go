package telemetry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/pyroscope-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestOffEmitsNothing(t *testing.T) {
	p := Off()

	assert.False(t, p.Enabled())
	assert.Nil(t, p.MetricsHandler())
	assert.Nil(t, p.CPUProfileHandler())
	assert.Nil(t, p.LogWriter())
	assert.Empty(t, p.LogWriters())
	require.NoError(t, p.Shutdown(t.Context()))
}

// The whole point of registering globally: a package declares an instrument
// against otel.Meter and it reaches this provider's readers with nothing handed
// to it (ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape).
func TestNewRegistersTheGlobalMeterProvider(t *testing.T) {
	p, err := New(t.Context(), Options{Scrape: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.WithoutCancel(t.Context())) })

	counter, err := otel.Meter("telemetry-test").Int64Counter("registration.probe")
	require.NoError(t, err)
	counter.Add(t.Context(), 3)

	assert.Contains(t, scrape(t, p.MetricsHandler()), "registration_probe")
}

// Spans have no local sink, so the scrape gate alone must leave the global
// tracer no-op: a span opened with nothing exporting stays non-recording rather
// than accumulating in a provider no reader drains.
func TestScrapeAloneLeavesTheGlobalTracerNoop(t *testing.T) {
	p, err := New(t.Context(), Options{Scrape: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.WithoutCancel(t.Context())) })

	_, span := otel.Tracer("telemetry-test").Start(t.Context(), "probe")
	span.End()

	assert.False(t, span.SpanContext().IsValid())
}

func TestNewWithAllGatesOffIsOff(t *testing.T) {
	p, err := New(t.Context(), Options{})
	require.NoError(t, err)
	assert.False(t, p.Enabled())
	assert.Nil(t, p.MetricsHandler())
	assert.Nil(t, p.CPUProfileHandler())
}

func TestNewRejectsUnusableExportConfig(t *testing.T) {
	base := Options{
		Export:   true,
		Endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp",
		User:     "123456",
		Token:    "glc_secret",
	}

	tests := []struct {
		name string
		edit func(*Options)
		want string
	}{
		{"no endpoint", func(o *Options) { o.Endpoint = "" }, "endpoint is required"},
		{"no user", func(o *Options) { o.User = "" }, "user is required"},
		{"no token", func(o *Options) { o.Token = "" }, "token is required"},
		{"plaintext endpoint", func(o *Options) { o.Endpoint = "http://gateway.example.com/otlp" }, "must use https"},
		{"endpoint has no host", func(o *Options) { o.Endpoint = "https:///otlp" }, "no host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := base
			tt.edit(&opts)

			p, err := New(t.Context(), opts)
			require.Error(t, err)
			assert.Nil(t, p)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// The scrape gate stands alone: no endpoint and no token, so the local debug
// loop works with no account configured.
func TestScrapeWithoutExport(t *testing.T) {
	p, err := New(t.Context(), Options{
		Scrape:            true,
		Version:           "1.4.0-dev.3",
		Environment:       "dev",
		HostID:            "machine-a",
		serviceInstanceID: "8ae6b36d-442a-4ba2-bad4-99bec9dc8f33",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.WithoutCancel(t.Context())) })

	assert.True(t, p.Enabled())
	require.NotNil(t, p.MetricsHandler())
	assert.Nil(t, p.LogWriter())

	body := scrape(t, p.MetricsHandler())
	assert.Contains(t, body, "go_memory_used_bytes")

	// The resource identity has to reach the exposition, or a query written
	// locally is not the same query run against the backend.
	assert.Contains(t, body, "target_info")
	for _, want := range []string{
		`service_name="hive-desktop"`,
		`service_version="1.4.0-dev.3"`,
		`deployment_environment_name="dev"`,
		`service_instance_id="8ae6b36d-442a-4ba2-bad4-99bec9dc8f33"`,
		`host_id="machine-a"`,
	} {
		assert.Contains(t, body, want)
	}
}

func TestNewGeneratesServiceInstanceIDAndOmitsEmptyOptionalAttributes(t *testing.T) {
	p, err := New(t.Context(), Options{Scrape: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.WithoutCancel(t.Context())) })

	body := scrape(t, p.MetricsHandler())
	assert.Contains(t, body, `service_name="hive-desktop"`)
	assert.Regexp(t, `service_instance_id="[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}"`, body)
	assert.NotContains(t, body, `service_version=""`)
	assert.NotContains(t, body, `deployment_environment_name=""`)
	assert.NotContains(t, body, `host_id=""`)
}

func TestProfileOnlyMode(t *testing.T) {
	var got pyroscope.Config
	profile := &fakeProfiler{}
	p, err := newProvider(t.Context(), profileOptions(), func(config pyroscope.Config) (profiler, error) {
		got = config
		return profile, nil
	})
	require.NoError(t, err)

	assert.True(t, p.Enabled())
	assert.Nil(t, p.MetricsHandler())
	assert.NotNil(t, p.CPUProfileHandler())
	assert.Nil(t, p.LogWriter())
	assert.Equal(t, ServiceName, got.ApplicationName)
	assert.Equal(t, "https://profiles.example.com", got.ServerAddress)
	assert.Equal(t, "profiles-user", got.BasicAuthUser)
	assert.Equal(t, "profiles-token", got.BasicAuthPassword)
	assert.True(t, got.DisableGCRuns)
	assert.Equal(t, []pyroscope.ProfileType{
		pyroscope.ProfileCPU,
		pyroscope.ProfileAllocObjects,
		pyroscope.ProfileAllocSpace,
		pyroscope.ProfileInuseObjects,
		pyroscope.ProfileInuseSpace,
	}, got.ProfileTypes)
	assert.Equal(t, map[string]string{
		"service_version":             "1.4.0-dev.3",
		"deployment_environment_name": "dev",
		"service_instance_id":         "8ae6b36d-442a-4ba2-bad4-99bec9dc8f33",
		"host_id":                     "machine-a",
	}, got.Tags)

	require.NoError(t, p.Shutdown(t.Context()))
	assert.Equal(t, 1, profile.stops)
	require.NoError(t, p.Shutdown(t.Context()))
	assert.Equal(t, 1, profile.stops)
}

func TestNewSharesGeneratedServiceInstanceIDAcrossSignals(t *testing.T) {
	var got pyroscope.Config
	opts := profileOptions()
	opts.Scrape = true
	opts.serviceInstanceID = ""

	p, err := newProvider(t.Context(), opts, func(config pyroscope.Config) (profiler, error) {
		got = config
		return &fakeProfiler{}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.WithoutCancel(t.Context())) })

	instanceID := got.Tags["service_instance_id"]
	_, err = uuid.Parse(instanceID)
	require.NoError(t, err)
	assert.Contains(t, scrape(t, p.MetricsHandler()), `service_instance_id="`+instanceID+`"`)
}

func TestProfileExporterSendsIngestRequest(t *testing.T) {
	type request struct {
		path string
		user string
		pass string
		name string
	}
	uploads := make(chan request, 8)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		uploads <- request{path: r.URL.Path, user: user, pass: pass, name: r.URL.Query().Get("name")}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := profileOptions()
	opts.Profiles.Endpoint = server.URL
	config := profileConfig(opts)
	config.HTTPClient = server.Client()
	config.UploadRate = time.Hour
	started, err := startPyroscope(config)
	require.NoError(t, err)
	profile, ok := started.(*pyroscope.Profiler)
	require.True(t, ok)
	defer func() { assert.NoError(t, profile.Stop()) }()

	until := time.Now().Add(25 * time.Millisecond)
	for i := 0; time.Now().Before(until); i++ {
		_ = i * i
	}
	profile.Flush(true)

	select {
	case got := <-uploads:
		assert.Equal(t, "/ingest", got.path)
		assert.Equal(t, "profiles-user", got.user)
		assert.Equal(t, "profiles-token", got.pass)
		assert.Contains(t, got.name, ServiceName)
		assert.Contains(t, got.name, "service_version=1.4.0-dev.3")
		assert.Contains(t, got.name, "deployment_environment_name=dev")
		assert.Contains(t, got.name, "service_instance_id=8ae6b36d-442a-4ba2-bad4-99bec9dc8f33")
		assert.Contains(t, got.name, "host_id=machine-a")
	case <-time.After(2 * time.Second):
		t.Fatal("profile upload did not reach the ingest endpoint")
	}
}

func TestProfileConfigOmitsEmptyTagsAndBoundsHTTP(t *testing.T) {
	config := profileConfig(Options{Profiles: ProfilesOptions{
		Enabled:     true,
		Endpoint:    "https://profiles.example.com",
		User:        "profiles-user",
		Token:       "profiles-token",
		HTTPTimeout: 750 * time.Millisecond,
	}})

	assert.Empty(t, config.Tags)
	client, ok := config.HTTPClient.(*http.Client)
	require.True(t, ok)
	assert.Equal(t, 750*time.Millisecond, client.Timeout)
}

func TestProfileEndpointCannotBeOverriddenByPyroscopeEnvironment(t *testing.T) {
	t.Setenv(pyroscopeAdhocServerAddressEnv, "http://attacker.example.com")

	p, err := New(t.Context(), profileOptions())

	assert.Nil(t, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), pyroscopeAdhocServerAddressEnv)
}

func TestProfileStartupFailureIsAtomic(t *testing.T) {
	want := errors.New("cannot profile")
	p, err := newProvider(t.Context(), profileOptions(), func(pyroscope.Config) (profiler, error) {
		return nil, want
	})

	assert.Nil(t, p)
	assert.ErrorIs(t, err, want)
}

func TestProfileShutdownRespectsContext(t *testing.T) {
	unblock := make(chan struct{})
	profile := &fakeProfiler{unblock: unblock}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := stopProfiler(profile)(ctx)
	require.ErrorIs(t, err, context.Canceled)
	close(unblock)
}

func TestNewRejectsUnusableProfileConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ProfilesOptions)
		want string
	}{
		{"no endpoint", func(o *ProfilesOptions) { o.Endpoint = "" }, "endpoint is required"},
		{"no user", func(o *ProfilesOptions) { o.User = "" }, "user is required"},
		{"no token", func(o *ProfilesOptions) { o.Token = "" }, "token is required"},
		{"plaintext endpoint", func(o *ProfilesOptions) { o.Endpoint = "http://profiles.example.com" }, "must use https"},
		{"endpoint has no host", func(o *ProfilesOptions) { o.Endpoint = "https:///ingest" }, "no host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := profileOptions()
			tt.edit(&opts.Profiles)
			p, err := newProvider(t.Context(), opts, func(pyroscope.Config) (profiler, error) {
				t.Fatal("profile starter called for invalid configuration")
				return nil, nil
			})
			require.Error(t, err)
			assert.Nil(t, p)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestShutdownIsRepeatable(t *testing.T) {
	p, err := New(t.Context(), Options{Scrape: true})
	require.NoError(t, err)

	require.NoError(t, p.Shutdown(t.Context()))
	require.NoError(t, p.Shutdown(t.Context()))
}

type fakeProfiler struct {
	stops   int
	unblock <-chan struct{}
}

func (p *fakeProfiler) Stop() error {
	p.stops++
	if p.unblock != nil {
		<-p.unblock
	}
	return nil
}

func profileOptions() Options {
	return Options{
		Profiles: ProfilesOptions{
			Enabled:  true,
			Endpoint: "https://profiles.example.com",
			User:     "profiles-user",
			Token:    "profiles-token",
		},
		Version:           "1.4.0-dev.3",
		Environment:       "dev",
		HostID:            "machine-a",
		serviceInstanceID: "8ae6b36d-442a-4ba2-bad4-99bec9dc8f33",
	}
}

func TestSignalURL(t *testing.T) {
	const want = "https://gw.example.com/otlp/v1/traces"
	assert.Equal(t, want, signalURL("https://gw.example.com/otlp", "traces"))
	assert.Equal(t, want, signalURL("https://gw.example.com/otlp/", "traces"))
	assert.Equal(t, want, signalURL("  https://gw.example.com/otlp  ", "traces"))
}

func TestBasicAuth(t *testing.T) {
	// base64("123456:glc_secret")
	assert.Equal(t, "Basic MTIzNDU2OmdsY19zZWNyZXQ=", basicAuth("123456", "glc_secret"))
}

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, MetricsPath, nil))
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	require.NotEmpty(t, strings.TrimSpace(body))
	return body
}
