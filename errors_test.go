package volt

import (
	"errors"
	"net/http"
	"testing"
)

func TestErrorConstructors(t *testing.T) {
	tests := []struct {
		name           string
		constructor    func() *Error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "ErrNotFound",
			constructor:    func() *Error { return ErrNotFound("user") },
			expectedStatus: http.StatusNotFound,
			expectedCode:   "NOT_FOUND",
		},
		{
			name:           "ErrBadRequest",
			constructor:    func() *Error { return ErrBadRequest("invalid input") },
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "BAD_REQUEST",
		},
		{
			name:           "ErrUnauthorized",
			constructor:    func() *Error { return ErrUnauthorized("") },
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
		},
		{
			name:           "ErrForbidden",
			constructor:    func() *Error { return ErrForbidden("") },
			expectedStatus: http.StatusForbidden,
			expectedCode:   "FORBIDDEN",
		},
		{
			name:           "ErrConflict",
			constructor:    func() *Error { return ErrConflict("already exists") },
			expectedStatus: http.StatusConflict,
			expectedCode:   "CONFLICT",
		},
		{
			name:           "ErrValidation",
			constructor:    func() *Error { return ErrValidation("invalid email") },
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   "VALIDATION_ERROR",
		},
		{
			name:           "ErrInternal",
			constructor:    func() *Error { return ErrInternal("") },
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
		},
		{
			name:           "ErrServiceUnavailable",
			constructor:    func() *Error { return ErrServiceUnavailable("") },
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "SERVICE_UNAVAILABLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.constructor()

			assertEqual(t, tt.expectedStatus, err.status)
			assertEqual(t, tt.expectedCode, err.code)
			assertEqual(t, tt.expectedStatus, err.GetStatus())
		})
	}
}

func TestNewError(t *testing.T) {
	t.Run("creates error with status and message", func(t *testing.T) {
		err := NewError(http.StatusTeapot, "I'm a teapot")

		assertEqual(t, http.StatusTeapot, err.status)
		assertEqual(t, "I'm a teapot", err.message)
		assertEqual(t, "I'm a teapot", err.Error())
	})
}

func TestWrap(t *testing.T) {
	t.Run("wraps error with message", func(t *testing.T) {
		cause := errors.New("database connection failed")
		err := Wrap(cause, "failed to get user")

		assertEqual(t, http.StatusInternalServerError, err.status)
		assertEqual(t, "failed to get user: database connection failed", err.Error())
	})

	t.Run("wrapped error can be unwrapped", func(t *testing.T) {
		cause := errors.New("original error")
		err := Wrap(cause, "wrapped")

		unwrapped := err.Unwrap()
		assertEqual(t, cause, unwrapped)
	})
}

func TestErrorChaining(t *testing.T) {
	t.Run("WithCode sets error code", func(t *testing.T) {
		err := ErrBadRequest("invalid").WithCode("CUSTOM_CODE")

		assertEqual(t, "CUSTOM_CODE", err.code)
	})

	t.Run("WithDetail sets detail", func(t *testing.T) {
		err := ErrBadRequest("invalid").WithDetail("field 'email' must be valid")

		assertEqual(t, "field 'email' must be valid", err.detail)
	})

	t.Run("WithCause sets cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := ErrBadRequest("invalid").WithCause(cause)

		assertEqual(t, cause, err.cause)
	})

	t.Run("chains can be combined", func(t *testing.T) {
		cause := errors.New("db error")
		err := ErrNotFound("user").
			WithCode("USER_NOT_FOUND").
			WithDetail("No user with ID 123").
			WithCause(cause)

		assertEqual(t, "USER_NOT_FOUND", err.code)
		assertEqual(t, "No user with ID 123", err.detail)
		assertEqual(t, cause, err.cause)
	})
}

func TestErrorCheckers(t *testing.T) {
	t.Run("IsNotFound", func(t *testing.T) {
		assertTrue(t, IsNotFound(ErrNotFound("test")))
		assertTrue(t, !IsNotFound(ErrBadRequest("test")))
		assertTrue(t, !IsNotFound(errors.New("regular error")))
	})

	t.Run("IsConflict", func(t *testing.T) {
		assertTrue(t, IsConflict(ErrConflict("test")))
		assertTrue(t, !IsConflict(ErrNotFound("test")))
		assertTrue(t, !IsConflict(errors.New("regular error")))
	})

	t.Run("IsValidation", func(t *testing.T) {
		assertTrue(t, IsValidation(ErrValidation("test")))
		assertTrue(t, IsValidation(ErrBadRequest("test")))
		assertTrue(t, !IsValidation(ErrNotFound("test")))
		assertTrue(t, !IsValidation(errors.New("regular error")))
	})
}

func TestStatusFromError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "volt error returns its status",
			err:      ErrNotFound("test"),
			expected: http.StatusNotFound,
		},
		{
			name:     "regular error returns 500",
			err:      errors.New("regular error"),
			expected: http.StatusInternalServerError,
		},
		{
			name:     "wrapped volt error returns correct status",
			err:      ErrForbidden("wrapped").WithCause(errors.New("cause")),
			expected: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := StatusFromError(tt.err)
			assertEqual(t, tt.expected, status)
		})
	}
}

func TestErrorsAs(t *testing.T) {
	t.Run("errors.As works with volt errors", func(t *testing.T) {
		err := ErrNotFound("user")
		var voltErr *Error

		assertTrue(t, errors.As(err, &voltErr))
		assertEqual(t, http.StatusNotFound, voltErr.status)
	})

	t.Run("errors.As works with wrapped errors", func(t *testing.T) {
		inner := ErrNotFound("user")
		outer := Wrap(inner, "failed to process")
		var voltErr *Error

		assertTrue(t, errors.As(outer, &voltErr))
	})
}
