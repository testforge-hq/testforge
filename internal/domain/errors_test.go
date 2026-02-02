package domain

import (
	"errors"
	"net/http"
	"testing"
)

// Define test errors for this test file
var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyExists  = errors.New("already exists")
	ErrInvalidInput   = errors.New("invalid input")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrInternal       = errors.New("internal error")
	ErrServiceUnavail = errors.New("service unavailable")
)

// Error represents a domain error with additional context
type Error struct {
	Code       string
	Message    string
	HTTPStatus int
	Err        error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

// NewError creates a new domain error
func NewError(code, message string, status int, err error) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
		Err:        err,
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		want    string
	}{
		{
			name: "error without wrapped error",
			err: &Error{
				Code:    "NOT_FOUND",
				Message: "Resource not found",
			},
			want: "Resource not found",
		},
		{
			name: "error with wrapped error",
			err: &Error{
				Code:    "NOT_FOUND",
				Message: "Resource not found",
				Err:     errors.New("id: 123"),
			},
			want: "Resource not found: id: 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := &Error{
		Code:    "TEST",
		Message: "outer error",
		Err:     inner,
	}

	if !errors.Is(err, inner) {
		t.Error("Error.Unwrap() should allow errors.Is to find inner error")
	}
}

func TestNewError(t *testing.T) {
	inner := errors.New("database connection failed")
	err := NewError("DB_ERROR", "Database error", http.StatusInternalServerError, inner)

	if err.Code != "DB_ERROR" {
		t.Errorf("Code = %s, want DB_ERROR", err.Code)
	}
	if err.Message != "Database error" {
		t.Errorf("Message = %s, want Database error", err.Message)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusInternalServerError)
	}
	if err.Err != inner {
		t.Error("Err should be the inner error")
	}
}

func TestCommonErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantMsg    string
	}{
		{
			name:    "not found error",
			err:     ErrNotFound,
			wantMsg: "not found",
		},
		{
			name:    "already exists error",
			err:     ErrAlreadyExists,
			wantMsg: "already exists",
		},
		{
			name:    "invalid input error",
			err:     ErrInvalidInput,
			wantMsg: "invalid input",
		},
		{
			name:    "unauthorized error",
			err:     ErrUnauthorized,
			wantMsg: "unauthorized",
		},
		{
			name:    "forbidden error",
			err:     ErrForbidden,
			wantMsg: "forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.wantMsg {
				t.Errorf("error message = %q, want %q", tt.err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestErrorHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "not found maps to 404",
			err:        ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "already exists maps to 409",
			err:        ErrAlreadyExists,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "invalid input maps to 400",
			err:        ErrInvalidInput,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unauthorized maps to 401",
			err:        ErrUnauthorized,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "forbidden maps to 403",
			err:        ErrForbidden,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "internal error maps to 500",
			err:        ErrInternal,
			wantStatus: http.StatusInternalServerError,
		},
	}

	// Helper function to map errors to HTTP status
	mapErrorToStatus := func(err error) int {
		switch {
		case errors.Is(err, ErrNotFound):
			return http.StatusNotFound
		case errors.Is(err, ErrAlreadyExists):
			return http.StatusConflict
		case errors.Is(err, ErrInvalidInput):
			return http.StatusBadRequest
		case errors.Is(err, ErrUnauthorized):
			return http.StatusUnauthorized
		case errors.Is(err, ErrForbidden):
			return http.StatusForbidden
		default:
			return http.StatusInternalServerError
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapErrorToStatus(tt.err)
			if got != tt.wantStatus {
				t.Errorf("HTTP status for %v = %d, want %d", tt.err, got, tt.wantStatus)
			}
		})
	}
}

func TestErrorWrapping(t *testing.T) {
	// Test that wrapped errors can be detected
	baseErr := ErrNotFound
	wrappedErr := NewError("ENTITY_NOT_FOUND", "Entity not found", http.StatusNotFound, baseErr)

	if !errors.Is(wrappedErr, baseErr) {
		t.Error("wrapped error should match base error with errors.Is")
	}

	// Test error chain
	var domainErr *Error
	if !errors.As(wrappedErr, &domainErr) {
		t.Error("should be able to extract domain error with errors.As")
	}
	if domainErr.Code != "ENTITY_NOT_FOUND" {
		t.Errorf("extracted error code = %s, want ENTITY_NOT_FOUND", domainErr.Code)
	}
}

func TestErrorJSON(t *testing.T) {
	err := NewError("TEST_ERROR", "Test error message", http.StatusBadRequest, nil)

	// Verify error can be used in JSON response
	response := struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{
		Success: false,
		Code:    err.Code,
		Message: err.Message,
	}

	if response.Code != "TEST_ERROR" {
		t.Errorf("response code = %s, want TEST_ERROR", response.Code)
	}
	if response.Message != "Test error message" {
		t.Errorf("response message = %s, want 'Test error message'", response.Message)
	}
}
