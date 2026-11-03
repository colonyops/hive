// Package opscli builds hive commands from catalog operations: one
// `<resource> <verb>` command per operation, with flags from its input
// fields, help from its summary and guidance, and the enum and range checks
// of its constraints.
package opscli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/pkg/iojson"
)

// Call sends one tool call to the desktop and returns its result.
type Call func(ctx context.Context, c *cli.Command, mcpPath, tool string, args map[string]any) (any, error)

// Caller resolves the caller field of an input.
type Caller func(c *cli.Command) (string, error)

// DesktopCommands builds `<resource-kebab> <verb>` groups for every
// desktop-owned op. It fails on an op whose input has a field the CLI cannot
// spell, so a bad spec fails the first test rather than a user's call.
//
// reserved names the flags of the parent command. urfave/cli resolves a flag
// name from the nearest command first, so a field flag with a reserved name
// would hide the parent's flag from the command.
func DesktopCommands(list []ops.Op, call Call, caller Caller, reserved ...string) ([]*cli.Command, error) {
	var groups []*cli.Command
	for _, op := range list {
		info := op.Info()
		if info.Owner != ops.OwnerDesktop {
			continue
		}
		cmd, err := command(op, func(ctx context.Context, c *cli.Command, args map[string]any) error {
			out, err := call(ctx, c, info.Toolset.MCPPath, info.ToolName, args)
			if err != nil {
				return err
			}
			return iojson.WriteLine(c.Root().Writer, out)
		}, caller, reserved)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", info.Name, err)
		}
		group := findOrAdd(&groups, Kebab(info.Resource))
		group.Commands = append(group.Commands, cmd)
	}
	return groups, nil
}

// Path is the command path of an op below its parent: resource, then verb.
func Path(info ops.Info) []string { return []string{Kebab(info.Resource), info.Verb} }

func findOrAdd(groups *[]*cli.Command, name string) *cli.Command {
	for _, g := range *groups {
		if g.Name == name {
			return g
		}
	}
	g := &cli.Command{Name: name, Usage: "Operations on " + strings.ReplaceAll(name, "-", " ")}
	*groups = append(*groups, g)
	return g
}

const inputFlag = "input"

func command(op ops.Op, run func(context.Context, *cli.Command, map[string]any) error, caller Caller, reserved []string) (*cli.Command, error) {
	info := op.Info()
	fields, err := ops.Fields(op)
	if err != nil {
		return nil, err
	}

	var positional []ops.Field
	for _, name := range info.Args {
		i := slices.IndexFunc(fields, func(f ops.Field) bool { return f.Name == name })
		if i < 0 {
			return nil, fmt.Errorf("arg %q names no field", name)
		}
		if fields[i].Kind() != reflect.String && !isNumber(fields[i].Kind()) {
			return nil, fmt.Errorf("arg %q: kind %v cannot be positional", name, fields[i].Kind())
		}
		positional = append(positional, fields[i])
	}

	flags := []cli.Flag{&cli.StringFlag{
		Name:  inputFlag,
		Usage: "The whole input as JSON: inline, @file, or - for stdin. Flags and arguments override its fields",
	}}
	var nested []string
	for _, f := range fields {
		if f.Name == ops.CallerField || slices.Contains(info.Args, f.Name) {
			continue
		}
		flag, ok := flagFor(f)
		if !ok {
			nested = append(nested, f.Name)
			continue
		}
		if name := Kebab(f.Name); name == inputFlag || slices.Contains(reserved, name) {
			return nil, fmt.Errorf("field %q: flag --%s is reserved", f.Name, name)
		}
		flags = append(flags, flag)
	}

	usage := make([]string, len(positional))
	for i, f := range positional {
		if f.Required {
			usage[i] = "<" + Kebab(f.Name) + ">"
		} else {
			usage[i] = "[" + Kebab(f.Name) + "]"
		}
	}
	description := info.Guidance
	if len(nested) > 0 {
		description += "\n\nSet " + strings.Join(nested, ", ") + " with --input."
	}

	return &cli.Command{
		Name:        info.Verb,
		Usage:       info.Summary,
		ArgsUsage:   strings.Join(usage, " "),
		Description: description,
		Flags:       flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			args, err := input(c)
			if err != nil {
				return err
			}
			if c.Args().Len() > len(positional) {
				return ops.Errorf(ops.KindInvalid, "%s takes at most %d argument(s): %s", info.Name, len(positional), strings.Join(usage, " "))
			}
			for i, f := range positional[:c.Args().Len()] {
				v, err := parseArg(f, c.Args().Get(i))
				if err != nil {
					return err
				}
				args[f.Name] = v
			}
			for _, f := range fields {
				if f.Name == ops.CallerField || slices.Contains(info.Args, f.Name) || !c.IsSet(Kebab(f.Name)) {
					continue
				}
				args[f.Name] = flagValue(c, f)
			}
			if slices.ContainsFunc(fields, func(f ops.Field) bool { return f.Name == ops.CallerField }) {
				if _, given := args[ops.CallerField]; !given {
					who, err := caller(c)
					if err != nil {
						return err
					}
					args[ops.CallerField] = who
				}
			}
			for _, f := range fields {
				v, ok := args[f.Name]
				if !ok {
					if f.Required {
						return ops.Errorf(ops.KindInvalid, "%s is required", spell(f, info))
					}
					continue
				}
				if err := f.Check(v); err != nil {
					return err
				}
			}
			return run(ctx, c, args)
		},
	}, nil
}

func spell(f ops.Field, info ops.Info) string {
	if slices.Contains(info.Args, f.Name) {
		return "<" + Kebab(f.Name) + ">"
	}
	if _, ok := flagFor(f); !ok {
		return f.Name + " (set it with --input)"
	}
	return "--" + Kebab(f.Name)
}

func isNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int64, reflect.Float64:
		return true
	default:
		return false
	}
}

func flagFor(f ops.Field) (cli.Flag, bool) {
	name := Kebab(f.Name)
	usage := strings.TrimSuffix(f.Description, ".")
	if len(f.Enum) > 0 {
		usage += ". One of: " + strings.Join(f.Enum, ", ")
	}
	if f.Default != nil {
		usage += fmt.Sprintf(". Default: %v", f.Default)
	}
	switch f.Kind() {
	case reflect.String:
		return &cli.StringFlag{Name: name, Usage: usage}, true
	case reflect.Bool:
		return &cli.BoolFlag{Name: name, Usage: usage}, true
	case reflect.Int, reflect.Int64:
		return &cli.Int64Flag{Name: name, Usage: usage}, true
	case reflect.Float64:
		return &cli.FloatFlag{Name: name, Usage: usage}, true
	case reflect.Slice:
		if f.Type.Elem().Kind() == reflect.String {
			return &cli.StringSliceFlag{Name: name, Usage: usage + ". Repeat the flag for more than one"}, true
		}
		return nil, false
	default:
		return nil, false
	}
}

func flagValue(c *cli.Command, f ops.Field) any {
	name := Kebab(f.Name)
	switch f.Kind() {
	case reflect.Bool:
		return c.Bool(name)
	case reflect.Int, reflect.Int64:
		return c.Int64(name)
	case reflect.Float64:
		return c.Float(name)
	case reflect.Slice:
		return c.StringSlice(name)
	default:
		return c.String(name)
	}
}

func parseArg(f ops.Field, raw string) (any, error) {
	switch f.Kind() {
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, ops.Errorf(ops.KindInvalid, "<%s> %q is not an integer", Kebab(f.Name), raw)
		}
		return n, nil
	case reflect.Float64:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, ops.Errorf(ops.KindInvalid, "<%s> %q is not a number", Kebab(f.Name), raw)
		}
		return n, nil
	default:
		return raw, nil
	}
}

// input reads --input: inline JSON, @file, or - for stdin.
func input(c *cli.Command) (map[string]any, error) {
	raw := c.String(inputFlag)
	args := map[string]any{}
	if raw == "" {
		return args, nil
	}
	var data []byte
	switch {
	case raw == "-":
		reader := c.Root().Reader
		if reader == nil {
			reader = os.Stdin
		}
		b, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("read --input from stdin: %w", err)
		}
		data = b
	case strings.HasPrefix(raw, "@"):
		b, err := os.ReadFile(raw[1:])
		if err != nil {
			return nil, ops.Errorf(ops.KindInvalid, "read --input: %v", err)
		}
		data = b
	default:
		data = []byte(raw)
	}
	if err := json.Unmarshal(data, &args); err != nil {
		return nil, ops.Errorf(ops.KindInvalid, "--input is not a JSON object: %v", err)
	}
	return args, nil
}

// Kebab spells a json or resource name the way a flag or command is spelled:
// profileId is profile-id and canvas_block is canvas-block.
func Kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '_':
			b.WriteByte('-')
		case unicode.IsUpper(r):
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
