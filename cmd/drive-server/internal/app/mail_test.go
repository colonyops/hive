package app

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/mail"
)

func TestNewSenderRefusesToStartWithoutMail(t *testing.T) {
	_, err := newSender(config.Config{}, &bytes.Buffer{})
	assert.ErrorIs(t, err, errNoMail)
}

func TestNewSenderChoosesLogOrSMTP(t *testing.T) {
	s, err := newSender(config.Config{DevLogMail: true}, &bytes.Buffer{})
	require.NoError(t, err)
	assert.IsType(t, &mail.Log{}, s)

	s, err = newSender(config.Config{SMTP: config.SMTP{URL: "smtp://127.0.0.1:1025", From: "drive@example.com"}}, &bytes.Buffer{})
	require.NoError(t, err)
	assert.IsType(t, &mail.SMTP{}, s)

	_, err = newSender(config.Config{SMTP: config.SMTP{URL: "smtp://127.0.0.1:1025"}}, &bytes.Buffer{})
	assert.Error(t, err, "smtp.from is required")
}
