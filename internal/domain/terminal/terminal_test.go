package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusSimplified(t *testing.T) {
	tests := []struct {
		in   Status
		want Status
	}{
		{StatusActive, StatusActive},
		{StatusApproval, StatusApproval},
		{StatusQuestion, StatusApproval},
		{StatusReady, StatusReady},
		{StatusMissing, StatusMissing},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.Simplified())
		})
	}
}
