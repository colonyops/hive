package config

import (
	"fmt"
	"strings"

	"github.com/hay-kot/criterio"
)

// ValidateUserCommandBasic returns structural validation errors for a single
// user command, prefixed by field. The result is empty when the command is valid.
func ValidateUserCommandBasic(field, name string, cmd UserCommand) criterio.FieldErrorsBuilder {
	var errs criterio.FieldErrorsBuilder

	if name == "" {
		return errs.Append(field, fmt.Errorf("command name cannot be empty"))
	}
	if !isValidCommandName(name) {
		return errs.Append(field, fmt.Errorf("invalid command name: must contain only alphanumeric characters, dashes, and underscores (no spaces)"))
	}

	hasAction := cmd.Action != ""
	hasSh := cmd.Sh != ""
	hasWindows := len(cmd.Windows) > 0

	if !hasAction && !hasSh && !hasWindows {
		return errs.Append(field, fmt.Errorf("must have at least one of: action, sh, windows"))
	}
	if hasAction && (hasSh || hasWindows) {
		return errs.Append(field, fmt.Errorf("action is mutually exclusive with sh and windows"))
	}
	if hasAction && !isValidAction(cmd.Action) {
		errs = errs.Append(field, fmt.Errorf("invalid action %q", cmd.Action))
	}

	for i, w := range cmd.Windows {
		wfield := fmt.Sprintf("%s.windows[%d]", field, i)
		if w.Name == "" {
			errs = errs.Append(wfield, fmt.Errorf("name is required"))
		}
		if w.Command != "" && len(w.Panes) > 0 {
			errs = errs.Append(wfield, fmt.Errorf("command and panes are mutually exclusive"))
		}
		for j, p := range w.Panes {
			if p.Split != "" && p.Split != "horizontal" && p.Split != "vertical" {
				errs = errs.Append(fmt.Sprintf("%s.panes[%d].split", wfield, j), fmt.Errorf("must be one of: horizontal, vertical"))
			}
		}
	}

	if cmd.Options.Remote != "" && cmd.Options.SessionName == "" {
		errs = errs.Append(field+".options.remote", fmt.Errorf("remote requires session_name to be set"))
	}
	if cmd.Options.Background && !hasWindows {
		errs = errs.Append(field+".options.background", fmt.Errorf("background only applies when windows are defined"))
	}
	if cmd.Options.SessionName != "" && !hasWindows {
		errs = errs.Append(field+".options.session_name", fmt.Errorf("session_name only applies when windows are defined"))
	}

	for _, scope := range cmd.Scope {
		if !isValidScope(scope) {
			errs = errs.Append(field+".scope", fmt.Errorf("invalid scope %q: must be one of: %s", scope, strings.Join(ValidScopes, ", ")))
		}
	}

	if len(cmd.Form) == 0 {
		return errs
	}

	if cmd.Action != "" {
		return errs.Append(field, fmt.Errorf("form can only be used with sh, not action"))
	}

	if err := criterio.ValidateSlice(field+".form", cmd.Form, validateFormField); err != nil {
		return errs.Append("", err)
	}

	seenVars := make(map[string]bool, len(cmd.Form))
	for i, ff := range cmd.Form {
		if seenVars[ff.Variable] {
			errs = errs.Append(fmt.Sprintf("%s.form[%d].variable", field, i), fmt.Errorf("duplicate variable %q", ff.Variable))
		}
		seenVars[ff.Variable] = true
	}

	return errs
}

// ValidateUserCommandTemplates returns template-syntax validation errors for a
// single user command, prefixed by field. The result is empty when all templates
// parse successfully.
func ValidateUserCommandTemplates(field string, cmd UserCommand) criterio.FieldErrorsBuilder {
	var errs criterio.FieldErrorsBuilder
	testData := buildValidationData(cmd.Form)

	if cmd.Sh != "" {
		if err := validateTemplate(cmd.Sh, testData); err != nil {
			errs = errs.Append(field, fmt.Errorf("template error in sh: %w", err))
		}
	}

	if cmd.Options.SessionName != "" {
		if err := validateTemplate(cmd.Options.SessionName, testData); err != nil {
			errs = errs.Append(field+".options.session_name", err)
		}
	}
	if cmd.Options.Remote != "" {
		if err := validateTemplate(cmd.Options.Remote, testData); err != nil {
			errs = errs.Append(field+".options.remote", err)
		}
	}

	for i, w := range cmd.Windows {
		wfield := fmt.Sprintf("%s.windows[%d]", field, i)
		for tname, tmplStr := range map[string]string{
			".name":    w.Name,
			".command": w.Command,
			".dir":     w.Dir,
		} {
			if tmplStr == "" {
				continue
			}
			if err := validateTemplate(tmplStr, testData); err != nil {
				errs = errs.Append(wfield+tname, err)
			}
		}
		for j, p := range w.Panes {
			pfield := fmt.Sprintf("%s.panes[%d]", wfield, j)
			for tname, tmplStr := range map[string]string{
				".command": p.Command,
				".dir":     p.Dir,
				".size":    p.Size,
			} {
				if tmplStr == "" {
					continue
				}
				if err := validateTemplate(tmplStr, testData); err != nil {
					errs = errs.Append(pfield+tname, err)
				}
			}
		}
	}

	return errs
}
