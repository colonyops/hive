// Package mcpsrv is the relay's MCP server for agents: the /v1 API as tools,
// over the same services and the same bearer tokens. httpapi mounts Handler
// at /mcp behind its bearer check, so every tool reads the caller's account
// from the request context.
package mcpsrv

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

const serverName = "hive-relay"

type Core struct {
	Auth     *relay.Auth
	SignIn   *relay.SignIn
	Sources  *relay.Sources
	Feed     *relay.Feed
	Accounts *relay.Accounts
	Store    relay.Store
	Limits   relay.Limits
	Version  string
}

type Controller struct {
	core   Core
	log    zerolog.Logger
	server *mcp.Server
}

func New(core Core, log zerolog.Logger) *Controller {
	version := core.Version
	if version == "" {
		version = "dev"
	}
	c := &Controller{core: core, log: log}
	c.server = mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Title:   "Hive Relay",
		Version: version,
		Description: "Manage a Hive Relay account: its webhook sources and ingress URLs, " +
			"the deliveries each source has stored, the account's API keys, and its encryption recipients.",
	}, nil)
	c.register(c.server)
	return c
}

// Handler serves the tools over stateless Streamable HTTP with plain JSON
// responses, the shape the desktop's own MCP server uses.
//
// Localhost protection is off: a relay usually sits behind a reverse proxy
// on loopback, so every request would arrive from 127.0.0.1 with the public
// Host and be refused. DNS rebinding needs a credential the browser sends on
// its own, and this endpoint takes only a bearer header.
func (c *Controller) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return c.server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true},
	)
}
