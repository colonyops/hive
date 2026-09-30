package stores

type MapFunc[T any, U any] func(T) U

func (a MapFunc[T, U]) Slice(v []T) []U {
	result := make([]U, len(v))
	for i := range v {
		result[i] = a(v[i])
	}
	return result
}

func (a MapFunc[T, U]) Err(v T, err error) (U, error) {
	if err != nil {
		var zero U
		return zero, err
	}

	return a(v), nil
}

func (a MapFunc[T, U]) SliceErr(v []T, err error) ([]U, error) {
	if err != nil {
		return nil, err
	}

	return a.Slice(v), nil
}
