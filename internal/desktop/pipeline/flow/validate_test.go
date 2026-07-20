package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_VersionMustBeOne(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 2
nodes: []
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestValidate_VersionMissing(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`nodes: []
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestValidate_Cycle(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - id: a
    type: function
    outputs: 1
    on_message: "return msg;"
  - id: b
    type: function
    outputs: 1
    on_message: "return msg;"
wires:
  - { from: a, to: b }
  - { from: b, to: a }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestValidate_OutOfRangeWirePort(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: search, query: "is:open" }
  - { id: sink, type: feed }
wires:
  - { from: src, out: 1, to: sink }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestValidate_WireIntoSource_IsError(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src1, type: github-source, kind: search, query: "is:open" }
  - { id: src2, type: github-source, kind: search, query: "is:open" }
wires:
  - { from: src1, to: src2 }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a source")
}

func TestValidate_WireOutOfTerminal_IsError(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: sink1, type: feed }
  - { id: sink2, type: feed }
wires:
  - { from: sink1, to: sink2 }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a terminal")
}

func TestValidate_DuplicateNodeID(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: dup, type: github-source, kind: search, query: "is:open" }
  - { id: dup, type: github-source, kind: search, query: "is:open" }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate node id")
}

func TestValidate_DuplicateWire(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: search, query: "is:open" }
  - { id: sink, type: feed }
wires:
  - { from: src, out: 0, to: sink }
  - { from: src, out: 0, to: sink }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate wire")
}

func TestValidate_EmptyGithubFilter(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: filt, type: github-filter }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github-filter")
}

func TestValidate_FunctionWithoutOnMessage(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: fn, type: function }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "on_message")
}

func TestValidate_BadSlug(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: "Not_A_Slug!", type: github-source, kind: search, query: "is:open" }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")
}

func TestValidate_BareIntegerDuration_IsHardError(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: fn, type: function, on_message: "return msg;", timeout: 5 }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bare number")
}

// --- github-source embedded config validation ---

func TestValidate_GithubSource_KindRequired(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind is required")
}

func TestValidate_GithubSource_SearchNeedsQuery(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: search }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a query")
}

func TestValidate_GithubSource_NotificationsRejectsQuery(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: notifications, query: "is:open" }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "takes no query")
}

func TestValidate_GithubSource_UnknownKind(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: webhook }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown kind")
}

func TestValidate_GithubSource_NotificationsOK(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: notifications }
`), nil)
	require.NoError(t, err)
}

func TestValidate_UnknownNodeType_IsError(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: rpc-source }
`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestValidate_UnresolvableActionRef(t *testing.T) {
	_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: act, type: action, action: no-such-action }
`), testRefs{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved reference")
}

func TestValidate_NilRefs_ActionResolvesToUnresolvedNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		_, _, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: act, type: action, action: a }
`), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unresolved reference")
	})
}

// --- soft warnings ---

func TestValidate_DisabledNode_IsWarningNotError(t *testing.T) {
	f, warnings, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: search, query: "is:open", disabled: true }
  - { id: sink, type: feed }
wires:
  - { from: src, to: sink }
`), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.Nodes)
	require.NotEmpty(t, warnings)
	found := false
	for _, w := range warnings {
		if w == `node "src" is disabled` {
			found = true
		}
	}
	assert.True(t, found, "warnings: %v", warnings)
}

func TestValidate_UntargetedTerminal_IsWarningNotError(t *testing.T) {
	_, warnings, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: search, query: "is:open" }
  - { id: sink, type: feed }
wires: []
`), nil)
	require.NoError(t, err)
	found := false
	for _, w := range warnings {
		if w == `terminal node "sink" (feed) has no inbound wire` {
			found = true
		}
	}
	assert.True(t, found, "warnings: %v", warnings)
}

func TestValidate_NoTerminal_IsWarningNotError(t *testing.T) {
	_, warnings, err := parseFlow("f", []byte(`version: 1
nodes:
  - { id: src, type: github-source, kind: search, query: "is:open" }
`), nil)
	require.NoError(t, err)
	found := false
	for _, w := range warnings {
		if w == "flow has no terminal node (feed or action)" {
			found = true
		}
	}
	assert.True(t, found, "warnings: %v", warnings)
}
