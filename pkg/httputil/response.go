package httputil

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/testforge/testforge/internal/domain"
)

// Response represents a standard API response
type Response struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   *Error `json:"error,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

// Error represents an API error
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Meta contains pagination and other metadata
type Meta struct {
	Page       int `json:"page,omitempty"`
	PerPage    int `json:"per_page,omitempty"`
	Total      int `json:"total,omitempty"`
	TotalPages int `json:"total_pages,omitempty"`
}

// JSON writes a JSON response
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: status >= 200 && status < 300,
		Data:    data,
	}

	json.NewEncoder(w).Encode(resp)
}

// JSONWithMeta writes a JSON response with pagination metadata
func JSONWithMeta(w http.ResponseWriter, status int, data any, meta *Meta) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: true,
		Data:    data,
		Meta:    meta,
	}

	json.NewEncoder(w).Encode(resp)
}

// JSONError writes a JSON error response
func JSONError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: false,
		Error: &Error{
			Code:    code,
			Message: message,
			Details: details,
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// ErrorFromDomain converts a domain error to HTTP response
func ErrorFromDomain(w http.ResponseWriter, err error) {
	var domainErr *domain.DomainError

	if errors.As(err, &domainErr) {
		status := domainErrorToStatus(domainErr)
		JSONError(w, status, domainErr.Code, domainErr.Message, domainErr.Details)
		return
	}

	// Check AppError
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		JSONError(w, appErr.HTTPStatus, appErr.Code, appErr.Message, nil)
		return
	}

	// Default to internal error
	JSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
}

func domainErrorToStatus(err *domain.DomainError) int {
	switch err.Code {
	case domain.ErrCodeNotFound:
		return http.StatusNotFound
	case domain.ErrCodeConflict:
		return http.StatusConflict
	case domain.ErrCodeValidation, domain.ErrCodeBadRequest:
		return http.StatusBadRequest
	case domain.ErrCodeUnauthorized:
		return http.StatusUnauthorized
	case domain.ErrCodeForbidden:
		return http.StatusForbidden
	case domain.ErrCodeQuotaExceeded, domain.ErrCodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// DecodeJSON decodes JSON from request body
func DecodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return domain.ValidationError("body", "request body is required")
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		return domain.ValidationError("body", "invalid JSON: "+err.Error())
	}

	return nil
}

// Pagination extracts pagination params from request
type Pagination struct {
	Page    int
	PerPage int
	Offset  int
}

// GetPagination extracts pagination from query params
func GetPagination(r *http.Request, defaultPerPage, maxPerPage int) Pagination {
	page := 1
	perPage := defaultPerPage

	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := parsePositiveInt(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if parsed, err := parsePositiveInt(pp); err == nil && parsed > 0 {
			perPage = parsed
		}
	}

	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	return Pagination{
		Page:    page,
		PerPage: perPage,
		Offset:  (page - 1) * perPage,
	}
}

func parsePositiveInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// CalculateTotalPages calculates total pages from total items and per page
func CalculateTotalPages(total, perPage int) int {
	if perPage <= 0 {
		return 0
	}
	pages := total / perPage
	if total%perPage > 0 {
		pages++
	}
	return pages
}
