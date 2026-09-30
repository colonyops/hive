package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

type webhookStatus struct {
	Running    bool   `json:"running"    jsonschema:"Whether the loopback webhook listener is accepting deliveries."`
	Host       string `json:"host"       jsonschema:"Host the listener is bound to."`
	Port       int    `json:"port"       jsonschema:"Port the listener is bound to; 0 when it is not running."`
	PathPrefix string `json:"pathPrefix" jsonschema:"Path prefix webhook deliveries are posted under."`
}

type statusResult struct {
	Version string        `json:"version" jsonschema:"The running build's version."`
	Webhook webhookStatus `json:"webhook"`
}

func (ctrl *Controller) GetStatus(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, statusResult, error) {
	running, port := ctrl.core.Webhooks.Endpoint(ctx)
	return nil, statusResult{
		Version: ctrl.opts.Version,
		Webhook: webhookStatus{
			Running:    running,
			Host:       ctrl.core.Webhooks.Host(),
			Port:       port,
			PathPrefix: webhook.PathPrefix,
		},
	}, nil
}
