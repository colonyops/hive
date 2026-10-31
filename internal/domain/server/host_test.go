package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRejectsUnsafeHosts(t *testing.T) {
	bad := []Host{
		{Name: "-oProxyCommand=x"},
		{Name: "a b"},
		{Name: ""},
		{Name: "box", Binary: "hive;rm -rf ~"},
		{Name: "box", Binary: "$(id)"},
		{Name: "box", Tmux: "tmux;id"},
		{Name: "box", ConfigFile: "a\nb"},
	}
	for _, h := range bad {
		require.Error(t, h.Validate(), "%+v", h)
	}
	assert.NoError(t, Host{Name: "ada@build-1.lan", Binary: "~/go/bin/hive"}.Validate())
}
