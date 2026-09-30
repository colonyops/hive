package ingest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceSpanNameOmitsAnAbsentKind(t *testing.T) {
	assert.Equal(t, "ingest.source github", sourceSpanName("github"))
	assert.Equal(t, "ingest.source", sourceSpanName(""))
}
