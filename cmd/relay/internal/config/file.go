package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

const (
	envAllowedEmails = "RELAY_ALLOWED_EMAILS"
	envSMTPURL       = "RELAY_SMTP_URL"
	envSMTPPassword  = "RELAY_SMTP_PASSWORD"
	envMailFrom      = "RELAY_MAIL_FROM"

	// SMTPLog is the SMTP URL that prints mail to stdout instead of sending it.
	SMTPLog = "log"
)

type User struct {
	Email        string `yaml:"email"`
	PasswordHash string `yaml:"password_hash"`
}

type SMTP struct {
	URL string
	// Password is a secrets reference (env:, file:, op://), resolved at send.
	Password string
	From     string
}

// fileConfig is relay.yaml. Pointers tell an absent key from a zero value, so
// only keys the file sets override a flag's default.
type fileConfig struct {
	Listen              *string        `yaml:"listen"`
	PublicURL           *string        `yaml:"public_url"`
	DataDir             *string        `yaml:"data_dir"`
	TrustedProxies      []string       `yaml:"trusted_proxies"`
	LogLevel            *string        `yaml:"log_level"`
	BodyBytes           *int64         `yaml:"body_bytes"`
	Retention           *time.Duration `yaml:"retention"`
	DeliveriesPerSource *int           `yaml:"deliveries_per_source"`
	BytesPerAccount     *int64         `yaml:"bytes_per_account"`
	SourcesPerAccount   *int           `yaml:"sources_per_account"`
	Concurrency         *int           `yaml:"concurrency"`
	AllowedEmails       []string       `yaml:"allowed_emails"`
	Users               []User         `yaml:"users"`
	SMTP                struct {
		URL      *string `yaml:"url"`
		Password *string `yaml:"password"`
		From     *string `yaml:"from"`
	} `yaml:"smtp"`
}

func readFile(p string) (fileConfig, error) {
	var f fileConfig
	if p == "" {
		return f, nil
	}
	data, err := os.ReadFile(p)
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

// access resolves the allowlist and the user list: RELAY_ALLOWED_EMAILS wins
// over the file when set, and users come from the file only.
func access(f fileConfig) ([]string, []User, error) {
	allowed := f.AllowedEmails
	if v, ok := os.LookupEnv(envAllowedEmails); ok {
		allowed = strings.Split(v, ",")
	}
	var errs []error
	patterns := make([]string, 0, len(allowed))
	for _, p := range allowed {
		p = relay.NormalizeEmail(p)
		if p == "" {
			continue
		}
		if _, err := path.Match(p, ""); err != nil {
			errs = append(errs, fmt.Errorf("allowed_emails: pattern %q: %w", p, err))
			continue
		}
		patterns = append(patterns, p)
	}

	users := make([]User, 0, len(f.Users))
	seen := map[string]bool{}
	for i, u := range f.Users {
		u.Email = relay.NormalizeEmail(u.Email)
		switch {
		case u.Email == "":
			errs = append(errs, fmt.Errorf("users[%d]: email is required", i))
		case seen[u.Email]:
			errs = append(errs, fmt.Errorf("users[%d]: %s is listed twice", i, u.Email))
		default:
			if err := relay.ParsePasswordHash(u.PasswordHash); err != nil {
				errs = append(errs, fmt.Errorf("users[%d] (%s): %w; make one with `hive-relay hash-password`", i, u.Email, err))
			}
		}
		seen[u.Email] = true
		users = append(users, u)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}
	return patterns, users, nil
}

func smtpConfig(f fileConfig) SMTP {
	pick := func(env string, file *string) string {
		if v, ok := os.LookupEnv(env); ok {
			return strings.TrimSpace(v)
		}
		if file != nil {
			return strings.TrimSpace(*file)
		}
		return ""
	}
	return SMTP{
		URL:      pick(envSMTPURL, f.SMTP.URL),
		Password: pick(envSMTPPassword, f.SMTP.Password),
		From:     pick(envMailFrom, f.SMTP.From),
	}
}

// Reload re-reads the config file and env for SIGHUP. Only the allowlist and
// the user list change; everything else stays as the process started.
func (c Config) Reload() (Config, error) {
	f, err := readFile(c.ConfigFile)
	if err != nil {
		return c, err
	}
	allowed, users, err := access(f)
	if err != nil {
		return c, err
	}
	c.AllowedEmails, c.Users = allowed, users
	return c, nil
}

func (c Config) UserEmails() []string {
	out := make([]string, len(c.Users))
	for i, u := range c.Users {
		out[i] = u.Email
	}
	return out
}

// UserHashes maps each listed user's email to its password hash.
func (c Config) UserHashes() map[string]string {
	out := make(map[string]string, len(c.Users))
	for _, u := range c.Users {
		out[u.Email] = u.PasswordHash
	}
	return out
}

// Enabled reports whether mail is configured, which email sign-in needs.
// "log" counts: a dev relay offers codes on stdout.
func (s SMTP) Enabled() bool { return s.URL != "" }

// IsLog reports whether mail goes to stdout rather than an SMTP server.
func (s SMTP) IsLog() bool { return s.URL == "" || s.URL == SMTPLog }
