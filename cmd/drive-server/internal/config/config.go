// Package config resolves the drive server's settings. Flags win over env,
// and env wins over <ConfigDir>/drive-server.yaml.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/colonyops/hive/internal/platform/secrets"
)

const (
	DefaultListen          = ":8080"
	DefaultDataDir         = "/data"
	DefaultConfigDir       = "/config"
	DefaultMaxDocumentSize = 5 << 20

	// ConfigFileName is the YAML file inside ConfigDir.
	ConfigFileName = "drive-server.yaml"
)

type Config struct {
	BaseURL         string
	Listen          string
	DataDir         string
	ConfigDir       string
	MaxDocumentSize Size
	SMTP            SMTP
	DevLogMail      bool
	TrustedProxies  []string
	UserPolicies    []Policy
}

// SMTP is read from slice 7 on; Validate only checks its shape.
type SMTP struct {
	URL  string `yaml:"url"`
	From string `yaml:"from"`
	// Password is a secret reference (env:, file:, op://), never the password.
	Password string `yaml:"password"`
}

// Policy admits the emails its Matches globs match. The first policy that
// matches an email applies.
type Policy struct {
	Name    string   `yaml:"name"`
	Matches []string `yaml:"matches"`
	// SignUp defaults to true when nil.
	SignUp *bool  `yaml:"sign_up"`
	Quotas Quotas `yaml:"quotas"`
}

// SignUpAllowed reports whether an email that matches the policy may create
// its account by signing in.
func (p Policy) SignUpAllowed() bool { return p.SignUp == nil || *p.SignUp }

type Quotas struct {
	Limit Size `yaml:"limit"`
}

// Flags holds the command-line values. An empty string or false means the
// flag was not set.
type Flags struct {
	BaseURL         string
	Listen          string
	DataDir         string
	ConfigDir       string
	MaxDocumentSize string
	TrustedProxies  string
	DevLogMail      bool
}

// Validate checks the whole config and reports every problem at once.
// Warnings do not stop the server.
func (c Config) Validate() (warnings []string, err error) {
	var errs []error
	if e := c.validateBaseURL(); e != nil {
		errs = append(errs, e)
	}
	if c.Listen == "" {
		errs = append(errs, errors.New("listen is required"))
	}
	if !c.MaxDocumentSize.IsSet() {
		errs = append(errs, errors.New("max_document_size must be set"))
	}
	errs = append(errs, c.SMTP.validate())

	warnings, policyErr := validatePolicies(c.UserPolicies)
	return warnings, errors.Join(append(errs, policyErr)...)
}

func (c Config) validateBaseURL() error {
	if c.BaseURL == "" {
		return errors.New("base_url is required")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("base_url must be an absolute URL, got %q", c.BaseURL)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopback(u.Hostname()) {
			return fmt.Errorf("base_url %q: http is only allowed on a loopback host", c.BaseURL)
		}
	default:
		return fmt.Errorf("base_url must start with https:// (or http:// on loopback), got %q", c.BaseURL)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func (s SMTP) validate() error {
	var errs []error
	if s.URL != "" {
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "smtp" && u.Scheme != "smtps") || u.Hostname() == "" {
			errs = append(errs, fmt.Errorf("smtp.url must be smtp://[user@]host[:port] or smtps://[user@]host[:port], got %q", s.URL))
		}
	}
	if s.Password != "" && !secrets.HasKnownPrefix(s.Password) {
		errs = append(errs, fmt.Errorf("smtp.password must be a secret reference (%s), not the password itself", strings.Join(secrets.KnownPrefixes, ", ")))
	}
	return errors.Join(errs...)
}

func validatePolicies(policies []Policy) (warnings []string, err error) {
	var errs []error
	names := map[string]bool{}
	catchAll := ""
	for i, p := range policies {
		label := fmt.Sprintf("user_policies[%d]", i)
		if p.Name != "" {
			label = fmt.Sprintf("user_policies[%d] (%s)", i, p.Name)
		}

		switch {
		case p.Name == "":
			errs = append(errs, fmt.Errorf("%s: name is required", label))
		case names[p.Name]:
			errs = append(errs, fmt.Errorf("%s: duplicate name", label))
		}
		names[p.Name] = true

		if len(p.Matches) == 0 {
			errs = append(errs, fmt.Errorf("%s: matches is empty", label))
		}
		for _, g := range p.Matches {
			if _, e := path.Match(strings.ToLower(g), ""); e != nil || strings.TrimSpace(g) == "" {
				errs = append(errs, fmt.Errorf("%s: bad glob %q", label, g))
			}
		}
		if !p.Quotas.Limit.IsSet() {
			errs = append(errs, fmt.Errorf("%s: quotas.limit is required", label))
		}

		if catchAll != "" {
			warnings = append(warnings, fmt.Sprintf("%s never matches: policy %q above it matches every email", label, catchAll))
		}
		if catchAll == "" && slices.ContainsFunc(p.Matches, func(m string) bool { return strings.TrimSpace(m) == "*" }) {
			catchAll = p.Name
		}
	}
	return warnings, errors.Join(errs...)
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
