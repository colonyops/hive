package extractors

import (
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/colonyops/hive/internal/web"
)

// Query decodes r.URL.Query() into T by `query:"name"` struct tags (string,
// int, bool, and slices of them), then calls T.Validate when it has one.
//
// A field without a tag, or tagged "-", is skipped. A missing key leaves the
// zero value. A non-slice field takes the first value of its key.
func Query[T any](r *http.Request) (T, error) {
	var v T
	rv := reflect.ValueOf(&v).Elem()
	if rv.Kind() != reflect.Struct {
		return v, &web.BadRequestError{Msg: "query target must be a struct"}
	}
	if err := decodeQuery(rv, r.URL.Query()); err != nil {
		return v, err
	}
	return v, validate(v)
}

func decodeQuery(rv reflect.Value, values url.Values) error {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		name := field.Tag.Get("query")
		if name == "" || name == "-" || !field.IsExported() {
			continue
		}
		raw, ok := values[name]
		if !ok || len(raw) == 0 {
			continue
		}
		if err := setQueryField(rv.Field(i), raw); err != nil {
			return &web.BadRequestError{Msg: "invalid query parameter " + name, Err: err}
		}
	}
	return nil
}

func setQueryField(fv reflect.Value, raw []string) error {
	if fv.Kind() != reflect.Slice {
		return setQueryScalar(fv, raw[0])
	}
	out := reflect.MakeSlice(fv.Type(), len(raw), len(raw))
	for i, s := range raw {
		if err := setQueryScalar(out.Index(i), s); err != nil {
			return err
		}
	}
	fv.Set(out)
	return nil
}

func setQueryScalar(fv reflect.Value, s string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(s)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	default:
		return &unsupportedKindError{kind: fv.Kind()}
	}
	return nil
}

type unsupportedKindError struct{ kind reflect.Kind }

func (e *unsupportedKindError) Error() string {
	return "unsupported query field kind " + e.kind.String()
}
