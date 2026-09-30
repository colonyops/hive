package sourcehttp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadValidators(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	h.Set("ETag", `W/"abc"`)
	h.Set("Last-Modified", "Sat, 18 Jul 2026 08:00:00 GMT")

	v := ReadValidators(h)
	assert.Equal(t, `W/"abc"`, v.ETag)
	assert.Equal(t, "Sat, 18 Jul 2026 08:00:00 GMT", v.LastModified)
	assert.False(t, v.Empty())
	assert.True(t, ReadValidators(http.Header{}).Empty())
}

func TestValidatorsApply(t *testing.T) {
	t.Parallel()

	const modified = "Sat, 18 Jul 2026 08:00:00 GMT"
	tests := []struct {
		name                             string
		validators                       Validators
		wantNoneMatch, wantModifiedSince string
	}{
		{name: "empty sets nothing"},
		{
			name:          "etag only",
			validators:    Validators{ETag: `W/"abc"`},
			wantNoneMatch: `W/"abc"`,
		},
		{
			name:              "last-modified only",
			validators:        Validators{LastModified: modified},
			wantModifiedSince: modified,
		},
		{
			name:              "both are sent",
			validators:        Validators{ETag: `W/"abc"`, LastModified: modified},
			wantNoneMatch:     `W/"abc"`,
			wantModifiedSince: modified,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := http.Header{}
			tc.validators.Apply(h)
			assert.Equal(t, tc.wantNoneMatch, h.Get("If-None-Match"))
			assert.Equal(t, tc.wantModifiedSince, h.Get("If-Modified-Since"))
		})
	}
}
