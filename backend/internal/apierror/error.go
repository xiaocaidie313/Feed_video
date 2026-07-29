package apierror

import (
	"errors"
	"net/http"
)

var (
	ErrUnauthorized = errors.New("unauthorized") // error的 interface 本质是 string
	ErrValidation   = errors.New("validation error")
	ErrNotFound     = errors.New("not found")
	ErrInternal     = errors.New("internal server error")
)

func MathErrorsToStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrValidation):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInternal):
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
