package httpapi_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/httpapi"
)

type routeAuth int

const (
	public routeAuth = iota
	readScope
	pushScope
	sessionOnly
)

// apiRoutes lists every /api/v1 route and the credential it needs.
var apiRoutes = map[string]routeAuth{
	"GET /api/v1/me":           readScope,
	"GET /api/v1/spaces":       readScope,
	"GET /api/v1/search":       readScope,
	"GET /api/v1/recent":       readScope,
	"GET /api/v1/shared-links": readScope,

	"GET /api/v1/shared":                     readScope,
	"GET /api/v1/shared/docs/{path...}":      readScope,
	"PUT /api/v1/shared/docs/{path...}":      pushScope,
	"DELETE /api/v1/shared/docs/{path...}":   pushScope,
	"POST /api/v1/shared/shares/{path...}":   pushScope,
	"DELETE /api/v1/shared/shares/{path...}": pushScope,

	"GET /api/v1/workspaces/{name}":                     readScope,
	"GET /api/v1/workspaces/{name}/docs/{path...}":      readScope,
	"PUT /api/v1/workspaces/{name}/docs/{path...}":      pushScope,
	"DELETE /api/v1/workspaces/{name}/docs/{path...}":   pushScope,
	"POST /api/v1/workspaces/{name}/shares/{path...}":   pushScope,
	"DELETE /api/v1/workspaces/{name}/shares/{path...}": pushScope,

	"GET /api/v1/repos/{owner}/{repo}":                     readScope,
	"GET /api/v1/repos/{owner}/{repo}/docs/{path...}":      readScope,
	"PUT /api/v1/repos/{owner}/{repo}/docs/{path...}":      pushScope,
	"DELETE /api/v1/repos/{owner}/{repo}/docs/{path...}":   pushScope,
	"POST /api/v1/repos/{owner}/{repo}/shares/{path...}":   pushScope,
	"DELETE /api/v1/repos/{owner}/{repo}/shares/{path...}": pushScope,

	"GET /api/v1/public/{slug}": public,

	"POST /api/v1/auth/code":     public,
	"POST /api/v1/auth/verify":   public,
	"POST /api/v1/auth/sign-out": sessionOnly,

	"POST /api/v1/tokens":        sessionOnly,
	"GET /api/v1/tokens":         sessionOnly,
	"DELETE /api/v1/tokens/{id}": sessionOnly,
}

var wildcards = strings.NewReplacer("{path...}", "a.md", "{name}", "ws", "{owner}", "o", "{repo}", "r", "{slug}", "x", "{id}", "x")

func TestRouteTableListsEveryAPIRoute(t *testing.T) {
	var registered []string
	for _, p := range httpapi.RoutePatterns(httpapi.Services{}) {
		if _, path, _ := strings.Cut(p, " "); strings.HasPrefix(path, "/api/v1/") {
			registered = append(registered, p)
		}
	}
	var listed []string
	for p := range apiRoutes {
		listed = append(listed, p)
	}
	slices.Sort(registered)
	slices.Sort(listed)
	assert.Equal(t, registered, listed, "routes.go and apiRoutes disagree")
}

func TestRouteAuth(t *testing.T) {
	s := newServer(t)
	read := s.token(t, "dev@example.com", core.ScopeRead)

	for pattern, auth := range apiRoutes {
		if auth == public {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			method, path, ok := strings.Cut(pattern, " ")
			require.True(t, ok)
			c := call{method: method, target: wildcards.Replace(path)}

			rec := s.send(c)
			assert.Equal(t, http.StatusUnauthorized, rec.Code, "no credentials")
			assert.Equal(t, "unauthenticated", errorKind(t, rec))

			if auth == pushScope || auth == sessionOnly {
				c.token = read
				rec := s.send(c)
				assert.Equal(t, http.StatusForbidden, rec.Code, "read-scope token")
				assert.Equal(t, "forbidden", errorKind(t, rec))
			}
		})
	}
}
