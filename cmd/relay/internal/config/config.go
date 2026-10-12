// Package config resolves the relay's settings from flags and RELAY_* env.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

const (
	StoreMemory = "memory"
	StoreSQLite = "sqlite"
)

const (
	flagListen              = "listen"
	flagPublicURL           = "public-url"
	flagStore               = "store"
	flagDataDir             = "data-dir"
	flagTrustedProxies      = "trusted-proxies"
	flagLogLevel            = "log-level"
	flagBodyBytes           = "body-bytes"
	flagRetention           = "retention"
	flagDeliveriesPerSource = "deliveries-per-source"
	flagBytesPerAccount     = "bytes-per-account"
	flagSourcesPerAccount   = "sources-per-account"
	flagConcurrency         = "concurrency"

	// FlagMint is the serve-only flag that mints a dev API key at start.
	FlagMint = "mint"
)

type Config struct {
	Listen         string
	PublicURL      string
	Store          string
	DataDir        string
	TrustedProxies []netip.Prefix
	MintFor        string
	Limits         relay.Limits
	LogLevel       string
}

// Flags returns fresh flag values on every call: a cli.Flag holds parse state,
// so commands must not share one.
func Flags() []cli.Flag {
	d := relay.DefaultLimits()
	return []cli.Flag{
		&cli.StringFlag{Name: flagListen, Usage: "address to listen on", Value: ":8080", Sources: cli.EnvVars("RELAY_LISTEN")},
		&cli.StringFlag{Name: flagPublicURL, Usage: "public base URL senders reach, used to build ingress URLs", Sources: cli.EnvVars("RELAY_PUBLIC_URL")},
		&cli.StringFlag{Name: flagStore, Usage: "store driver: memory or sqlite", Value: StoreSQLite, Sources: cli.EnvVars("RELAY_STORE")},
		&cli.StringFlag{Name: flagDataDir, Usage: "directory for the sqlite store (relay.db)", Value: ".", Sources: cli.EnvVars("RELAY_DATA_DIR")},
		&cli.StringFlag{Name: flagTrustedProxies, Usage: "comma-separated CIDRs whose X-Forwarded-For is honoured", Sources: cli.EnvVars("RELAY_TRUSTED_PROXIES")},
		&cli.StringFlag{Name: flagLogLevel, Usage: "log level (debug, info, warn, error)", Value: "info", Sources: cli.EnvVars("RELAY_LOG_LEVEL")},
		&cli.Int64Flag{Name: flagBodyBytes, Usage: "max ingress body bytes", Value: d.BodyBytes, Sources: cli.EnvVars("RELAY_BODY_BYTES")},
		&cli.DurationFlag{Name: flagRetention, Usage: "how long deliveries are kept", Value: d.Retention, Sources: cli.EnvVars("RELAY_RETENTION")},
		&cli.IntFlag{Name: flagDeliveriesPerSource, Usage: "deliveries kept per source", Value: d.DeliveriesPerSource, Sources: cli.EnvVars("RELAY_DELIVERIES_PER_SOURCE")},
		&cli.Int64Flag{Name: flagBytesPerAccount, Usage: "stored bytes allowed per account", Value: d.BytesPerAccount, Sources: cli.EnvVars("RELAY_BYTES_PER_ACCOUNT")},
		&cli.IntFlag{Name: flagSourcesPerAccount, Usage: "sources allowed per account", Value: d.SourcesPerAccount, Sources: cli.EnvVars("RELAY_SOURCES_PER_ACCOUNT")},
		&cli.IntFlag{Name: flagConcurrency, Usage: "in-flight ingress requests", Value: d.Concurrency, Sources: cli.EnvVars("RELAY_CONCURRENCY")},
	}
}

// Load reads cmd's flags, which must include Flags(), into a validated Config.
func Load(cmd *cli.Command) (Config, error) {
	limits := relay.DefaultLimits()
	limits.BodyBytes = cmd.Int64(flagBodyBytes)
	limits.Retention = cmd.Duration(flagRetention)
	limits.DeliveriesPerSource = cmd.Int(flagDeliveriesPerSource)
	limits.BytesPerAccount = cmd.Int64(flagBytesPerAccount)
	limits.SourcesPerAccount = cmd.Int(flagSourcesPerAccount)
	limits.Concurrency = cmd.Int(flagConcurrency)

	cfg := Config{
		Listen:    cmd.String(flagListen),
		PublicURL: strings.TrimRight(cmd.String(flagPublicURL), "/"),
		Store:     cmd.String(flagStore),
		DataDir:   cmd.String(flagDataDir),
		MintFor:   strings.TrimSpace(cmd.String(FlagMint)),
		Limits:    limits,
		LogLevel:  cmd.String(flagLogLevel),
	}

	var errs []error
	proxies, err := ParseTrustedProxies(cmd.String(flagTrustedProxies))
	errs = append(errs, err)
	cfg.TrustedProxies = proxies
	errs = append(errs, cfg.validate())
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// RequirePublicURL is the extra check serve makes: without a public URL the
// relay cannot tell anyone where to send.
func (c Config) RequirePublicURL() error {
	if c.PublicURL == "" {
		return errors.New("--public-url (RELAY_PUBLIC_URL) is required")
	}
	return nil
}

func (c Config) validate() error {
	var errs []error
	switch c.Store {
	case StoreMemory, StoreSQLite:
	default:
		errs = append(errs, fmt.Errorf("--store must be %q or %q, got %q", StoreMemory, StoreSQLite, c.Store))
	}
	if c.MintFor != "" && c.Store != StoreMemory {
		errs = append(errs, errors.New("--mint works only with --store memory; mint a persistent key with `hive-relay tokens create`"))
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("--public-url must be an absolute http(s) URL, got %q", c.PublicURL))
		}
	}
	if c.Listen == "" {
		errs = append(errs, errors.New("--listen is required"))
	}
	if _, err := zerolog.ParseLevel(c.LogLevel); err != nil {
		errs = append(errs, fmt.Errorf("--log-level: %w", err))
	}
	l := c.Limits
	if l.BodyBytes <= 0 || l.DeliveriesPerSource <= 0 || l.BytesPerAccount <= 0 ||
		l.SourcesPerAccount <= 0 || l.Concurrency <= 0 || l.Retention <= 0 {
		errs = append(errs, errors.New("every limit must be positive"))
	}
	return errors.Join(errs...)
}

// ParseTrustedProxies parses a comma-separated list of CIDRs. A bare address
// becomes a single-host prefix.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q is not a CIDR or an IP", part)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// SQLitePath is where the sqlite driver keeps its database.
func (c Config) SQLitePath() string {
	return filepath.Join(c.DataDir, "relay.db")
}
