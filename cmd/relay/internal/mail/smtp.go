package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

const smtpTimeout = 30 * time.Second

// SMTP sends through one server. smtp:// upgrades with STARTTLS and refuses a
// server that does not offer it, except on loopback, where a local catcher
// such as mailpit speaks plain SMTP. smtps:// is TLS from the first byte.
type SMTP struct {
	addr     string
	host     string
	implicit bool
	user     string
	password func() (string, error)
	from     string
}

// NewSMTP parses rawURL (smtp://user@host:587 or smtps://user@host:465; the
// port defaults by scheme). password runs on every send, so a rotated secret
// takes effect without a restart.
func NewSMTP(rawURL string, password func() (string, error), from string) (*SMTP, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("smtp url: %w", err)
	}
	s := &SMTP{host: u.Hostname(), password: password, from: from}
	port := u.Port()
	switch u.Scheme {
	case "smtp":
		if port == "" {
			port = "587"
		}
	case "smtps":
		s.implicit = true
		if port == "" {
			port = "465"
		}
	default:
		return nil, fmt.Errorf("smtp url must start with smtp:// or smtps://, got %q", rawURL)
	}
	if s.host == "" {
		return nil, fmt.Errorf("smtp url %q has no host", rawURL)
	}
	if from == "" {
		return nil, errors.New("a sender address (RELAY_MAIL_FROM) is required for smtp")
	}
	if u.User != nil {
		s.user = u.User.Username()
	}
	s.addr = net.JoinHostPort(s.host, port)
	return s, nil
}

func (s *SMTP) Send(ctx context.Context, m relay.Mail) error {
	ctx, cancel := context.WithTimeout(ctx, smtpTimeout)
	defer cancel()

	conn, err := s.dial(ctx)
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp hello: %w", err)
	}
	defer func() { _ = c.Close() }()

	if !s.implicit {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		} else if !isLoopback(s.host) {
			return fmt.Errorf("smtp server %s does not offer STARTTLS", s.addr)
		}
	}
	if s.user != "" {
		pw, err := s.password()
		if err != nil {
			return fmt.Errorf("resolve smtp password: %w", err)
		}
		if err := c.Auth(smtp.PlainAuth("", s.user, pw, s.host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(m.To); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(s.message(m)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp end data: %w", err)
	}
	return c.Quit()
}

func (s *SMTP) dial(ctx context.Context) (net.Conn, error) {
	if s.implicit {
		d := tls.Dialer{Config: &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}}
		conn, err := d.DialContext(ctx, "tcp", s.addr)
		if err != nil {
			return nil, fmt.Errorf("smtp dial %s: %w", s.addr, err)
		}
		return conn, nil
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial %s: %w", s.addr, err)
	}
	return conn, nil
}

func (s *SMTP) message(m relay.Mail) []byte {
	var b strings.Builder
	header := func(k, v string) {
		b.WriteString(k + ": " + stripCRLF(v) + "\r\n")
	}
	header("From", s.from)
	header("To", m.To)
	header("Subject", m.Subject)
	header("Date", m.CreatedAt.Format(time.RFC1123Z))
	header("Message-ID", "<"+m.ID+"@hive-relay>")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}

func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
