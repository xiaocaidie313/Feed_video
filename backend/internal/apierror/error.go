package apierror

import (
	"errors"
	"net/http"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrValidation   = errors.New("validation error")
	ErrNotFound     = errors.New("not found")
	ErrInternal     = errors.New("internal server error")
)

func MathErrorsToStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK // 200
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized // 401
	case errors.Is(err, ErrValidation):
		return http.StatusBadRequest // 400
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound // 404
	case errors.Is(err, ErrInternal):
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError //500
	}
}
