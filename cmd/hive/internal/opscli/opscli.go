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

// Run runs one engine operation in-process and returns its answer.
type Run func(ctx context.Context, c *cli.Command, info ops.Info, args map[string]any) (any, error)

// DesktopCommands builds a command group for every desktop-owned op, at
// Path below the parent. It fails on an op whose input has a field the CLI
// cannot spell, so a bad spec fails the first test rather than a user's call.
//
// reserved names the flags of the parent command. urfave/cli resolves a flag
// name from the nearest command first, so a field flag with a reserved name
// would hide the parent's flag from the command.
func DesktopCommands(list []ops.Op, call Call, caller Caller, reserved ...string) ([]*cli.Command, error) {
	return commands(list, ops.OwnerDesktop, Path, nil, nil, func(ctx context.Context, c *cli.Command, info ops.Info, args map[string]any) error {
		out, err := call(ctx, c, info.Toolset.MCPPath, info.ToolName, args)
		if err != nil {
			return err
		}
		return iojson.WriteLine(c.Root().Writer, out)
	}, caller, reserved)
}

// Engine says how EngineCommands names, runs and prints the engine ops.
type Engine struct {
	Run Run
	// Print writes an answer. Nil prints it with Print.
	Print func(ctx context.Context, c *cli.Command, info ops.Info, out any) error
	// Path places an op's command below the root. Nil is Path.
	Path func(ops.Info) []string
	// Aliases are other names for an op's command, by op name.
	Aliases map[string][]string
	// JoinRest names the ops whose last positional string argument takes the
	// rest of the words, joined by spaces, as in hive session create fix the bug.
	JoinRest map[string]bool
	Caller   Caller
	Reserved []string
}

// EngineCommands builds a command group for every engine-owned op.
func EngineCommands(list []ops.Op, e Engine) ([]*cli.Command, error) {
	path, print := e.Path, e.Print
	if path == nil {
		path = Path
	}
	if print == nil {
		print = func(_ context.Context, c *cli.Command, _ ops.Info, out any) error { return Print(c.Root().Writer, out) }
	}
	return commands(list, ops.OwnerEngine, path, e.Aliases, e.JoinRest, func(ctx context.Context, c *cli.Command, info ops.Info, args map[string]any) error {
		out, err := e.Run(ctx, c, info, args)
		if err != nil {
			return err
		}
		return print(ctx, c, info, out)
	}, e.Caller, e.Reserved)
}

// JSONLines is an answer the CLI prints as one JSON line per value, such as a
// list printed one element per line.
type JSONLines interface{ JSONLines() []any }

// Print writes out as JSON Lines: each of its JSONLines when it has them,
// else out itself on one line.
func Print(w io.Writer, out any) error {
	lines, ok := out.(JSONLines)
	if !ok {
		return iojson.WriteLine(w, out)
	}
	for _, line := range lines.JSONLines() {
		if err := iojson.WriteLine(w, line); err != nil {
			return err
		}
	}
	return nil
}

func commands(list []ops.Op, owner ops.Owner, path func(ops.Info) []string, aliases map[string][]string, joinRest map[string]bool, run func(context.Context, *cli.Command, ops.Info, map[string]any) error, caller Caller, reserved []string) ([]*cli.Command, error) {
	root := &cli.Command{}
	for _, op := range list {
		info := op.Info()
		if info.Owner != owner {
			continue
		}
		cmd, err := command(op, func(ctx context.Context, c *cli.Command, args map[string]any) error {
			return run(ctx, c, info, args)
		}, caller, reserved, joinRest[info.Name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", info.Name, err)
		}
		p := path(info)
		cmd.Name, cmd.Aliases = p[len(p)-1], aliases[info.Name]
		parent := root
		for i, name := range p[:len(p)-1] {
			usage := "Operations on " + strings.Join(p[:i+1], " ")
			if i == len(p)-2 {
				usage = "Operations on " + strings.ReplaceAll(info.Resource, "_", " ")
			}
			if parent, err = group(parent, name, usage); err != nil {
				return nil, fmt.Errorf("%s: %w", info.Name, err)
			}
		}
		if slices.ContainsFunc(parent.Commands, func(c *cli.Command) bool { return c.Name == cmd.Name }) {
			return nil, fmt.Errorf("%s: %s is already a command", info.Name, strings.Join(p, " "))
		}
		parent.Commands = append(parent.Commands, cmd)
	}
	return root.Commands, nil
}

// Path is the command path of an op below its parent: one group for each
// word of the resource, then the verb. diff_review.put is diff review put.
func Path(info ops.Info) []string {
	words := strings.Split(info.Resource, "_")
	for i, w := range words {
		words[i] = Kebab(w)
	}
	return append(words, info.Verb)
}

func group(parent *cli.Command, name, usage string) (*cli.Command, error) {
	for _, c := range parent.Commands {
		if c.Name != name {
			continue
		}
		if c.Action != nil {
			return nil, fmt.Errorf("%s is both a command and a group", name)
		}
		return c, nil
	}
	g := &cli.Command{Name: name, Usage: usage}
	parent.Commands = append(parent.Commands, g)
	return g, nil
}

const inputFlag = "input"

func command(op ops.Op, run func(context.Context, *cli.Command, map[string]any) error, caller Caller, reserved []string, joinRest bool) (*cli.Command, error) {
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
		last := len(positional) == len(info.Args)-1
		if fields[i].Kind() != reflect.String && !isNumber(fields[i].Kind()) && (!last || !isStrings(fields[i])) {
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

	variadic := len(positional) > 0 && isStrings(positional[len(positional)-1])
	if joinRest && (len(positional) == 0 || positional[len(positional)-1].Kind() != reflect.String) {
		return nil, fmt.Errorf("JoinRest needs a last positional string argument")
	}
	rest := variadic || joinRest
	usage := make([]string, len(positional))
	for i, f := range positional {
		if f.Required {
			usage[i] = "<" + Kebab(f.Name) + ">"
		} else {
			usage[i] = "[" + Kebab(f.Name) + "]"
		}
	}
	if rest {
		usage[len(usage)-1] += "..."
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
			if c.Args().Len() > len(positional) && !rest {
				return ops.Errorf(ops.KindInvalid, "%s takes at most %d argument(s): %s", info.Name, len(positional), strings.Join(usage, " "))
			}
			for i, f := range positional[:min(c.Args().Len(), len(positional))] {
				if variadic && i == len(positional)-1 {
					args[f.Name] = c.Args().Slice()[i:]
					break
				}
				if joinRest && i == len(positional)-1 {
					args[f.Name] = strings.Join(c.Args().Slice()[i:], " ")
					break
				}
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
			if err := validate(op, args); err != nil {
				return err
			}
			return run(ctx, c, args)
		},
	}, nil
}

// validate runs the input type's cross-field rules, so the CLI refuses the
// same mistakes the desktop would without dialing it.
func validate(op ops.Op, args map[string]any) error {
	in := reflect.New(op.InputType())
	v, ok := in.Interface().(ops.Validator)
	if !ok {
		return nil
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode arguments: %w", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return ops.Errorf(ops.KindInvalid, "%v", err)
	}
	if err := v.Validate(); err != nil {
		return ops.Errorf(ops.KindInvalid, "%s", err.Error())
	}
	return nil
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

func isStrings(f ops.Field) bool {
	return f.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String
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
	if args == nil {
		return nil, ops.Errorf(ops.KindInvalid, "--input is not a JSON object")
	}
	return args, nil
}

// Kebab spells a json or resource name the way a flag or command is spelled:
// profileId is profile-id and diff_review is diff-review.
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
