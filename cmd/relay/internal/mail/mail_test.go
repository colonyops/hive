package mail

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/memory"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestLogSender(t *testing.T) {
	var buf bytes.Buffer
	err := (&Log{Out: &buf}).Send(t.Context(), relay.Mail{To: "a@example.com", Subject: "Hi", Body: "code 12345678"})
	require.NoError(t, err)
	assert.Equal(t, "mail to a@example.com: Hi\ncode 12345678\n", buf.String())
}

type fakeSender struct {
	fail error
	sent []string
}

func (f *fakeSender) Send(_ context.Context, m relay.Mail) error {
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, m.ID)
	return nil
}

func newMailer(t *testing.T, sender Sender, now *time.Time) (*Mailer, relay.OutboxStore) {
	t.Helper()
	store := memory.New()
	m := NewMailer(store.Outbox(), sender, zerolog.Nop())
	m.now = func() time.Time { return *now }
	return m, store.Outbox()
}

func queue(t *testing.T, o relay.OutboxStore, id string, at time.Time) {
	t.Helper()
	require.NoError(t, o.Append(t.Context(), relay.Mail{ID: id, To: "a@example.com", CreatedAt: at, NextAt: at}))
}

func TestMailerDrainsAndMarksSent(t *testing.T) {
	now := epoch
	sender := &fakeSender{}
	m, o := newMailer(t, sender, &now)
	queue(t, o, "m1", epoch)
	queue(t, o, "m2", epoch.Add(time.Second))

	now = epoch.Add(time.Minute)
	require.NoError(t, m.Drain(t.Context()))
	assert.Equal(t, []string{"m1", "m2"}, sender.sent)

	require.NoError(t, m.Drain(t.Context()))
	assert.Len(t, sender.sent, 2, "sent mail is not sent again")
}

func TestMailerRetriesWithBackoff(t *testing.T) {
	now := epoch
	sender := &fakeSender{fail: errors.New("connection refused")}
	m, o := newMailer(t, sender, &now)
	queue(t, o, "m1", epoch)

	require.NoError(t, m.Drain(t.Context()))
	due, err := o.Due(t.Context(), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, 1, due[0].Attempts)
	assert.True(t, epoch.Add(30*time.Second).Equal(due[0].NextAt), due[0].NextAt)

	sender.fail = nil
	now = epoch.Add(29 * time.Second)
	require.NoError(t, m.Drain(t.Context()))
	assert.Empty(t, sender.sent, "a deferred mail waits for NextAt")

	now = epoch.Add(30 * time.Second)
	require.NoError(t, m.Drain(t.Context()))
	assert.Equal(t, []string{"m1"}, sender.sent)
}

func TestBackoff(t *testing.T) {
	assert.Equal(t, 30*time.Second, Backoff(0))
	assert.Equal(t, 2*time.Minute, Backoff(1))
	assert.Equal(t, 10*time.Minute, Backoff(2))
	assert.Equal(t, time.Hour, Backoff(3))
	assert.Equal(t, time.Hour, Backoff(40))
}

func TestNewSMTP(t *testing.T) {
	pw := func() (string, error) { return "", nil }
	s, err := NewSMTP("smtp://user@mail.example.com", pw, "relay@example.com")
	require.NoError(t, err)
	assert.Equal(t, "mail.example.com:587", s.addr)
	assert.Equal(t, "user", s.user)
	assert.False(t, s.implicit)

	s, err = NewSMTP("smtps://mail.example.com", pw, "relay@example.com")
	require.NoError(t, err)
	assert.Equal(t, "mail.example.com:465", s.addr)
	assert.True(t, s.implicit)

	for _, bad := range []string{"http://mail.example.com", "smtp://", "smtp://user@:25"} {
		_, err := NewSMTP(bad, pw, "relay@example.com")
		require.Error(t, err, bad)
	}
	_, err = NewSMTP("smtp://mail.example.com", pw, "")
	assert.Error(t, err)
}

func TestSMTPMessageStripsHeaderBreaks(t *testing.T) {
	s, err := NewSMTP("smtp://mail.example.com", nil, "relay@example.com")
	require.NoError(t, err)
	msg := string(s.message(relay.Mail{ID: "m1", To: "a@example.com", Subject: "x\r\nBcc: evil@example.com", Body: "line1\nline2", CreatedAt: epoch}))
	assert.Contains(t, msg, "Subject: xBcc: evil@example.com\r\n")
	assert.Contains(t, msg, "\r\n\r\nline1\r\nline2")
}
