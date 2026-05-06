package config

import (
	"fmt"
	"strings"

	"github.com/hay-kot/criterio"
)

// AppendUserCommandBasicValidation appends basic structural validation errors
// for a single user command onto errs using the provided field prefix.
func AppendUserCommandBasicValidation(errs *criterio.FieldErrorsBuilder, field, name string, cmd UserCommand) {
	if errs == nil {
		return
	}

	// Validate command name format.
	if name == "" {
		*errs = errs.Append(field, fmt.Errorf("command name cannot be empty"))
		return
	}
	if !isValidCommandName(name) {
		*errs = errs.Append(field, fmt.Errorf("invalid command name: must contain only alphanumeric characters, dashes, and underscores (no spaces)"))
		return
	}

	// Must have at least one of: action, sh, windows.
	hasAction := cmd.Action != ""
	hasSh := cmd.Sh != ""
	hasWindows := len(cmd.Windows) > 0

	if !hasAction && !hasSh && !hasWindows {
		*errs = errs.Append(field, fmt.Errorf("must have at least one of: action, sh, windows"))
		return
	}
	if hasAction && (hasSh || hasWindows) {
		*errs = errs.Append(field, fmt.Errorf("action is mutually exclusive with sh and windows"))
		return
	}
	if hasAction && !isValidAction(cmd.Action) {
		*errs = errs.Append(field, fmt.Errorf("invalid action %q", cmd.Action))
	}

	// Validate windows entries.
	for i, w := range cmd.Windows {
		wfield := fmt.Sprintf("%s.windows[%d]", field, i)
		if w.Name == "" {
			*errs = errs.Append(wfield, fmt.Errorf("name is required"))
		}
	}

	// options.remote requires options.session_name.
	if cmd.Options.Remote != "" && cmd.Options.SessionName == "" {
		*errs = errs.Append(field+".options.remote", fmt.Errorf("remote requires session_name to be set"))
	}

	// options.background only meaningful when windows are present.
	if cmd.Options.Background && !hasWindows {
		*errs = errs.Append(field+".options.background", fmt.Errorf("background only applies when windows are defined"))
	}

	// options.session_name only applies when windows are defined.
	if cmd.Options.SessionName != "" && !hasWindows {
		*errs = errs.Append(field+".options.session_name", fmt.Errorf("session_name only applies when windows are defined"))
	}

	// Validate scope values.
	for _, scope := range cmd.Scope {
		if !isValidScope(scope) {
			*errs = errs.Append(field+".scope", fmt.Errorf("invalid scope %q: must be one of: %s", scope, strings.Join(ValidScopes, ", ")))
		}
	}

	// Validate form fields.
	if len(cmd.Form) == 0 {
		return
	}

	if cmd.Action != "" {
		*errs = errs.Append(field, fmt.Errorf("form can only be used with sh, not action"))
		return
	}

	if err := criterio.ValidateSlice(field+".form", cmd.Form, validateFormField); err != nil {
		*errs = errs.Append("", err)
		return
	}

	seenVars := make(map[string]bool, len(cmd.Form))
	for i, ff := range cmd.Form {
		if seenVars[ff.Variable] {
			*errs = errs.Append(fmt.Sprintf("%s.form[%d].variable", field, i), fmt.Errorf("duplicate variable %q", ff.Variable))
		}
		seenVars[ff.Variable] = true
	}
}

// AppendUserCommandTemplateValidation appends template syntax validation errors
// for a single user command onto errs using the provided field prefix.
func AppendUserCommandTemplateValidation(errs *criterio.FieldErrorsBuilder, field string, cmd UserCommand) {
	if errs == nil {
		return
	}

	testData := buildValidationData(cmd.Form)

	if cmd.Sh != "" {
		if err := validateTemplate(cmd.Sh, testData); err != nil {
			*errs = errs.Append(field, fmt.Errorf("template error in sh: %w", err))
		}
	}

	if cmd.Options.SessionName != "" {
		if err := validateTemplate(cmd.Options.SessionName, testData); err != nil {
			*errs = errs.Append(field+".options.session_name", err)
		}
	}
	if cmd.Options.Remote != "" {
		if err := validateTemplate(cmd.Options.Remote, testData); err != nil {
			*errs = errs.Append(field+".options.remote", err)
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
				*errs = errs.Append(wfield+tname, err)
			}
		}
	}
}
