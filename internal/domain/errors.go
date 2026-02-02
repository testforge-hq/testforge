package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors for the domain layer
var (
	ErrNotFound          = errors.New("resource not found")
	ErrAlreadyExists     = errors.New("resource already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrConflict          = errors.New("conflict")
	ErrQuotaExceeded     = errors.New("quota exceeded")
	ErrWorkflowFailed    = errors.New("workflow failed")
	ErrExecutionFailed   = errors.New("execution failed")
	ErrSelfHealingFailed = errors.New("self-healing failed")
)

// DomainError wraps errors with additional context
type DomainError struct {
	Err     error
	Message string
	Code    string
	Details map[string]any
}

func (e *DomainError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Err.Error()
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewDomainError creates a new domain error
func NewDomainError(err error, message, code string) *DomainError {
	return &DomainError{
		Err:     err,
		Message: message,
		Code:    code,
	}
}

// WithDetails adds details to the error
func (e *DomainError) WithDetails(details map[string]any) *DomainError {
	e.Details = details
	return e
}

// Common error constructors
func NotFoundError(resource string, id any) *DomainError {
	return &DomainError{
		Err:     ErrNotFound,
		Message: fmt.Sprintf("%s not found", resource),
		Code:    "NOT_FOUND",
		Details: map[string]any{"resource": resource, "id": id},
	}
}

func AlreadyExistsError(resource, field string, value any) *DomainError {
	return &DomainError{
		Err:     ErrAlreadyExists,
		Message: fmt.Sprintf("%s with %s '%v' already exists", resource, field, value),
		Code:    "ALREADY_EXISTS",
		Details: map[string]any{"resource": resource, "field": field, "value": value},
	}
}

func ValidationError(field, reason string) *DomainError {
	return &DomainError{
		Err:     ErrInvalidInput,
		Message: fmt.Sprintf("validation failed for %s: %s", field, reason),
		Code:    "VALIDATION_ERROR",
		Details: map[string]any{"field": field, "reason": reason},
	}
}

func QuotaExceededError(resource string, limit, current int) *DomainError {
	return &DomainError{
		Err:     ErrQuotaExceeded,
		Message: fmt.Sprintf("%s quota exceeded: limit=%d, current=%d", resource, limit, current),
		Code:    "QUOTA_EXCEEDED",
		Details: map[string]any{"resource": resource, "limit": limit, "current": current},
	}
}

// IsNotFound checks if error is a not found error
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists checks if error is an already exists error
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsValidationError checks if error is a validation error
func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}

// IsQuotaExceeded checks if error is a quota exceeded error
func IsQuotaExceeded(err error) bool {
	return errors.Is(err, ErrQuotaExceeded)
}
