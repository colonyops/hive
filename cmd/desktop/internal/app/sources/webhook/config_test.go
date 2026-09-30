package webhook

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{name: "simple path", config: Config{Path: "ci-alerts"}},
		{name: "nested path", config: Config{Path: "ci/deploys/prod_1"}},
		{name: "with secret", config: Config{Path: "ci", Secret: "s3cret-token_9"}},
		{name: "with icon", config: Config{Path: "ci", Icon: "bell"}},
		{name: "unknown icon", config: Config{Path: "ci", Icon: "no-such-icon"}, wantErr: "not a supported feed icon"},
		{name: "with image", config: Config{Path: "ci", Image: "0123456789abcdef0123456789abcdef"}},
		{name: "uppercase image", config: Config{Path: "ci", Image: "0123456789ABCDEF0123456789abcdef"}, wantErr: "not a valid mark reference"},
		{name: "short image", config: Config{Path: "ci", Image: "0123"}, wantErr: "not a valid mark reference"},
		{name: "missing path", config: Config{}, wantErr: "path is required"},
		{name: "blank path", config: Config{Path: "   "}, wantErr: "path is required"},
		{name: "uppercase", config: Config{Path: "CI"}, wantErr: "invalid path"},
		{name: "leading slash", config: Config{Path: "/ci"}, wantErr: "invalid path"},
		{name: "trailing slash", config: Config{Path: "ci/"}, wantErr: "invalid path"},
		{name: "empty segment", config: Config{Path: "ci//x"}, wantErr: "invalid path"},
		{name: "leading dash segment", config: Config{Path: "-ci"}, wantErr: "invalid path"},
		{name: "spaces", config: Config{Path: "ci alerts"}, wantErr: "invalid path"},
		{name: "path too long", config: Config{Path: strings.Repeat("a", 129)}, wantErr: "exceeds 128"},
		{name: "secret too long", config: Config{Path: "ci", Secret: strings.Repeat("s", 129)}, wantErr: "secret exceeds 128"},
		{name: "secret with space", config: Config{Path: "ci", Secret: "no spaces"}, wantErr: "printable ASCII"},
		{name: "secret with control char", config: Config{Path: "ci", Secret: "a\nb"}, wantErr: "printable ASCII"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.config.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// The descriptor is what the flow package derives a node type from and what
// the producer reads capabilities off. Webhook deliveries arrive on their own
// schedule, so it is push and carries no Pull; and there is no absence to
// confirm, because a sender that stops sending says nothing about the item.
func TestDescriptorDeclaresPushWithClassificationOnly(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "sources.webhook", Descriptor.Type)
	assert.Equal(t, connector.ModePush, Descriptor.Mode)
	assert.True(t, Descriptor.Capabilities.Has(connector.CapClassify))
	assert.False(t, Descriptor.Capabilities.Has(connector.CapConfirmAbsence))
	assert.False(t, Descriptor.Capabilities.Has(connector.CapBatchPrefetch))
}
