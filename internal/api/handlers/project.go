package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/testforge/testforge/pkg/httputil"
	"github.com/testforge/testforge/internal/api/middleware"
	"github.com/testforge/testforge/internal/domain"
	"github.com/testforge/testforge/internal/repository/postgres"
	"go.uber.org/zap"
)

// ProjectHandler handles project-related requests
type ProjectHandler struct {
	repo       *postgres.ProjectRepository
	tenantRepo *postgres.TenantRepository
	logger     *zap.Logger
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(repo *postgres.ProjectRepository, tenantRepo *postgres.TenantRepository, logger *zap.Logger) *ProjectHandler {
	return &ProjectHandler{
		repo:       repo,
		tenantRepo: tenantRepo,
		logger:     logger,
	}
}

// ProjectResponse is the API representation of a project
type ProjectResponse struct {
	ID          string                  `json:"id"`
	TenantID    string                  `json:"tenant_id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	BaseURL     string                  `json:"base_url"`
	Settings    domain.ProjectSettings  `json:"settings"`
	CreatedAt   string                  `json:"created_at"`
	UpdatedAt   string                  `json:"updated_at"`
}

func toProjectResponse(p *domain.Project) ProjectResponse {
	return ProjectResponse{
		ID:          p.ID.String(),
		TenantID:    p.TenantID.String(),
		Name:        p.Name,
		Description: p.Description,
		BaseURL:     p.BaseURL,
		Settings:    p.Settings,
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// List handles GET /api/v1/tenants/{tenant_id}/projects
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid tenant ID format", nil)
		return
	}

	pagination := httputil.GetPagination(r, 20, 100)

	projects, total, err := h.repo.GetByTenantID(r.Context(), tenantID, pagination.PerPage, pagination.Offset)
	if err != nil {
		h.logger.Error("Failed to list projects", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	response := make([]ProjectResponse, len(projects))
	for i, p := range projects {
		response[i] = toProjectResponse(p)
	}

	httputil.JSONWithMeta(w, http.StatusOK, response, &httputil.Meta{
		Page:       pagination.Page,
		PerPage:    pagination.PerPage,
		Total:      total,
		TotalPages: httputil.CalculateTotalPages(total, pagination.PerPage),
	})
}

// Get handles GET /api/v1/projects/{id}
func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid project ID format", nil)
		return
	}

	project, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check tenant access
	if tenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if project.TenantID != tenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	httputil.JSON(w, http.StatusOK, toProjectResponse(project))
}

// CreateProjectRequest is the request body for creating a project
type CreateProjectRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	BaseURL     string                  `json:"base_url"`
	Settings    *domain.ProjectSettings `json:"settings,omitempty"`
}

// Create handles POST /api/v1/tenants/{tenant_id}/projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid tenant ID format", nil)
		return
	}

	var req CreateProjectRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Validate input
	if err := h.validateCreateRequest(&req); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Verify tenant exists
	if _, err := h.tenantRepo.GetByID(r.Context(), tenantID); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check if project name already exists for tenant
	exists, err := h.repo.ExistsByNameAndTenant(r.Context(), req.Name, tenantID)
	if err != nil {
		h.logger.Error("Failed to check project existence", zap.Error(err))
		httputil.JSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create project", nil)
		return
	}
	if exists {
		httputil.JSONError(w, http.StatusConflict, "ALREADY_EXISTS", "Project with this name already exists", map[string]any{
			"field": "name",
			"value": req.Name,
		})
		return
	}

	// Create project
	project := domain.NewProject(tenantID, req.Name, req.Description, req.BaseURL)
	if req.Settings != nil {
		project.Settings = *req.Settings
	}

	if err := h.repo.Create(r.Context(), project); err != nil {
		h.logger.Error("Failed to create project", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	h.logger.Info("Project created",
		zap.String("project_id", project.ID.String()),
		zap.String("tenant_id", tenantID.String()),
	)

	httputil.JSON(w, http.StatusCreated, toProjectResponse(project))
}

// UpdateProjectRequest is the request body for updating a project
type UpdateProjectRequest struct {
	Name        *string                 `json:"name,omitempty"`
	Description *string                 `json:"description,omitempty"`
	BaseURL     *string                 `json:"base_url,omitempty"`
	Settings    *domain.ProjectSettings `json:"settings,omitempty"`
}

// Update handles PUT /api/v1/projects/{id}
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid project ID format", nil)
		return
	}

	var req UpdateProjectRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Get existing project
	project, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check tenant access
	if tenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if project.TenantID != tenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	// Apply updates
	if req.Name != nil {
		if *req.Name == "" {
			httputil.JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Name cannot be empty", nil)
			return
		}
		project.Name = *req.Name
	}

	if req.Description != nil {
		project.Description = *req.Description
	}

	if req.BaseURL != nil {
		if err := validateURL(*req.BaseURL); err != nil {
			httputil.JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
		project.BaseURL = *req.BaseURL
	}

	if req.Settings != nil {
		project.Settings = *req.Settings
	}

	// Save updates
	if err := h.repo.Update(r.Context(), project); err != nil {
		h.logger.Error("Failed to update project", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	h.logger.Info("Project updated", zap.String("project_id", id.String()))

	httputil.JSON(w, http.StatusOK, toProjectResponse(project))
}

// Delete handles DELETE /api/v1/projects/{id}
func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid project ID format", nil)
		return
	}

	// Get project to check ownership
	project, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check tenant access
	if tenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if project.TenantID != tenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	h.logger.Info("Project deleted", zap.String("project_id", id.String()))

	httputil.JSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// getTenantID extracts tenant ID from URL or context
func (h *ProjectHandler) getTenantID(r *http.Request) (uuid.UUID, error) {
	// Try URL param first
	if idStr := chi.URLParam(r, "tenant_id"); idStr != "" {
		return uuid.Parse(idStr)
	}

	// Try context
	if id, ok := middleware.GetTenantID(r.Context()); ok {
		return id, nil
	}

	return uuid.Nil, domain.ErrUnauthorized("tenant ID not found")
}

// validateCreateRequest validates the create project request
func (h *ProjectHandler) validateCreateRequest(req *CreateProjectRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return domain.ValidationError("name", "name is required")
	}

	if len(req.Name) > 255 {
		return domain.ValidationError("name", "name must be 255 characters or less")
	}

	if strings.TrimSpace(req.BaseURL) == "" {
		return domain.ValidationError("base_url", "base_url is required")
	}

	if err := validateURL(req.BaseURL); err != nil {
		return domain.ValidationError("base_url", err.Error())
	}

	return nil
}

// validateURL validates a URL string
func validateURL(urlStr string) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return err
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return domain.ValidationError("url", "URL must use http or https scheme")
	}

	if parsed.Host == "" {
		return domain.ValidationError("url", "URL must have a host")
	}

	return nil
}
