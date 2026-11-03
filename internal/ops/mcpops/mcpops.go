// Package mcpops mounts catalog operations on an MCP server. The tool's name,
// title, description, input schema and read-only hint all come from the Spec;
// the binder supplies only the handler and its output type.
package mcpops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/platform/observe"
)

// Mount registers s on srv with handler h. A handler error that is an
// *ops.Error crosses as "<kind>: <message>"; any other error goes through
// mapErr, the binder's own error mapping. It panics on a schema error, as
// mcp.AddTool does.
func Mount[In, Out any](srv *mcp.Server, s ops.Spec[In], h func(context.Context, In) (Out, error), mapErr func(error) error) {
	MountResult(srv, s, func(ctx context.Context, in In) (*mcp.CallToolResult, Out, error) {
		out, err := h(ctx, in)
		return nil, out, err
	}, mapErr)
}

// MountResult is Mount for a handler that may answer with its own content,
// such as an image, instead of a structured value.
//
// It registers a raw handler rather than going through mcp.AddTool, because
// the SDK reports a schema violation as text with no kind in front of it, and
// a caller reads the kind to decide what went wrong. Arguments are validated
// against the same schema the tool advertises, and a violation is invalid.
func MountResult[In, Out any](srv *mcp.Server, s ops.Spec[In], h func(context.Context, In) (*mcp.CallToolResult, Out, error), mapErr func(error) error) {
	tool, err := Tool(s)
	if err != nil {
		panic(err)
	}
	input, ok := tool.InputSchema.(*jsonschema.Schema)
	if !ok {
		panic(fmt.Sprintf("%s: input schema is %T", s.Name, tool.InputSchema))
	}
	resolved, err := input.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		panic(fmt.Sprintf("%s: input schema: %v", s.Name, err))
	}
	if reflect.TypeFor[Out]() != reflect.TypeFor[any]() {
		output, err := jsonschema.ForType(reflect.TypeFor[Out](), &jsonschema.ForOptions{})
		if err != nil {
			panic(fmt.Sprintf("%s: output schema: %v", s.Name, err))
		}
		tool.OutputSchema = output
	}

	fields, err := ops.Fields(s)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", s.Name, err))
	}

	call := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, any, error) {
		in, err := decode[In](req.Params.Arguments, fields, resolved)
		if err != nil {
			return nil, nil, err
		}
		if v, ok := any(in).(ops.Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, nil, ops.Errorf(ops.KindInvalid, "%s", err.Error())
			}
		}
		res, out, err := h(ctx, in)
		if err != nil {
			if opErr, ok := errors.AsType[*ops.Error](err); ok {
				return nil, nil, opErr
			}
			return nil, nil, mapErr(err)
		}
		return res, out, nil
	}
	srv.AddTool(tool, instrumentRaw(tool.Name, call))
}

// decode checks each constraint before the whole schema, because the
// constraint's message is the one the CLI gives for the same mistake.
func decode[In any](args json.RawMessage, fields []ops.Field, schema *jsonschema.Resolved) (In, error) {
	var in In
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	var value map[string]any
	if err := json.Unmarshal(args, &value); err != nil {
		return in, ops.Errorf(ops.KindInvalid, "arguments are not a JSON object: %v", err)
	}
	for _, f := range fields {
		if v, ok := value[f.Name]; ok {
			if err := f.Check(v); err != nil {
				return in, err
			}
		}
	}
	if err := schema.Validate(value); err != nil {
		return in, ops.Errorf(ops.KindInvalid, "%v", err)
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return in, ops.Errorf(ops.KindInvalid, "%v", err)
	}
	return in, nil
}

// result renders a handler's answer the way the SDK's typed handlers do: the
// output as structured content, and the same JSON as text when the handler
// set no content of its own.
func result(res *mcp.CallToolResult, out any) (*mcp.CallToolResult, error) {
	if res == nil {
		res = &mcp.CallToolResult{}
	}
	if out == nil || reflect.ValueOf(out).Kind() == reflect.Pointer && reflect.ValueOf(out).IsNil() {
		return res, nil
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshaling output: %w", err)
	}
	res.StructuredContent = json.RawMessage(raw)
	if res.Content == nil {
		res.Content = []mcp.Content{&mcp.TextContent{Text: string(raw)}}
	}
	return res, nil
}

// Tool is the MCP tool entry for s.
func Tool[In any](s ops.Spec[In]) (*mcp.Tool, error) {
	schema, err := s.InputSchema()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.Name, err)
	}
	info := s.Info()
	return &mcp.Tool{
		Name:        info.ToolName,
		Title:       info.Summary,
		Description: info.Description(),
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: info.Effect == ops.EffectRead},
	}, nil
}

var (
	tracer = observe.Tracer("/internal/ops/mcpops")
	meter  = observe.Meter("/internal/ops/mcpops")

	// The OpenTelemetry MCP semantic conventions' name and attributes, so a
	// dashboard built for any MCP server reads it. The tool name is bounded
	// by the catalog, which is what makes it safe as a label.
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

// instrumentRaw makes every call of a tool root a trigger span and record
// its duration (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
// The span is named from the registered tool rather than from the request,
// so a client naming a tool that does not exist cannot mint a span name: the
// SDK refuses that call before any handler runs.
func instrumentRaw(name string, call func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, any, error)) mcp.ToolHandler {
	t := newTiming(name)
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, done := t.start(ctx)
		res, out, err := call(ctx, req)
		done(err)
		if err != nil {
			var errRes mcp.CallToolResult
			errRes.SetError(err)
			return &errRes, nil
		}
		return result(res, out)
	}
}

type timing struct {
	spanName          string
	name              string
	succeeded, failed metric.MeasurementOption
}

func newTiming(name string) timing {
	return timing{
		spanName: "mcp.tool " + name,
		name:     name,
		succeeded: metric.WithAttributes(
			attribute.String(semconvMethodName, methodToolsCall),
			attribute.String(semconvToolName, name),
		),
		failed: metric.WithAttributes(
			attribute.String(semconvMethodName, methodToolsCall),
			attribute.String(semconvToolName, name),
			attribute.String(semconvErrorType, errorTypeTool),
		),
	}
}

func (t timing) start(ctx context.Context) (context.Context, func(error)) {
	ctx, span := tracer.Start(ctx, t.spanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String(attrToolName, t.name)),
	)
	start := time.Now()
	return ctx, func(err error) {
		outcome := t.succeeded
		if err != nil {
			outcome = t.failed
		}
		toolDuration.Record(ctx, time.Since(start).Seconds(), outcome)
		observe.End(span, &err)
	}
}
