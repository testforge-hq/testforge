package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Error codes for categorization
const (
	// Client errors (4xx)
	ErrCodeValidation      = "VALIDATION_ERROR"
	ErrCodeNotFound        = "NOT_FOUND"
	ErrCodeConflict        = "CONFLICT"
	ErrCodeUnauthorized    = "UNAUTHORIZED"
	ErrCodeForbidden       = "FORBIDDEN"
	ErrCodeRateLimited     = "RATE_LIMITED"
	ErrCodeBadRequest      = "BAD_REQUEST"
	ErrCodePayloadTooLarge = "PAYLOAD_TOO_LARGE"

	// Server errors (5xx)
	ErrCodeInternal       = "INTERNAL_ERROR"
	ErrCodeDatabase       = "DATABASE_ERROR"
	ErrCodeExternalAPI    = "EXTERNAL_API_ERROR"
	ErrCodeTimeout        = "TIMEOUT_ERROR"
	ErrCodeServiceUnavail = "SERVICE_UNAVAILABLE"

	// Business logic errors
	ErrCodeDiscoveryFailed   = "DISCOVERY_FAILED"
	ErrCodeTestDesignFailed  = "TEST_DESIGN_FAILED"
	ErrCodeExecutionFailed   = "EXECUTION_FAILED"
	ErrCodeHealingFailed     = "HEALING_FAILED"
	ErrCodeReportGenFailed   = "REPORT_GENERATION_FAILED"
	ErrCodeQuotaExceeded     = "QUOTA_EXCEEDED"
	ErrCodeInvalidWorkflow   = "INVALID_WORKFLOW"
)

// AppError is the base error type for all application errors
type AppError struct {
	// Error code for programmatic handling
	Code string `json:"code"`

	// Human-readable message
	Message string `json:"message"`

	// Detailed description (optional, for developers)
	Details string `json:"details,omitempty"`

	// HTTP status code
	HTTPStatus int `json:"-"`

	// Original error (for error wrapping)
	Cause error `json:"-"`

	// Metadata for additional context
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Timestamp when error occurred
	Timestamp time.Time `json:"timestamp"`

	// Request ID for tracing
	RequestID string `json:"request_id,omitempty"`

	// Retry information
	Retryable   bool          `json:"retryable"`
	RetryAfter  time.Duration `json:"retry_after,omitempty"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// Is implements errors.Is for error comparison
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// WithDetails adds details to the error
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// WithCause adds the underlying cause
func (e *AppError) WithCause(err error) *AppError {
	e.Cause = err
	return e
}

// WithMetadata adds metadata to the error
func (e *AppError) WithMetadata(key string, value interface{}) *AppError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// WithRequestID adds request ID for tracing
func (e *AppError) WithRequestID(requestID string) *AppError {
	e.RequestID = requestID
	return e
}

// WithRetry marks the error as retryable
func (e *AppError) WithRetry(after time.Duration) *AppError {
	e.Retryable = true
	e.RetryAfter = after
	return e
}

// ToJSON serializes the error to JSON
func (e *AppError) ToJSON() []byte {
	data, _ := json.Marshal(e)
	return data
}

// Error constructors

// NewError creates a new AppError
func NewError(code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Timestamp:  time.Now().UTC(),
	}
}

// Validation errors

func ErrValidation(message string) *AppError {
	return NewError(ErrCodeValidation, message, http.StatusBadRequest)
}

func ErrValidationField(field, message string) *AppError {
	return NewError(ErrCodeValidation, message, http.StatusBadRequest).
		WithMetadata("field", field)
}

// Not found errors

func ErrNotFound(resource, id string) *AppError {
	return NewError(ErrCodeNotFound, fmt.Sprintf("%s not found: %s", resource, id), http.StatusNotFound).
		WithMetadata("resource", resource).
		WithMetadata("id", id)
}

func ErrTenantNotFound(id string) *AppError {
	return ErrNotFound("tenant", id)
}

func ErrProjectNotFound(id string) *AppError {
	return ErrNotFound("project", id)
}

func ErrTestRunNotFound(id string) *AppError {
	return ErrNotFound("test_run", id)
}

// Authorization errors

func ErrUnauthorized(message string) *AppError {
	if message == "" {
		message = "Authentication required"
	}
	return NewError(ErrCodeUnauthorized, message, http.StatusUnauthorized)
}

func ErrForbidden(message string) *AppError {
	if message == "" {
		message = "Access denied"
	}
	return NewError(ErrCodeForbidden, message, http.StatusForbidden)
}

// Rate limiting

func ErrRateLimited(retryAfter time.Duration) *AppError {
	return NewError(ErrCodeRateLimited, "Rate limit exceeded", http.StatusTooManyRequests).
		WithRetry(retryAfter)
}

// Conflict errors

func ErrConflict(message string) *AppError {
	return NewError(ErrCodeConflict, message, http.StatusConflict)
}

func ErrDuplicate(resource, field, value string) *AppError {
	return NewError(ErrCodeConflict, fmt.Sprintf("%s with %s '%s' already exists", resource, field, value), http.StatusConflict).
		WithMetadata("resource", resource).
		WithMetadata("field", field).
		WithMetadata("value", value)
}

// Server errors

func ErrInternal(message string) *AppError {
	if message == "" {
		message = "Internal server error"
	}
	return NewError(ErrCodeInternal, message, http.StatusInternalServerError)
}

func ErrDatabase(err error) *AppError {
	return NewError(ErrCodeDatabase, "Database error", http.StatusInternalServerError).
		WithCause(err)
}

func ErrExternalAPI(service string, err error) *AppError {
	return NewError(ErrCodeExternalAPI, fmt.Sprintf("External API error: %s", service), http.StatusBadGateway).
		WithCause(err).
		WithMetadata("service", service).
		WithRetry(5 * time.Second)
}

func ErrTimeout(operation string) *AppError {
	return NewError(ErrCodeTimeout, fmt.Sprintf("Operation timed out: %s", operation), http.StatusGatewayTimeout).
		WithMetadata("operation", operation).
		WithRetry(10 * time.Second)
}

func ErrServiceUnavailable(service string) *AppError {
	return NewError(ErrCodeServiceUnavail, fmt.Sprintf("Service unavailable: %s", service), http.StatusServiceUnavailable).
		WithMetadata("service", service).
		WithRetry(30 * time.Second)
}

// Business logic errors

func ErrDiscoveryFailed(reason string, err error) *AppError {
	return NewError(ErrCodeDiscoveryFailed, fmt.Sprintf("Discovery failed: %s", reason), http.StatusUnprocessableEntity).
		WithCause(err)
}

func ErrTestDesignFailed(reason string, err error) *AppError {
	return NewError(ErrCodeTestDesignFailed, fmt.Sprintf("Test design failed: %s", reason), http.StatusUnprocessableEntity).
		WithCause(err)
}

func ErrExecutionFailed(reason string, err error) *AppError {
	return NewError(ErrCodeExecutionFailed, fmt.Sprintf("Test execution failed: %s", reason), http.StatusUnprocessableEntity).
		WithCause(err)
}

func ErrHealingFailed(reason string, err error) *AppError {
	return NewError(ErrCodeHealingFailed, fmt.Sprintf("Self-healing failed: %s", reason), http.StatusUnprocessableEntity).
		WithCause(err)
}

func ErrQuotaExceeded(resource string, limit int) *AppError {
	return NewError(ErrCodeQuotaExceeded, fmt.Sprintf("Quota exceeded for %s (limit: %d)", resource, limit), http.StatusForbidden).
		WithMetadata("resource", resource).
		WithMetadata("limit", limit)
}

// Helper functions

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	var appErr *AppError
	return errors.As(err, &appErr)
}

// AsAppError converts an error to AppError if possible
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// WrapError wraps a standard error into an AppError
func WrapError(err error, code, message string, httpStatus int) *AppError {
	return NewError(code, message, httpStatus).WithCause(err)
}

// GetHTTPStatus returns the HTTP status code for an error
func GetHTTPStatus(err error) int {
	if appErr, ok := AsAppError(err); ok {
		return appErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

// GetErrorCode returns the error code for an error
func GetErrorCode(err error) string {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code
	}
	return ErrCodeInternal
}

// Sentinel errors for comparison (used with errors.Is)
var (
	ErrNotFoundSentinel     = NewError(ErrCodeNotFound, "not found", http.StatusNotFound)
	ErrUnauthorizedSentinel = NewError(ErrCodeUnauthorized, "unauthorized", http.StatusUnauthorized)
	ErrForbiddenSentinel    = NewError(ErrCodeForbidden, "forbidden", http.StatusForbidden)
	ErrConflictSentinel     = NewError(ErrCodeConflict, "conflict", http.StatusConflict)
)

// DomainError is a structured error for domain operations
type DomainError struct {
	Code    string
	Message string
	Details map[string]any
	Err     error
}

// Error implements the error interface
func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *DomainError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is for error comparison
func (e *DomainError) Is(target error) bool {
	t, ok := target.(*DomainError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// Sentinel domain errors (used with errors.Is)
var (
	ErrNotFoundVal      = &DomainError{Code: ErrCodeNotFound, Message: "not found"}
	ErrAlreadyExistsVal = &DomainError{Code: ErrCodeConflict, Message: "already exists"}
	ErrInvalidInputVal  = &DomainError{Code: ErrCodeValidation, Message: "invalid input"}
	ErrUnauthorizedVal  = &DomainError{Code: ErrCodeUnauthorized, Message: "unauthorized"}
	ErrForbiddenVal     = &DomainError{Code: ErrCodeForbidden, Message: "forbidden"}
	ErrQuotaExceededVal = &DomainError{Code: ErrCodeQuotaExceeded, Message: "quota exceeded"}
	ErrConflictVal      = &DomainError{Code: ErrCodeConflict, Message: "conflict"}
)

// IsSentinelError checks if err matches a sentinel error
func IsSentinelError(err error, sentinel *DomainError) bool {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return domainErr.Code == sentinel.Code
	}
	return false
}

// NotFoundError creates a not found domain error
func NotFoundError(resource string, id any) *DomainError {
	return &DomainError{
		Code:    ErrCodeNotFound,
		Message: fmt.Sprintf("%s not found: %v", resource, id),
		Details: map[string]any{"resource": resource, "id": id},
		Err:     ErrNotFoundVal,
	}
}

// AlreadyExistsError creates an already exists domain error
func AlreadyExistsError(resource, field, value string) *DomainError {
	return &DomainError{
		Code:    ErrCodeConflict,
		Message: fmt.Sprintf("%s with %s '%s' already exists", resource, field, value),
		Details: map[string]any{"resource": resource, "field": field, "value": value},
		Err:     ErrAlreadyExistsVal,
	}
}

// ValidationError creates a validation domain error
func ValidationError(field, message string) *DomainError {
	return &DomainError{
		Code:    ErrCodeValidation,
		Message: message,
		Details: map[string]any{"field": field},
		Err:     ErrInvalidInputVal,
	}
}
