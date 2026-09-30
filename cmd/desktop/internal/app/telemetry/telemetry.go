// Package telemetry configures the OpenTelemetry and Pyroscope SDKs, registers
// the OTel providers globally, and owns their shutdown (ADR
// continuous-profiles-are-pushed-directly-with-pyroscope). It is SDK setup,
// not an instrumentation API; a package that emits reaches the registered OTel
// providers through internal/app/observe.
//
// OTLP export, profile export, and local scrape are independent gates. With
// every gate off, [New] returns what [Off] returns and the globals keep the
// API's no-op default.
package telemetry

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/pyroscope-go"
	pyroscopepprof "github.com/grafana/pyroscope-go/http/pprof"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	runtimemetrics "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	MetricsPath = "/metrics"

	ScopeName = "github.com/colonyops/hive/cmd/desktop"
)

type Options struct {
	// Endpoint is the signal-less OTLP base
	// ("https://otlp-gateway-<zone>.grafana.net/otlp"); per-signal paths are
	// appended. On Grafana Cloud User is the stack's OTLP instance id, which is
	// not the stack id.
	Export   bool
	Endpoint string
	User     string
	Token    string

	Profiles ProfilesOptions
	Scrape   bool

	Version     string
	Environment string
	HostID      string

	serviceInstanceID string
}

// ProfilesOptions configures direct Pyroscope profile export.
type ProfilesOptions struct {
	Enabled     bool
	Endpoint    string
	User        string
	Token       string
	HTTPTimeout time.Duration
}

func (o Options) validate() error {
	if o.Export {
		if err := validateDestination("telemetry", o.Endpoint, o.User, o.Token); err != nil {
			return err
		}
	}
	if o.Profiles.Enabled {
		if err := validateDestination("telemetry profiles", o.Profiles.Endpoint, o.Profiles.User, o.Profiles.Token); err != nil {
			return err
		}
	}
	return nil
}

func validateDestination(name, endpoint, user, token string) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("%s: endpoint is required to export", name)
	}
	if strings.TrimSpace(user) == "" {
		return fmt.Errorf("%s: user is required to export", name)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%s: token is required to export", name)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%s: endpoint is not a URL: %w", name, err)
	}
	// https rather than the loopback rule development.github.api_base follows:
	// these endpoints are remote by definition, so what is left to enforce is
	// that the credentials do not cross the network in the clear.
	if parsed.Scheme != "https" {
		return fmt.Errorf("%s: endpoint must use https", name)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s: endpoint has no host", name)
	}
	return nil
}

type Provider struct {
	metricsHandler    http.Handler
	cpuProfileHandler http.Handler
	logw              io.Writer

	// Run in reverse, so a batch processor flushes before its exporter closes.
	shutdown []func(context.Context) error
}

// Off registers nothing, so the globals keep the API's no-op default and a
// measurement is safe to take without checking whether telemetry is configured.
func Off() *Provider { return &Provider{} }

type profiler interface {
	Stop() error
}

type profileStarter func(pyroscope.Config) (profiler, error)

// New returns [Off] when every gate is off, and an error when an enabled
// destination is misconfigured. A stated endpoint that cannot be used is worth
// reporting rather than silently dropping telemetry over.
func New(ctx context.Context, opts Options) (*Provider, error) {
	return newProvider(ctx, opts, startPyroscope)
}

const pyroscopeAdhocServerAddressEnv = "PYROSCOPE_ADHOC_SERVER_ADDRESS"

func startPyroscope(config pyroscope.Config) (profiler, error) {
	// The SDK applies this override after our HTTPS validation and retains the
	// configured basic-auth credentials.
	if _, exists := os.LookupEnv(pyroscopeAdhocServerAddressEnv); exists {
		return nil, fmt.Errorf("telemetry profiles: %s is not supported; configure telemetry.profiles.endpoint", pyroscopeAdhocServerAddressEnv)
	}
	return pyroscope.Start(config)
}

func newProvider(ctx context.Context, opts Options, startProfiles profileStarter) (*Provider, error) {
	if !opts.Export && !opts.Profiles.Enabled && !opts.Scrape {
		return Off(), nil
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}
	if opts.serviceInstanceID == "" {
		instanceID, err := uuid.NewRandom()
		if err != nil {
			return nil, fmt.Errorf("telemetry: generate service instance id: %w", err)
		}
		opts.serviceInstanceID = instanceID.String()
	}

	p := Off()
	fail := func(err error) (*Provider, error) {
		_ = p.Shutdown(ctx)
		return nil, err
	}

	var (
		mp *sdkmetric.MeterProvider
		tp *sdktrace.TracerProvider
	)
	if opts.Export || opts.Scrape {
		res, err := newResource(opts)
		if err != nil {
			return nil, err
		}

		var readers []sdkmetric.Option
		if opts.Scrape {
			// A private registry, never promclient.DefaultRegisterer — the reason
			// PprofHandler builds its own mux.
			reg := promclient.NewRegistry()
			promReader, err := otelprom.New(otelprom.WithRegisterer(reg))
			if err != nil {
				return fail(fmt.Errorf("telemetry: prometheus reader: %w", err))
			}
			readers = append(readers, sdkmetric.WithReader(promReader))
			p.metricsHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
		}

		if opts.Export {
			headers := map[string]string{"Authorization": basicAuth(opts.User, opts.Token)}

			metricExp, err := otlpmetrichttp.New(ctx,
				otlpmetrichttp.WithEndpointURL(signalURL(opts.Endpoint, "metrics")),
				otlpmetrichttp.WithHeaders(headers),
				otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
			)
			if err != nil {
				return fail(fmt.Errorf("telemetry: metric exporter: %w", err))
			}
			readers = append(readers, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)))

			traceExp, err := otlptracehttp.New(ctx,
				otlptracehttp.WithEndpointURL(signalURL(opts.Endpoint, "traces")),
				otlptracehttp.WithHeaders(headers),
				otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
			)
			if err != nil {
				return fail(fmt.Errorf("telemetry: trace exporter: %w", err))
			}
			tp = sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExp), sdktrace.WithResource(res))
			p.shutdown = append(p.shutdown, tp.Shutdown)

			logExp, err := otlploghttp.New(ctx,
				otlploghttp.WithEndpointURL(signalURL(opts.Endpoint, "logs")),
				otlploghttp.WithHeaders(headers),
				otlploghttp.WithCompression(otlploghttp.GzipCompression),
			)
			if err != nil {
				return fail(fmt.Errorf("telemetry: log exporter: %w", err))
			}
			lp := sdklog.NewLoggerProvider(
				sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
				sdklog.WithResource(res),
			)
			p.logw = newLogWriter(ctx, lp.Logger(ScopeName))
			p.shutdown = append(p.shutdown, lp.Shutdown)
		}

		mp = sdkmetric.NewMeterProvider(append(readers, sdkmetric.WithResource(res))...)
		p.shutdown = append(p.shutdown, mp.Shutdown)

		if err := runtimemetrics.Start(runtimemetrics.WithMeterProvider(mp)); err != nil {
			return fail(fmt.Errorf("telemetry: runtime metrics: %w", err))
		}
	}

	if opts.Profiles.Enabled {
		profile, err := startProfiles(profileConfig(opts))
		if err != nil {
			return fail(fmt.Errorf("telemetry: profile exporter: %w", err))
		}
		p.cpuProfileHandler = http.HandlerFunc(pyroscopepprof.Profile)
		p.shutdown = append(p.shutdown, stopProfiler(profile))
	}

	// Last, so a construction failure never leaves a provider fail() has already
	// shut down reachable through the global. Spans have no local sink, so the
	// TracerProvider registers only for export.
	if tp != nil {
		otel.SetTracerProvider(tp)
	}
	if mp != nil {
		otel.SetMeterProvider(mp)
	}

	return p, nil
}

func (p *Provider) Enabled() bool                   { return len(p.shutdown) > 0 || p.metricsHandler != nil }
func (p *Provider) MetricsHandler() http.Handler    { return p.metricsHandler }
func (p *Provider) CPUProfileHandler() http.Handler { return p.cpuProfileHandler }
func (p *Provider) LogWriter() io.Writer            { return p.logw }

// LogWriters is LogWriter as a variadic-friendly slice for settings.NewLogger.
func (p *Provider) LogWriters() []io.Writer {
	if p.logw == nil {
		return nil
	}
	return []io.Writer{p.logw}
}

// Shutdown is safe on an Off provider and safe to call more than once.
func (p *Provider) Shutdown(ctx context.Context) error {
	var errs []error
	for _, fn := range slices.Backward(p.shutdown) {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	p.shutdown = nil
	return errors.Join(errs...)
}

const defaultProfileHTTPTimeout = 2 * time.Second

func profileConfig(opts Options) pyroscope.Config {
	timeout := opts.Profiles.HTTPTimeout
	if timeout <= 0 {
		timeout = defaultProfileHTTPTimeout
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	transport = transport.Clone()
	transport.MaxConnsPerHost = 5
	return pyroscope.Config{
		ApplicationName:   ServiceName,
		ServerAddress:     strings.TrimSpace(opts.Profiles.Endpoint),
		BasicAuthUser:     opts.Profiles.User,
		BasicAuthPassword: opts.Profiles.Token,
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
		},
		DisableGCRuns: true,
		Tags:          profileTags(opts),
		HTTPClient: &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: timeout,
		},
	}
}

func profileTags(opts Options) map[string]string {
	tags := make(map[string]string, 4)
	for key, value := range map[string]string{
		"service_version":             opts.Version,
		"service_instance_id":         opts.serviceInstanceID,
		"deployment_environment_name": opts.Environment,
		"host_id":                     opts.HostID,
	} {
		if value != "" {
			tags[key] = value
		}
	}
	return tags
}

func stopProfiler(profile profiler) func(context.Context) error {
	return func(ctx context.Context) error {
		stopped := make(chan error, 1)
		go func() { stopped <- profile.Stop() }()
		select {
		case err := <-stopped:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func signalURL(base, signal string) string {
	return strings.TrimSuffix(strings.TrimSpace(base), "/") + "/v1/" + signal
}

func basicAuth(user, token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+token))
}
