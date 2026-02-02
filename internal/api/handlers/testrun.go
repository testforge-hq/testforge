package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/api/middleware"
	"github.com/testforge/testforge/internal/domain"
	"github.com/testforge/testforge/internal/repository/postgres"
	"github.com/testforge/testforge/internal/workflows"
	"github.com/testforge/testforge/pkg/httputil"
)

// TestRunHandler handles test run related requests
type TestRunHandler struct {
	repo           *postgres.TestRunRepository
	projectRepo    *postgres.ProjectRepository
	tenantRepo     *postgres.TenantRepository
	temporalClient client.Client
	taskQueue      string
	logger         *zap.Logger
}

// NewTestRunHandler creates a new test run handler
func NewTestRunHandler(
	repo *postgres.TestRunRepository,
	projectRepo *postgres.ProjectRepository,
	tenantRepo *postgres.TenantRepository,
	temporalClient client.Client,
	taskQueue string,
	logger *zap.Logger,
) *TestRunHandler {
	return &TestRunHandler{
		repo:           repo,
		projectRepo:    projectRepo,
		tenantRepo:     tenantRepo,
		temporalClient: temporalClient,
		taskQueue:      taskQueue,
		logger:         logger,
	}
}

// TestRunResponse is the API representation of a test run
type TestRunResponse struct {
	ID              string                    `json:"id"`
	TenantID        string                    `json:"tenant_id"`
	ProjectID       string                    `json:"project_id"`
	Status          string                    `json:"status"`
	TargetURL       string                    `json:"target_url"`
	WorkflowID      string                    `json:"workflow_id,omitempty"`
	WorkflowStatus  string                    `json:"workflow_status,omitempty"`
	DiscoveryResult *domain.DiscoveryResult   `json:"discovery_result,omitempty"`
	AIAnalysis      *domain.AIAnalysisResult  `json:"ai_analysis,omitempty"`
	TestPlan        *domain.TestPlan          `json:"test_plan,omitempty"`
	Summary         *domain.RunSummary        `json:"summary,omitempty"`
	ReportURL       string                    `json:"report_url,omitempty"`
	TriggeredBy     string                    `json:"triggered_by"`
	AIEnabled       bool                      `json:"ai_enabled"`
	StartedAt       *string                   `json:"started_at,omitempty"`
	CompletedAt     *string                   `json:"completed_at,omitempty"`
	CreatedAt       string                    `json:"created_at"`
	UpdatedAt       string                    `json:"updated_at"`
}

func toTestRunResponse(r *domain.TestRun) TestRunResponse {
	resp := TestRunResponse{
		ID:              r.ID.String(),
		TenantID:        r.TenantID.String(),
		ProjectID:       r.ProjectID.String(),
		Status:          string(r.Status),
		TargetURL:       r.TargetURL,
		WorkflowID:      r.WorkflowID,
		DiscoveryResult: r.DiscoveryResult,
		AIAnalysis:      r.AIAnalysis,
		TestPlan:        r.TestPlan,
		Summary:         r.Summary,
		ReportURL:       r.ReportURL,
		TriggeredBy:     r.TriggeredBy,
		AIEnabled:       r.AIEnabled,
		CreatedAt:       r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       r.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if r.StartedAt != nil {
		s := r.StartedAt.Format("2006-01-02T15:04:05Z")
		resp.StartedAt = &s
	}

	if r.CompletedAt != nil {
		s := r.CompletedAt.Format("2006-01-02T15:04:05Z")
		resp.CompletedAt = &s
	}

	return resp
}

// ListByProject handles GET /api/v1/projects/{project_id}/runs
func (h *TestRunHandler) ListByProject(w http.ResponseWriter, r *http.Request) {
	projectIDStr := chi.URLParam(r, "project_id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid project ID format", nil)
		return
	}

	// Verify project exists and belongs to tenant
	project, err := h.projectRepo.GetByID(r.Context(), projectID)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	if tenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if project.TenantID != tenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	pagination := httputil.GetPagination(r, 20, 100)

	runs, total, err := h.repo.GetByProjectID(r.Context(), projectID, pagination.PerPage, pagination.Offset)
	if err != nil {
		h.logger.Error("Failed to list test runs", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	response := make([]TestRunResponse, len(runs))
	for i, run := range runs {
		response[i] = toTestRunResponse(run)
		// Enrich with workflow status if available
		if run.WorkflowID != "" && h.temporalClient != nil {
			response[i].WorkflowStatus = h.getWorkflowStatus(r.Context(), run.WorkflowID, run.WorkflowRunID)
		}
	}

	httputil.JSONWithMeta(w, http.StatusOK, response, &httputil.Meta{
		Page:       pagination.Page,
		PerPage:    pagination.PerPage,
		Total:      total,
		TotalPages: httputil.CalculateTotalPages(total, pagination.PerPage),
	})
}

// Get handles GET /api/v1/runs/{id}
func (h *TestRunHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid test run ID format", nil)
		return
	}

	run, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check tenant access
	if tenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if run.TenantID != tenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	response := toTestRunResponse(run)

	// Enrich with workflow status if available
	if run.WorkflowID != "" && h.temporalClient != nil {
		response.WorkflowStatus = h.getWorkflowStatus(r.Context(), run.WorkflowID, run.WorkflowRunID)
	}

	httputil.JSON(w, http.StatusOK, response)
}

// CreateTestRunRequest is the request body for creating a test run
type CreateTestRunRequest struct {
	ProjectID         string `json:"project_id"`
	TargetURL         string `json:"target_url,omitempty"`
	EnableAIDiscovery bool   `json:"enable_ai_discovery,omitempty"` // Use AI-powered multi-agent discovery
	EnableABA         bool   `json:"enable_aba,omitempty"`          // Enable Autonomous Business Analyst
}

// Create handles POST /api/v1/runs
func (h *TestRunHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTestRunRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid project ID format", nil)
		return
	}

	// Get project
	project, err := h.projectRepo.GetByID(r.Context(), projectID)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check tenant access
	tenantID := project.TenantID
	if ctxTenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if project.TenantID != ctxTenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
		tenantID = ctxTenantID
	}

	// Get tenant for quota check
	tenant, err := h.tenantRepo.GetByID(r.Context(), tenantID)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check concurrent run quota
	activeCount, err := h.repo.CountActiveByTenant(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to count active runs", zap.Error(err))
		httputil.JSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create test run", nil)
		return
	}

	if activeCount >= tenant.Settings.MaxConcurrentRuns {
		httputil.JSONError(w, http.StatusTooManyRequests, "QUOTA_EXCEEDED",
			"Maximum concurrent runs exceeded", map[string]any{
				"limit":   tenant.Settings.MaxConcurrentRuns,
				"current": activeCount,
			})
		return
	}

	// Determine target URL
	targetURL := req.TargetURL
	if targetURL == "" {
		targetURL = project.BaseURL
	}

	// Validate target URL
	if err := validateURL(targetURL); err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid target URL", nil)
		return
	}

	// Create test run
	triggeredBy := "api"
	if userID, ok := middleware.GetUserID(r.Context()); ok {
		triggeredBy = "user:" + userID
	}

	// Determine AI settings - request overrides project defaults
	enableAI := req.EnableAIDiscovery || project.Settings.EnableAIDiscovery
	enableABA := req.EnableABA || project.Settings.EnableABA

	run := domain.NewTestRun(tenantID, projectID, targetURL, triggeredBy)
	run.AIEnabled = enableAI

	if err := h.repo.Create(r.Context(), run); err != nil {
		h.logger.Error("Failed to create test run", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Start Temporal workflow if client is available
	if h.temporalClient != nil {
		workflowID := fmt.Sprintf("testrun-%s", run.ID.String())

		workflowInput := workflows.TestRunInput{
			TestRunID:          run.ID,
			TenantID:           tenantID,
			ProjectID:          projectID,
			TargetURL:          targetURL,
			TriggeredBy:        triggeredBy,
			MaxCrawlDepth:      project.Settings.MaxCrawlDepth,
			MaxTestCases:       tenant.Settings.MaxTestCasesPerRun,
			EnableSelfHealing:  tenant.Settings.EnableSelfHealing,
			EnableVisualTesting: tenant.Settings.EnableVisualTesting,
			EnableAIDiscovery:  enableAI,
			EnableABA:          enableABA,
			Browser:            project.Settings.DefaultBrowser,
			ViewportWidth:      project.Settings.ViewportWidth,
			ViewportHeight:     project.Settings.ViewportHeight,
			Timeout:            project.Settings.DefaultTimeout,
		}

		workflowOptions := client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: h.taskQueue,
		}

		// Choose workflow based on AI mode
		var workflowRun client.WorkflowRun
		if enableAI {
			workflowRun, err = h.temporalClient.ExecuteWorkflow(r.Context(), workflowOptions, workflows.AIEnhancedOrchestrationWorkflow, workflowInput)
		} else {
			workflowRun, err = h.temporalClient.ExecuteWorkflow(r.Context(), workflowOptions, workflows.MasterOrchestrationWorkflow, workflowInput)
		}
		if err != nil {
			h.logger.Error("Failed to start workflow", zap.Error(err))
			// Don't fail the request, just log the error
			// The test run is created, we can retry starting the workflow later
		} else {
			// Update test run with workflow info
			run.SetWorkflowInfo(workflowID, workflowRun.GetRunID())
			run.Status = domain.RunStatusDiscovering
			run.Start()

			if err := h.repo.Update(r.Context(), run); err != nil {
				h.logger.Error("Failed to update test run with workflow info", zap.Error(err))
			}

			h.logger.Info("Workflow started",
				zap.String("workflow_id", workflowID),
				zap.String("run_id", workflowRun.GetRunID()),
			)
		}
	}

	h.logger.Info("Test run created",
		zap.String("run_id", run.ID.String()),
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", run.WorkflowID),
		zap.Bool("ai_enabled", enableAI),
		zap.Bool("aba_enabled", enableABA),
	)

	httputil.JSON(w, http.StatusCreated, toTestRunResponse(run))
}

// Cancel handles POST /api/v1/runs/{id}/cancel
func (h *TestRunHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid test run ID format", nil)
		return
	}

	run, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check tenant access
	if tenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if run.TenantID != tenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	// Check if already in terminal state
	if run.Status.IsTerminal() {
		httputil.JSONError(w, http.StatusConflict, "INVALID_STATE",
			"Test run is already in terminal state", map[string]any{
				"status": string(run.Status),
			})
		return
	}

	// Cancel Temporal workflow if running
	if run.WorkflowID != "" && h.temporalClient != nil {
		if err := h.temporalClient.CancelWorkflow(r.Context(), run.WorkflowID, run.WorkflowRunID); err != nil {
			h.logger.Warn("Failed to cancel workflow", zap.Error(err), zap.String("workflow_id", run.WorkflowID))
		}
	}

	// Update status
	if err := h.repo.UpdateStatus(r.Context(), id, domain.RunStatusCancelled); err != nil {
		h.logger.Error("Failed to cancel test run", zap.Error(err))
		httputil.ErrorFromDomain(w, err)
		return
	}

	h.logger.Info("Test run cancelled", zap.String("run_id", id.String()))

	run.Status = domain.RunStatusCancelled
	httputil.JSON(w, http.StatusOK, toTestRunResponse(run))
}

// Delete handles DELETE /api/v1/runs/{id}
func (h *TestRunHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid test run ID format", nil)
		return
	}

	run, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	// Check tenant access
	if tenantID, ok := middleware.GetTenantID(r.Context()); ok {
		if run.TenantID != tenantID {
			httputil.JSONError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	// Only allow deletion of terminal runs
	if !run.Status.IsTerminal() {
		httputil.JSONError(w, http.StatusConflict, "INVALID_STATE",
			"Cannot delete active test run. Cancel it first.", map[string]any{
				"status": string(run.Status),
			})
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		httputil.ErrorFromDomain(w, err)
		return
	}

	h.logger.Info("Test run deleted", zap.String("run_id", id.String()))

	httputil.JSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// getWorkflowStatus fetches the current workflow status from Temporal
func (h *TestRunHandler) getWorkflowStatus(ctx context.Context, workflowID, runID string) string {
	if h.temporalClient == nil {
		return ""
	}

	desc, err := h.temporalClient.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err != nil {
		h.logger.Debug("Failed to describe workflow", zap.Error(err), zap.String("workflow_id", workflowID))
		return "unknown"
	}

	return desc.WorkflowExecutionInfo.Status.String()
}
