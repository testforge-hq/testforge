package domain

import (
	"errors"
	"net/http"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		want string
	}{
		{
			name: "error without cause",
			err: &AppError{
				Code:    ErrCodeNotFound,
				Message: "Resource not found",
			},
			want: "[NOT_FOUND] Resource not found",
		},
		{
			name: "error with cause",
			err: &AppError{
				Code:    ErrCodeNotFound,
				Message: "Resource not found",
				Cause:   errors.New("id: 123"),
			},
			want: "[NOT_FOUND] Resource not found: id: 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("AppError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := &AppError{
		Code:    "TEST",
		Message: "outer error",
		Cause:   inner,
	}

	if !errors.Is(err, inner) {
		t.Error("AppError.Unwrap() should allow errors.Is to find inner error")
	}
}

func TestNewError(t *testing.T) {
	err := NewError("DB_ERROR", "Database error", http.StatusInternalServerError)

	if err.Code != "DB_ERROR" {
		t.Errorf("Code = %s, want DB_ERROR", err.Code)
	}
	if err.Message != "Database error" {
		t.Errorf("Message = %s, want Database error", err.Message)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusInternalServerError)
	}
}

func TestAppError_WithMethods(t *testing.T) {
	err := NewError("TEST", "Test error", http.StatusBadRequest).
		WithDetails("Additional details").
		WithMetadata("key", "value").
		WithRequestID("req-123")

	if err.Details != "Additional details" {
		t.Errorf("Details = %s, want 'Additional details'", err.Details)
	}
	if err.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %v, want 'value'", err.Metadata["key"])
	}
	if err.RequestID != "req-123" {
		t.Errorf("RequestID = %s, want 'req-123'", err.RequestID)
	}
}

func TestDomainError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *DomainError
		want string
	}{
		{
			name: "error without wrapped error",
			err: &DomainError{
				Code:    ErrCodeNotFound,
				Message: "Resource not found",
			},
			want: "[NOT_FOUND] Resource not found",
		},
		{
			name: "error with wrapped error",
			err: &DomainError{
				Code:    ErrCodeNotFound,
				Message: "Resource not found",
				Err:     errors.New("id: 123"),
			},
			want: "[NOT_FOUND] Resource not found: id: 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("DomainError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNotFoundError(t *testing.T) {
	err := NotFoundError("user", "123")

	if err.Code != ErrCodeNotFound {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeNotFound)
	}
	if err.Details["resource"] != "user" {
		t.Errorf("Details[resource] = %v, want 'user'", err.Details["resource"])
	}
	if err.Details["id"] != "123" {
		t.Errorf("Details[id] = %v, want '123'", err.Details["id"])
	}
}

func TestAlreadyExistsError(t *testing.T) {
	err := AlreadyExistsError("tenant", "slug", "my-tenant")

	if err.Code != ErrCodeConflict {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeConflict)
	}
	if err.Details["resource"] != "tenant" {
		t.Errorf("Details[resource] = %v, want 'tenant'", err.Details["resource"])
	}
}

func TestIsNotFoundError(t *testing.T) {
	notFoundErr := NotFoundError("user", "123")
	validationErr := ValidationError("name", "Name is required")

	if !IsNotFoundError(notFoundErr) {
		t.Error("IsNotFoundError should return true for NotFoundError")
	}
	if IsNotFoundError(validationErr) {
		t.Error("IsNotFoundError should return false for ValidationError")
	}
	if IsNotFoundError(errors.New("random error")) {
		t.Error("IsNotFoundError should return false for non-domain errors")
	}
}

func TestIsAlreadyExistsError(t *testing.T) {
	existsErr := AlreadyExistsError("tenant", "slug", "my-tenant")
	notFoundErr := NotFoundError("user", "123")

	if !IsAlreadyExistsError(existsErr) {
		t.Error("IsAlreadyExistsError should return true for AlreadyExistsError")
	}
	if IsAlreadyExistsError(notFoundErr) {
		t.Error("IsAlreadyExistsError should return false for NotFoundError")
	}
}

func TestIsValidationError(t *testing.T) {
	validationErr := ValidationError("email", "Invalid email format")
	notFoundErr := NotFoundError("user", "123")

	if !IsValidationError(validationErr) {
		t.Error("IsValidationError should return true for ValidationError")
	}
	if IsValidationError(notFoundErr) {
		t.Error("IsValidationError should return false for NotFoundError")
	}
}

func TestSentinelErrors(t *testing.T) {
	// Test that sentinel errors can be used with errors.Is
	notFoundErr := NotFoundError("user", "123")

	if !errors.Is(notFoundErr, ErrNotFoundVal) {
		t.Error("NotFoundError should match ErrNotFoundVal with errors.Is")
	}

	existsErr := AlreadyExistsError("tenant", "slug", "test")
	if !errors.Is(existsErr, ErrAlreadyExistsVal) {
		t.Error("AlreadyExistsError should match ErrAlreadyExistsVal with errors.Is")
	}
}

func TestGetHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "not found error",
			err:        ErrNotFound("user", "123"),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "validation error",
			err:        ErrValidation("Invalid input"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unauthorized error",
			err:        ErrUnauthorized(""),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "non-app error",
			err:        errors.New("random error"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetHTTPStatus(tt.err)
			if got != tt.wantStatus {
				t.Errorf("GetHTTPStatus() = %d, want %d", got, tt.wantStatus)
			}
		})
	}
}

func TestTimestamps_SetTimestamps(t *testing.T) {
	var ts Timestamps
	ts.SetTimestamps()

	if ts.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero after SetTimestamps")
	}
	if ts.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero after SetTimestamps")
	}
	if ts.DeletedAt != nil {
		t.Error("DeletedAt should be nil after SetTimestamps")
	}
}
