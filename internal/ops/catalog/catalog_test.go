package catalog_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
)

// TestEverySpecIsListed reads each toolset package's source for the vars of
// type ops.Spec[...], so a spec that is declared but missing from its
// package's All() fails here instead of being served nowhere.
func TestEverySpecIsListed(t *testing.T) {
	listed := map[string]bool{}
	for _, op := range catalog.All() {
		listed[op.InputType().PkgPath()+"."+op.Info().Name] = true
	}

	declared := map[string]string{}
	entries, err := os.ReadDir("..")
	require.NoError(t, err)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "catalog" || entry.Name() == "mcpops" {
			continue
		}
		for name, spec := range specVars(t, filepath.Join("..", entry.Name())) {
			declared[entry.Name()+"."+name] = spec
		}
	}
	require.NotEmpty(t, declared)

	names := map[string]bool{}
	for _, op := range catalog.All() {
		names[op.Info().Name] = true
	}
	for v, name := range declared {
		assert.True(t, names[name], "%s (%s) is not in catalog.All()", v, name)
	}
	assert.Len(t, catalog.All(), len(declared), "catalog.All() lists a spec more than once, or one this test cannot see")
}

func specVars(t *testing.T, dir string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]string{}
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, value := range spec.Values {
				lit, ok := value.(*ast.CompositeLit)
				if !ok || !isSpecType(lit.Type) {
					continue
				}
				out[spec.Names[i].Name] = specName(t, lit)
			}
			return true
		})
	}
	return out
}

func isSpecType(e ast.Expr) bool {
	idx, ok := e.(*ast.IndexExpr)
	if !ok {
		return false
	}
	sel, ok := idx.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "ops" && sel.Sel.Name == "Spec"
}

func specName(t *testing.T, lit *ast.CompositeLit) string {
	t.Helper()
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Name" {
			if s, ok := kv.Value.(*ast.BasicLit); ok {
				return s.Value[1 : len(s.Value)-1]
			}
		}
	}
	t.Fatalf("a spec literal has no string Name")
	return ""
}

var toolName = regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)

func TestCatalogShape(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range catalog.All() {
		info := op.Info()
		t.Run(info.Name, func(t *testing.T) {
			assert.False(t, seen[info.ToolName], "tool name %s is not unique", info.ToolName)
			seen[info.ToolName] = true
			assert.Regexp(t, toolName, info.ToolName)
			assert.Contains(t, ops.Verbs, info.Verb)
			assert.NotEmpty(t, info.Summary)
			assert.NotEqual(t, '.', rune(info.Summary[len(info.Summary)-1]), "a summary has no final period")
			assert.NotEmpty(t, info.Toolset.Name)
			assert.NotEmpty(t, info.Toolset.MCPPath)
			if info.Effect != ops.EffectRead {
				assert.NotEmpty(t, info.Guidance, "a write or exec op says what it changes")
			}

			_, err := op.InputSchema()
			require.NoError(t, err)

			fields, err := ops.Fields(op)
			require.NoError(t, err)
			for _, arg := range info.Args {
				assert.True(t, slices.ContainsFunc(fields, func(f ops.Field) bool { return f.Name == arg }),
					"arg %q names no input field", arg)
			}
		})
	}
}
