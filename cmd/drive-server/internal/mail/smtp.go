package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/netip"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

const smtpTimeout = 30 * time.Second

// SMTP sends through one server. smtp:// upgrades with STARTTLS and refuses a
// server that does not offer it, except on loopback, where a local catcher
// such as Mailpit speaks plain SMTP. smtps:// is TLS from the first byte.
// Without a user in the URL, the client does not authenticate.
type SMTP struct {
	addr     string
	host     string
	implicit bool
	user     string
	password func() (string, error)
	from     string
	// envelope is the bare address from `from`, which may carry a display
	// name: MAIL FROM accepts only the address.
	envelope string
}

// NewSMTP parses rawURL (smtp://user@host:587 or smtps://user@host:465; the
// port defaults by scheme). password runs on every send and is only called
// when the URL names a user.
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
		return nil, errors.New("smtp.from is required with smtp.url")
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("smtp.from %q: %w", from, err)
	}
	s.from = addr.String()
	s.envelope = addr.Address
	if u.User != nil {
		s.user = u.User.Username()
	}
	s.addr = net.JoinHostPort(s.host, port)
	return s, nil
}

func (s *SMTP) Send(ctx context.Context, m core.Mail) error {
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
	if err := c.Mail(s.envelope); err != nil {
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
	var (
		conn net.Conn
		err  error
	)
	if s.implicit {
		d := tls.Dialer{Config: &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}}
		conn, err = d.DialContext(ctx, "tcp", s.addr)
	} else {
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", s.addr)
	}
	if err != nil {
		return nil, fmt.Errorf("smtp dial %s: %w", s.addr, err)
	}
	return conn, nil
}

func (s *SMTP) message(m core.Mail) []byte {
	var b strings.Builder
	header := func(k, v string) {
		b.WriteString(k + ": " + stripCRLF(v) + "\r\n")
	}
	header("From", s.from)
	header("To", m.To)
	header("Subject", m.Subject)
	header("Date", m.CreatedAt.Format(time.RFC1123Z))
	header("Message-ID", "<"+m.ID+"@hive-drive-server>")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}

// stripCRLF keeps a subject or address from injecting headers.
func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}
