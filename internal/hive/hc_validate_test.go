package hive

import (
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/hay-kot/criterio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs, "want criterio.FieldErrors, got %T: %v", err, err)
	got := make(map[string]string, len(fieldErrs))
	for _, fe := range fieldErrs {
		got[fe.Field] = fe.Err.Error()
	}
	return got
}

func TestValidateCreateInput_Valid(t *testing.T) {
	input := hc.CreateInput{Title: "Epic", Type: hc.ItemTypeEpic, Children: []hc.CreateInput{
		{Ref: "jwt", Title: "JWT", Type: hc.ItemTypeTask},
		{Ref: "db", Title: "DB", Type: hc.ItemTypeTask},
		{Title: "Login", Type: hc.ItemTypeTask, Blockers: []string{"jwt", "db"}, Children: []hc.CreateInput{
			{Title: "Form", Type: hc.ItemTypeTask, Blockers: []string{"jwt"}},
		}},
	}}
	require.NoError(t, ValidateCreateInput(input))
	assert.Equal(t, 5, input.Count())
}

func TestValidateCreateInput_RootMustBeEpic(t *testing.T) {
	got := fieldsOf(t, ValidateCreateInput(hc.CreateInput{Title: "T", Type: hc.ItemTypeTask}))
	assert.Contains(t, got["type"], "root item must be of type epic")
}

func TestValidateCreateInput_DuplicateRef(t *testing.T) {
	input := hc.CreateInput{Title: "Epic", Type: hc.ItemTypeEpic, Children: []hc.CreateInput{
		{Ref: "a", Title: "x", Type: hc.ItemTypeTask},
		{Ref: "a", Title: "y", Type: hc.ItemTypeTask},
	}}
	got := fieldsOf(t, ValidateCreateInput(input))
	assert.Contains(t, got["children[1].ref"], `duplicate ref "a"`)
	assert.Contains(t, got["children[1].ref"], "children[0]")
}

func TestValidateCreateInput_UnknownBlocker(t *testing.T) {
	input := hc.CreateInput{Title: "Epic", Type: hc.ItemTypeEpic, Children: []hc.CreateInput{
		{Ref: "jwt", Title: "x", Type: hc.ItemTypeTask},
		{Title: "y", Type: hc.ItemTypeTask, Blockers: []string{"jwt", "nope"}},
	}}
	got := fieldsOf(t, ValidateCreateInput(input))
	assert.Contains(t, got["children[1].blockers[1]"], `unknown blocker ref "nope"`)
	_, first := got["children[1].blockers[0]"]
	assert.False(t, first, "a resolving blocker must not be reported")
}

func TestValidateCreateInput_Cycle(t *testing.T) {
	input := hc.CreateInput{Title: "Epic", Type: hc.ItemTypeEpic, Children: []hc.CreateInput{
		{Ref: "a", Title: "x", Type: hc.ItemTypeTask, Blockers: []string{"c"}},
		{Ref: "b", Title: "y", Type: hc.ItemTypeTask, Blockers: []string{"a"}},
		{Ref: "c", Title: "z", Type: hc.ItemTypeTask, Blockers: []string{"b"}},
	}}
	got := fieldsOf(t, ValidateCreateInput(input))
	assert.Len(t, got, 1, "exactly one edge closes the cycle: %v", got)
	assert.Contains(t, got["children[2].blockers[0]"], "cycle")
}

func TestValidateCreateInput_SelfBlock(t *testing.T) {
	input := hc.CreateInput{Title: "Epic", Type: hc.ItemTypeEpic, Children: []hc.CreateInput{
		{Ref: "a", Title: "x", Type: hc.ItemTypeTask, Blockers: []string{"a"}},
	}}
	got := fieldsOf(t, ValidateCreateInput(input))
	assert.Contains(t, got["children[0].blockers[0]"], "cycle")
}
