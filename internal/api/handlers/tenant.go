package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/testforge/testforge/internal/domain"
	"github.com/testforge/testforge/internal/repository/postgres"
	rediscache "github.com/testforge/testforge/internal/repository/redis"
	"github.com/testforge/testforge/pkg/httputil"
	"go.uber.org/zap"
)

// TenantHandler handles tenant-related requests
type TenantHandler struct {
	repo   *postgres.TenantRepository
	cache  *rediscache.Cache
	logger *zap.Logger
}

// NewTenantHandler creates a new tenant handler
func NewTenantHandler(repo *postgres.TenantRepository, cache *rediscache.Cache, logger *zap.Logger) *TenantHandler {
	return &TenantHandler{
		repo:   repo,
		cache:  cache,
		logger: logger,
	}
}

// slugRegex validates slug format
var slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{2,62}[a-z0-9]$`)

// TenantResponse is the API representation of a tenant
type TenantResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Slug      string                 `json:"slug"`
	Plan      string                 `json:"plan"`
	Settings  domain.TenantSettings  `json:"settings"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

func toTenantResponse(t *domain.Tenant) TenantResponse {
	return TenantResponse{
		ID:        t.ID.String(),
		Name:      t.Name,
		Slug:      t.Slug,
		Plan:      string(t.Plan),
		Settings:  t.Settings,
		CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// List handles GET /api/v1/tenants
func (h *TenantHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := httputil.GetPagination(r, 20, 100)

	tenants, total, err := h.repo.List(r.Context(), pagination.PerPage, pagination.Offset)
	if err != nil {
		h.logger.Error("Failed to list tenants", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	response := make([]TenantResponse, len(tenants))
	for i, t := range tenants {
		response[i] = toTenantResponse(t)
	}

	httputil.JSONWithMeta(w, http.StatusOK, response, &httputil.Meta{
		Page:       pagination.Page,
		PerPage:    pagination.PerPage,
		Total:      total,
		TotalPages: httputil.CalculateTotalPages(total, pagination.PerPage),
	})
}

// Get handles GET /api/v1/tenants/{id}
func (h *TenantHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid tenant ID format", nil)
		return
	}

	// Try cache first
	if h.cache != nil {
		if cached, err := h.cache.GetTenant(r.Context(), id); err == nil && cached != nil {
			httputil.JSON(w, http.StatusOK, toTenantResponse(cached))
			return
		}
	}

	tenant, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Cache the result
	if h.cache != nil {
		h.cache.SetTenant(r.Context(), tenant)
	}

	httputil.JSON(w, http.StatusOK, toTenantResponse(tenant))
}

// GetBySlug handles GET /api/v1/tenants/slug/{slug}
func (h *TenantHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	// Try cache first
	if h.cache != nil {
		if cached, err := h.cache.GetTenantBySlug(r.Context(), slug); err == nil && cached != nil {
			httputil.JSON(w, http.StatusOK, toTenantResponse(cached))
			return
		}
	}

	tenant, err := h.repo.GetBySlug(r.Context(), slug)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Cache the result
	if h.cache != nil {
		h.cache.SetTenant(r.Context(), tenant)
		h.cache.SetTenantBySlug(r.Context(), tenant)
	}

	httputil.JSON(w, http.StatusOK, toTenantResponse(tenant))
}

// CreateTenantRequest is the request body for creating a tenant
type CreateTenantRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Plan string `json:"plan"`
}

// Create handles POST /api/v1/tenants
func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTenantRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Validate input
	if err := h.validateCreateRequest(&req); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check if slug already exists
	exists, err := h.repo.ExistsBySlug(r.Context(), req.Slug)
	if err != nil {
		h.logger.Error("Failed to check slug existence", zap.Error(err))
		httputil.JSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create tenant", nil)
		return
	}
	if exists {
		httputil.JSONError(w, http.StatusConflict, "ALREADY_EXISTS", "Tenant with this slug already exists", map[string]any{
			"field": "slug",
			"value": req.Slug,
		})
		return
	}

	// Create tenant
	plan := domain.Plan(req.Plan)
	tenant := domain.NewTenant(req.Name, req.Slug, plan)

	if err := h.repo.Create(r.Context(), tenant); err != nil {
		h.logger.Error("Failed to create tenant", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	h.logger.Info("Tenant created",
		zap.String("tenant_id", tenant.ID.String()),
		zap.String("slug", tenant.Slug),
	)

	httputil.JSON(w, http.StatusCreated, toTenantResponse(tenant))
}

// UpdateTenantRequest is the request body for updating a tenant
type UpdateTenantRequest struct {
	Name     *string                 `json:"name,omitempty"`
	Plan     *string                 `json:"plan,omitempty"`
	Settings *domain.TenantSettings  `json:"settings,omitempty"`
}

// Update handles PUT /api/v1/tenants/{id}
func (h *TenantHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid tenant ID format", nil)
		return
	}

	var req UpdateTenantRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Get existing tenant
	tenant, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Apply updates
	if req.Name != nil {
		if *req.Name == "" {
			httputil.JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Name cannot be empty", nil)
			return
		}
		tenant.Name = *req.Name
	}

	if req.Plan != nil {
		plan := domain.Plan(*req.Plan)
		if !plan.IsValid() {
			httputil.JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid plan. Must be: free, pro, or enterprise", nil)
			return
		}
		tenant.Plan = plan
	}

	if req.Settings != nil {
		tenant.Settings = *req.Settings
	}

	// Save updates
	if err := h.repo.Update(r.Context(), tenant); err != nil {
		h.logger.Error("Failed to update tenant", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Invalidate cache
	if h.cache != nil {
		h.cache.InvalidateTenant(r.Context(), id)
	}

	h.logger.Info("Tenant updated", zap.String("tenant_id", id.String()))

	httputil.JSON(w, http.StatusOK, toTenantResponse(tenant))
}

// Delete handles DELETE /api/v1/tenants/{id}
func (h *TenantHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid tenant ID format", nil)
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Invalidate cache
	if h.cache != nil {
		h.cache.InvalidateTenant(r.Context(), id)
	}

	h.logger.Info("Tenant deleted", zap.String("tenant_id", id.String()))

	httputil.JSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// validateCreateRequest validates the create tenant request
func (h *TenantHandler) validateCreateRequest(req *CreateTenantRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return domain.ValidationError("name", "name is required")
	}

	if len(req.Name) > 255 {
		return domain.ValidationError("name", "name must be 255 characters or less")
	}

	if strings.TrimSpace(req.Slug) == "" {
		return domain.ValidationError("slug", "slug is required")
	}

	if !slugRegex.MatchString(req.Slug) {
		return domain.ValidationError("slug", "slug must be 4-64 characters, start with a letter, and contain only lowercase letters, numbers, and hyphens")
	}

	plan := domain.Plan(req.Plan)
	if req.Plan != "" && !plan.IsValid() {
		return domain.ValidationError("plan", "plan must be: free, pro, or enterprise")
	}

	// Default to free plan if not specified
	if req.Plan == "" {
		req.Plan = string(domain.PlanFree)
	}

	return nil
}
