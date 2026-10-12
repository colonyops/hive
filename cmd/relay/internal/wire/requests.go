package wire

import (
	"errors"
	"strings"

	"github.com/hay-kot/criterio"
)

const (
	MaxSourceName   = 80
	MaxSourceSecret = 128
	MaxTokenName    = 80
	DefaultPageSize = 200
	MaxPageSize     = 500
)

type CreateSource struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

func (b CreateSource) Validate() error {
	return criterio.ValidateStruct(
		validateSourceName(b.Name),
		criterio.Run("secret", b.Secret, secretRules...),
	)
}

type UpdateSource struct {
	Name   *string `json:"name"`
	Secret *string `json:"secret"`
}

func (b UpdateSource) Validate() error {
	var errs []error
	if b.Name != nil {
		errs = append(errs, validateSourceName(*b.Name))
	}
	if b.Secret != nil {
		errs = append(errs, criterio.Run("secret", *b.Secret, secretRules...))
	}
	return criterio.ValidateStruct(errs...)
}

type CreateToken struct {
	Name string `json:"name"`
}

func (b CreateToken) Validate() error {
	return criterio.Run("name", strings.TrimSpace(b.Name), criterio.StrBetween(1, MaxTokenName))
}

type DeliveriesQuery struct {
	After string `query:"after"`
	// Limit of 0 takes DefaultPageSize.
	Limit int `query:"limit"`
}

func (q DeliveriesQuery) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("after", q.After, criterio.When(q.After != "", criterio.StrUUID)),
		criterio.Run("limit", q.Limit, criterio.When(q.Limit != 0, criterio.Between(1, MaxPageSize))),
	)
}

func (q DeliveriesQuery) PageSize() int {
	if q.Limit == 0 {
		return DefaultPageSize
	}
	return q.Limit
}

func validateSourceName(name string) error {
	return criterio.Run("name", strings.TrimSpace(name), criterio.StrBetween(1, MaxSourceName))
}

var secretRules = []criterio.Validator[string]{criterio.StrMax(MaxSourceSecret), printableASCII}

func printableASCII(s string) error {
	for i := range len(s) {
		if s[i] < 0x20 || s[i] > 0x7e {
			return errors.New("must be printable ASCII")
		}
	}
	return nil
}
