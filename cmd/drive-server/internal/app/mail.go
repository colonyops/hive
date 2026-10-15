package app

import (
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/mail"
	"github.com/colonyops/hive/internal/platform/secrets"
)

var errNoMail = errors.New("no way to send sign-in mail: set smtp.url and smtp.from (or HIVE_DRIVE_SERVER_SMTP_URL and HIVE_DRIVE_SERVER_SMTP_FROM), or pass --dev-log-mail for development")

// The SMTP password resolves once here, so a bad reference fails at start.
func newSender(cfg config.Config, stdout io.Writer) (mail.Sender, error) {
	if cfg.DevLogMail {
		return &mail.Log{Out: stdout}, nil
	}
	if cfg.SMTP.URL == "" {
		return nil, errNoMail
	}
	password := ""
	if u, err := url.Parse(cfg.SMTP.URL); err == nil && u.User != nil && cfg.SMTP.Password != "" {
		password, err = secrets.Resolve(cfg.SMTP.Password)
		if err != nil {
			return nil, fmt.Errorf("resolve smtp.password: %w", err)
		}
	}
	s, err := mail.NewSMTP(cfg.SMTP.URL, func() (string, error) { return password, nil }, cfg.SMTP.From)
	if err != nil {
		return nil, fmt.Errorf("smtp: %w", err)
	}
	return s, nil
}
