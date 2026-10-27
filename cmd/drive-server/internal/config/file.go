package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// fileConfig is drive-server.yaml. Pointers tell an absent key from a zero
// value, so only keys the file sets override a default.
type fileConfig struct {
	BaseURL         *string  `yaml:"base_url"`
	Listen          *string  `yaml:"listen"`
	DataDir         *string  `yaml:"data_dir"`
	MaxDocumentSize *Size    `yaml:"max_document_size"`
	TrustedProxies  []string `yaml:"trusted_proxies"`
	SMTP            *SMTP    `yaml:"smtp"`
	UserPolicies    []Policy `yaml:"user_policies"`
}

// Load resolves the config: defaults, then drive-server.yaml, then HIVE_DRIVE_SERVER_*
// env, then flags. It does not validate; call Validate on the result.
func Load(getenv func(string) string, flags Flags) (Config, error) {
	env := func(name string) string { return strings.TrimSpace(getenv("HIVE_DRIVE_SERVER_" + name)) }
	first := func(values ...string) string {
		for _, v := range values {
			if v != "" {
				return v
			}
		}
		return ""
	}

	configDir := first(flags.ConfigDir, env("CONFIG_DIR"), DefaultConfigDir)
	f, err := readFile(filepath.Join(configDir, ConfigFileName))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		BaseURL:         valueOr(f.BaseURL, ""),
		Listen:          valueOr(f.Listen, DefaultListen),
		DataDir:         valueOr(f.DataDir, DefaultDataDir),
		ConfigDir:       configDir,
		MaxDocumentSize: valueOr(f.MaxDocumentSize, Size{Bytes: DefaultMaxDocumentSize}),
		SMTP:            valueOr(f.SMTP, SMTP{}),
		TrustedProxies:  f.TrustedProxies,
		UserPolicies:    f.UserPolicies,
	}

	cfg.BaseURL = first(flags.BaseURL, env("BASE_URL"), cfg.BaseURL)
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	cfg.Listen = first(flags.Listen, env("LISTEN"), cfg.Listen)
	cfg.DataDir = first(flags.DataDir, env("DATA_DIR"), cfg.DataDir)
	cfg.SMTP.URL = first(env("SMTP_URL"), cfg.SMTP.URL)
	cfg.SMTP.From = first(env("SMTP_FROM"), cfg.SMTP.From)
	cfg.SMTP.Password = first(env("SMTP_PASSWORD"), cfg.SMTP.Password)

	if v := first(flags.TrustedProxies, env("TRUSTED_PROXIES")); v != "" {
		cfg.TrustedProxies = splitList(v)
	}

	if v := first(flags.MaxDocumentSize, env("MAX_DOCUMENT_SIZE")); v != "" {
		if err := cfg.MaxDocumentSize.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("max_document_size: %w", err)
		}
	}

	cfg.DevLogMail = flags.DevLogMail
	if v := env("DEV_LOG_MAIL"); v != "" && !flags.DevLogMail {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("HIVE_DRIVE_SERVER_DEV_LOG_MAIL: %w", err)
		}
		cfg.DevLogMail = b
	}
	return cfg, nil
}

func valueOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// readFile returns an empty config when the file or its directory is
// missing, so a container with no mounted config still starts.
func readFile(p string) (fileConfig, error) {
	var f fileConfig
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, fmt.Errorf("read config file: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return f, fmt.Errorf("parse config file %s: %w", p, err)
	}
	return f, nil
}
