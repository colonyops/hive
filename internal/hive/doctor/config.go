package doctor

import (
	"context"
	"errors"

	"github.com/colonyops/hive/internal/config"
	"github.com/hay-kot/criterio"
)

// ConfigCheck validates the configuration file.
type ConfigCheck struct {
	validate func() error
	warnings []config.ValidationWarning
}

// NewConfigCheck creates a configuration check. validate is the program's
// whole-config validator; a nil validate reports the config as not loaded.
func NewConfigCheck(validate func() error, warnings []config.ValidationWarning) *ConfigCheck {
	return &ConfigCheck{
		validate: validate,
		warnings: warnings,
	}
}

func (c *ConfigCheck) Name() string {
	return "Configuration"
}

func (c *ConfigCheck) Run(ctx context.Context) Result {
	result := Result{Name: c.Name()}

	if c.validate == nil {
		result.Items = append(result.Items, CheckItem{
			Label:  "Config loaded",
			Status: StatusFail,
			Detail: "configuration not loaded",
		})
		return result
	}

	err := c.validate()
	warnings := c.warnings

	// If no errors and no warnings, report success
	if err == nil && len(warnings) == 0 {
		result.Items = append(result.Items, CheckItem{
			Label:  "Config valid",
			Status: StatusPass,
		})
		return result
	}

	// Extract and report errors
	if err != nil {
		var fieldErrs criterio.FieldErrors
		if errors.As(err, &fieldErrs) {
			for _, fe := range fieldErrs {
				label := fe.Field
				if label == "" {
					label = "validation"
				}
				result.Items = append(result.Items, CheckItem{
					Label:  label,
					Status: StatusFail,
					Detail: fe.Err.Error(),
				})
			}
		} else {
			result.Items = append(result.Items, CheckItem{
				Label:  "validation",
				Status: StatusFail,
				Detail: err.Error(),
			})
		}
	}

	// Extract and report warnings
	for _, w := range warnings {
		label := w.Category
		if w.Item != "" {
			label += " (" + w.Item + ")"
		}
		result.Items = append(result.Items, CheckItem{
			Label:  label,
			Status: StatusWarn,
			Detail: w.Message,
		})
	}

	return result
}
