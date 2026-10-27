package mail

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestNewSMTP(t *testing.T) {
	s, err := NewSMTP("smtp://user@mail.example.com", "", "drive@example.com")
	require.NoError(t, err)
	assert.Equal(t, "mail.example.com:587", s.addr)
	assert.Equal(t, "user", s.user)
	assert.False(t, s.implicit)

	s, err = NewSMTP("smtps://mail.example.com", "", "drive@example.com")
	require.NoError(t, err)
	assert.Equal(t, "mail.example.com:465", s.addr)
	assert.True(t, s.implicit)

	for _, bad := range []string{"http://mail.example.com", "smtp://", "smtp://user@:25"} {
		_, err := NewSMTP(bad, "", "drive@example.com")
		require.Error(t, err, bad)
	}
	_, err = NewSMTP("smtp://mail.example.com", "", "")
	require.Error(t, err)
	_, err = NewSMTP("smtp://mail.example.com", "", "not an address")
	require.Error(t, err)

	s, err = NewSMTP("smtp://mail.example.com", "", "Hive Drive <drive@example.com>")
	require.NoError(t, err)
	assert.Equal(t, "drive@example.com", s.envelope)
	assert.Equal(t, `"Hive Drive" <drive@example.com>`, s.from)
}

func TestSMTPMessageStripsHeaderBreaks(t *testing.T) {
	s, err := NewSMTP("smtp://mail.example.com", "", "drive@example.com")
	require.NoError(t, err)
	msg := string(s.message(core.Mail{ID: "m1", To: "a@example.com", Subject: "x\r\nBcc: evil@example.com", Body: "line1\nline2", CreatedAt: epoch}))
	assert.Contains(t, msg, "Subject: xBcc: evil@example.com\r\n")
	assert.Contains(t, msg, "\r\n\r\nline1\r\nline2")
}

// plainSMTPServer accepts one session without STARTTLS or AUTH, like Mailpit,
// and sends the DATA section it received on the returned channel.
func plainSMTPServer(t *testing.T) (addr string, data <-chan string) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	out := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		reply := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		reply("220 test")
		var body strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case inData && line == ".":
				inData = false
				out <- body.String()
				reply("250 ok")
			case inData:
				body.WriteString(line + "\n")
			case strings.HasPrefix(line, "EHLO"):
				reply("250 test")
			case line == "DATA":
				inData = true
				reply("354 go")
			case line == "QUIT":
				reply("221 bye")
				return
			default:
				reply("250 ok")
			}
		}
	}()
	return ln.Addr().String(), out
}

func TestSMTPSendsPlainWithoutCredentialsOnLoopback(t *testing.T) {
	addr, data := plainSMTPServer(t)
	s, err := NewSMTP("smtp://"+addr, "", "drive@example.com")
	require.NoError(t, err)

	err = s.Send(t.Context(), core.Mail{ID: "m1", To: "a@example.com", Subject: "Hi", Body: "code 12345678", CreatedAt: epoch})
	require.NoError(t, err)
	assert.Contains(t, <-data, "code 12345678")
}

func TestLogSender(t *testing.T) {
	var buf bytes.Buffer
	err := (&Log{Out: &buf}).Send(t.Context(), core.Mail{To: "a@example.com", Subject: "Hi", Body: "code 12345678"})
	require.NoError(t, err)
	assert.Equal(t, "mail to a@example.com: Hi\ncode 12345678\n", buf.String())
}

type fakeSender struct {
	fail error
	sent []string
}

func (f *fakeSender) Send(_ context.Context, m core.Mail) error {
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, m.ID)
	return nil
}

func newMailer(t *testing.T, sender Sender, now *time.Time) (*Mailer, core.OutboxStore) {
	t.Helper()
	store, err := control.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return NewMailer(zerolog.Nop(), store.Outbox(), sender, func() time.Time { return *now }), store.Outbox()
}

func queue(t *testing.T, o core.OutboxStore, id string, at time.Time) {
	t.Helper()
	require.NoError(t, o.Append(t.Context(), core.Mail{ID: id, To: "a@example.com", CreatedAt: at, NextAt: at}))
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

func TestRetryDelay(t *testing.T) {
	assert.Equal(t, 30*time.Second, retryDelay(0))
	assert.Equal(t, 10*time.Minute, retryDelay(2))
	assert.Equal(t, time.Hour, retryDelay(3))
}
