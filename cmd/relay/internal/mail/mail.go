// Package mail sends the relay's outbox: a Mailer drains queued mail through
// a Sender, either SMTP or a Log sender for development.
package mail

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

type Sender interface {
	Send(ctx context.Context, m relay.Mail) error
}

// Log writes each mail to Out instead of sending it, so a dev relay prints
// sign-in codes where the operator can read them.
type Log struct {
	Out io.Writer
	mu  sync.Mutex
}

func (l *Log) Send(_ context.Context, m relay.Mail) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := fmt.Fprintf(l.Out, "mail to %s: %s\n%s\n", m.To, m.Subject, m.Body)
	return err
}
