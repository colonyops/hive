package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"filippo.io/age"
)

func parseRecipient(s string) (*age.X25519Recipient, error) {
	r, err := age.ParseX25519Recipient(s)
	if err != nil {
		return nil, fmt.Errorf("parse recipient: %w", err)
	}
	return r, nil
}

// seal encrypts record to every recipient in age's binary format; any one of
// their identities opens it.
func seal(record []byte, recipients []string) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, errors.New("seal: no recipients")
	}
	rs := make([]age.Recipient, 0, len(recipients))
	for _, s := range recipients {
		r, err := parseRecipient(s)
		if err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, rs...)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	if _, err := io.Copy(w, bytes.NewReader(record)); err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	return buf.Bytes(), nil
}
