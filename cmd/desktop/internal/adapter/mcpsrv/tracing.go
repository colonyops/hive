package mcpsrv

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/platform/observe"
)

var (
	tracer = observe.Tracer("/internal/adapter/mcpsrv")
	meter  = observe.Meter("/internal/adapter/mcpsrv")

	// The OpenTelemetry MCP semantic conventions' name and attributes, so a
	// dashboard built for any MCP server reads it. The tool name is bounded
	// by the tool table, which is what makes it safe as a label.
	toolDuration = observe.Must(meter.Float64Histogram(
		"mcp.server.operation.duration",
		metric.WithDescription("Duration of one MCP tool call, by tool and outcome."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
		),
	))
)

const (
	attrToolName = "mcp.tool.name"

	semconvMethodName = "mcp.method.name"
	semconvToolName   = "gen_ai.tool.name"
	semconvErrorType  = "error.type"
	methodToolsCall   = "tools/call"
	errorTypeTool     = "tool_error"
)

// addTool registers a tool whose every call roots a trigger span
// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
// The span is named from the tool table rather than from the request, so a
// client naming a tool that does not exist cannot mint a span name: the SDK
// refuses that call before any handler runs.
func addTool[In, Out any](srv *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	spanName := "mcp.tool " + tool.Name
	succeeded := metric.WithAttributes(
		attribute.String(semconvMethodName, methodToolsCall),
		attribute.String(semconvToolName, tool.Name),
	)
	failed := metric.WithAttributes(
		attribute.String(semconvMethodName, methodToolsCall),
		attribute.String(semconvToolName, tool.Name),
		attribute.String(semconvErrorType, errorTypeTool),
	)
	mcp.AddTool(srv, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, out Out, err error) {
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String(attrToolName, tool.Name)),
		)
		start := time.Now()
		defer func() {
			outcome := succeeded
			if err != nil {
				outcome = failed
			}
			toolDuration.Record(ctx, time.Since(start).Seconds(), outcome)
		}()
		defer observe.End(span, &err)
		return handler(ctx, req, in)
	})
}
