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

func TestAppError_WithCause(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewError("TEST", "Test error", http.StatusBadRequest).
		WithCause(cause)

	if err.Cause != cause {
		t.Error("WithCause should set the Cause field")
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the cause")
	}
}

func TestAppError_WithRetry(t *testing.T) {
	err := NewError("TEST", "Test error", http.StatusTooManyRequests).
		WithRetry(30 * 1000000000) // 30 seconds

	if !err.Retryable {
		t.Error("WithRetry should set Retryable to true")
	}
	if err.RetryAfter != 30*1000000000 {
		t.Errorf("RetryAfter = %v, want 30s", err.RetryAfter)
	}
}

func TestAppError_ToJSON(t *testing.T) {
	err := NewError("TEST", "Test error", http.StatusBadRequest)
	json := err.ToJSON()

	if len(json) == 0 {
		t.Error("ToJSON should return non-empty bytes")
	}
	// Check it contains expected fields
	jsonStr := string(json)
	if !contains(jsonStr, "TEST") {
		t.Error("ToJSON should contain error code")
	}
	if !contains(jsonStr, "Test error") {
		t.Error("ToJSON should contain error message")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestErrValidationField(t *testing.T) {
	err := ErrValidationField("email", "Invalid email format")

	if err.Code != ErrCodeValidation {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeValidation)
	}
	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusBadRequest)
	}
	if err.Metadata["field"] != "email" {
		t.Errorf("Metadata[field] = %v, want 'email'", err.Metadata["field"])
	}
}

func TestErrTenantNotFound(t *testing.T) {
	err := ErrTenantNotFound("tenant-123")

	if err.Code != ErrCodeNotFound {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeNotFound)
	}
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusNotFound)
	}
}

func TestErrProjectNotFound(t *testing.T) {
	err := ErrProjectNotFound("project-123")

	if err.Code != ErrCodeNotFound {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeNotFound)
	}
}

func TestErrTestRunNotFound(t *testing.T) {
	err := ErrTestRunNotFound("run-123")

	if err.Code != ErrCodeNotFound {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeNotFound)
	}
}

func TestErrForbidden(t *testing.T) {
	// With empty message
	err := ErrForbidden("")
	if err.Message != "Access denied" {
		t.Errorf("Message = %s, want 'Access denied'", err.Message)
	}
	if err.HTTPStatus != http.StatusForbidden {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusForbidden)
	}

	// With custom message
	err2 := ErrForbidden("Custom forbidden message")
	if err2.Message != "Custom forbidden message" {
		t.Errorf("Message = %s, want 'Custom forbidden message'", err2.Message)
	}
}

func TestErrRateLimited(t *testing.T) {
	err := ErrRateLimited(60 * 1000000000) // 60 seconds

	if err.Code != ErrCodeRateLimited {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeRateLimited)
	}
	if err.HTTPStatus != http.StatusTooManyRequests {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusTooManyRequests)
	}
	if !err.Retryable {
		t.Error("Should be retryable")
	}
}

func TestErrConflict(t *testing.T) {
	err := ErrConflict("Resource already exists")

	if err.Code != ErrCodeConflict {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeConflict)
	}
	if err.HTTPStatus != http.StatusConflict {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusConflict)
	}
}

func TestErrDuplicate(t *testing.T) {
	err := ErrDuplicate("tenant", "slug", "my-tenant")

	if err.Code != ErrCodeConflict {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeConflict)
	}
	if err.Metadata["resource"] != "tenant" {
		t.Errorf("Metadata[resource] = %v, want 'tenant'", err.Metadata["resource"])
	}
	if err.Metadata["field"] != "slug" {
		t.Errorf("Metadata[field] = %v, want 'slug'", err.Metadata["field"])
	}
	if err.Metadata["value"] != "my-tenant" {
		t.Errorf("Metadata[value] = %v, want 'my-tenant'", err.Metadata["value"])
	}
}

func TestErrInternal(t *testing.T) {
	// With empty message
	err := ErrInternal("")
	if err.Message != "Internal server error" {
		t.Errorf("Message = %s, want 'Internal server error'", err.Message)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusInternalServerError)
	}

	// With custom message
	err2 := ErrInternal("Custom internal error")
	if err2.Message != "Custom internal error" {
		t.Errorf("Message = %s, want 'Custom internal error'", err2.Message)
	}
}

func TestErrDatabase(t *testing.T) {
	cause := errors.New("connection refused")
	err := ErrDatabase(cause)

	if err.Code != ErrCodeDatabase {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeDatabase)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusInternalServerError)
	}
	if !errors.Is(err, cause) {
		t.Error("Should wrap the cause error")
	}
}

func TestErrExternalAPI(t *testing.T) {
	cause := errors.New("timeout")
	err := ErrExternalAPI("stripe", cause)

	if err.Code != ErrCodeExternalAPI {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeExternalAPI)
	}
	if err.HTTPStatus != http.StatusBadGateway {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusBadGateway)
	}
	if !err.Retryable {
		t.Error("Should be retryable")
	}
	if err.Metadata["service"] != "stripe" {
		t.Errorf("Metadata[service] = %v, want 'stripe'", err.Metadata["service"])
	}
}

func TestErrTimeout(t *testing.T) {
	err := ErrTimeout("database query")

	if err.Code != ErrCodeTimeout {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeTimeout)
	}
	if err.HTTPStatus != http.StatusGatewayTimeout {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusGatewayTimeout)
	}
	if !err.Retryable {
		t.Error("Should be retryable")
	}
	if err.Metadata["operation"] != "database query" {
		t.Errorf("Metadata[operation] = %v, want 'database query'", err.Metadata["operation"])
	}
}

func TestErrServiceUnavailable(t *testing.T) {
	err := ErrServiceUnavailable("temporal")

	if err.Code != ErrCodeServiceUnavail {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeServiceUnavail)
	}
	if err.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusServiceUnavailable)
	}
	if !err.Retryable {
		t.Error("Should be retryable")
	}
}
