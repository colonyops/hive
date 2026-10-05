package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/platform/observe"
)

var tracer = observe.Tracer("/internal/adapter/mcpsrv")

const attrToolName = "mcp.tool.name"

// addTool registers a tool whose every call roots a trigger span
// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
// The span is named from the tool table rather than from the request, so a
// client naming a tool that does not exist cannot mint a span name: the SDK
// refuses that call before any handler runs.
func addTool[In, Out any](srv *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	spanName := "mcp.tool " + tool.Name
	mcp.AddTool(srv, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, out Out, err error) {
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String(attrToolName, tool.Name)),
		)
		defer observe.End(span, &err)
		return handler(ctx, req, in)
	})
}
