package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testforge/testforge/internal/domain"
	"github.com/testforge/testforge/internal/repository/postgres"
	"github.com/testforge/testforge/pkg/httputil"
	"go.uber.org/zap"
)

func TestTestRunHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	tenantRepo := postgres.NewTenantRepository(db)
	projectRepo := postgres.NewProjectRepository(db)
	testRunRepo := postgres.NewTestRunRepository(db)
	logger := zap.NewNop()

	// Create handler without Temporal client for testing basic operations
	handler := NewTestRunHandler(testRunRepo, projectRepo, tenantRepo, nil, "test-queue", logger)
	ctx := context.Background()

	// Helper to create a tenant for tests
	createTestTenant := func(t *testing.T) *domain.Tenant {
		tenant := domain.NewTenant("Test Tenant", uuid.New().String()[:8], domain.PlanFree)
		tenant.Settings.MaxConcurrentRuns = 5
		err := tenantRepo.Create(ctx, tenant)
		require.NoError(t, err)
		return tenant
	}

	// Helper to create a project for tests
	createTestProject := func(t *testing.T, tenant *domain.Tenant) *domain.Project {
		project := domain.NewProject(tenant.ID, "Test Project", "A test project", "https://example.com")
		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)
		return project
	}

	t.Run("Create_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		body := `{"project_id": "` + project.ID.String() + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, tenant.ID.String(), data["tenant_id"])
		assert.Equal(t, project.ID.String(), data["project_id"])
		assert.Equal(t, "https://example.com", data["target_url"])
		assert.Equal(t, "pending", data["status"])
		assert.Equal(t, "api", data["triggered_by"])
	})

	t.Run("Create_WithCustomTargetURL", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		body := `{"project_id": "` + project.ID.String() + `", "target_url": "https://custom.example.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "https://custom.example.com", data["target_url"])
	})

	t.Run("Create_InvalidProjectID", func(t *testing.T) {
		testDB.TruncateTables(t)

		body := `{"project_id": "invalid-uuid"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Create_ProjectNotFound", func(t *testing.T) {
		testDB.TruncateTables(t)
		createTestTenant(t)

		body := `{"project_id": "` + uuid.New().String() + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Create_InvalidTargetURL", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		body := `{"project_id": "` + project.ID.String() + `", "target_url": "not-a-valid-url"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Create_WithAIEnabled", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		body := `{"project_id": "` + project.ID.String() + `", "enable_ai_discovery": true}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.True(t, data["ai_enabled"].(bool))
	})

	t.Run("Get_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://get.example.com", "test")
		err := testRunRepo.Create(ctx, run)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+run.ID.String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", run.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, run.ID.String(), data["id"])
		assert.Equal(t, "https://get.example.com", data["target_url"])
		assert.Equal(t, "test", data["triggered_by"])
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+uuid.New().String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", uuid.New().String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Get_InvalidID", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/invalid-uuid", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "invalid-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ListByProject_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		// Create multiple test runs
		for i := 0; i < 5; i++ {
			run := domain.NewTestRun(tenant.ID, project.ID, "https://list.example.com", "test")
			err := testRunRepo.Create(ctx, run)
			require.NoError(t, err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID.String()+"/runs?page=1&per_page=3", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("project_id", project.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.ListByProject(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Meta)
		assert.Equal(t, 5, resp.Meta.Total)
		assert.Equal(t, 3, resp.Meta.PerPage)

		data := resp.Data.([]interface{})
		assert.Len(t, data, 3)
	})

	t.Run("ListByProject_InvalidProjectID", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/invalid-uuid/runs", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("project_id", "invalid-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.ListByProject(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ListByProject_ProjectNotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+uuid.New().String()+"/runs", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("project_id", uuid.New().String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.ListByProject(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Cancel_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://cancel.example.com", "test")
		run.Status = domain.RunStatusDiscovering // Active status
		err := testRunRepo.Create(ctx, run)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+run.ID.String()+"/cancel", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", run.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Cancel(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "cancelled", data["status"])
	})

	t.Run("Cancel_AlreadyTerminal", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://cancel.example.com", "test")
		run.Status = domain.RunStatusCompleted // Already terminal
		err := testRunRepo.Create(ctx, run)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+run.ID.String()+"/cancel", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", run.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Cancel(rec, req)

		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "INVALID_STATE", resp.Error.Code)
	})

	t.Run("Cancel_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+uuid.New().String()+"/cancel", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", uuid.New().String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Cancel(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Delete_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://delete.example.com", "test")
		run.Status = domain.RunStatusCompleted // Must be terminal to delete
		err := testRunRepo.Create(ctx, run)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/runs/"+run.ID.String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", run.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Delete(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.True(t, data["deleted"].(bool))

		// Verify it's deleted
		_, err = testRunRepo.GetByID(ctx, run.ID)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete_ActiveRun", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		project := createTestProject(t, tenant)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://delete.example.com", "test")
		run.Status = domain.RunStatusDiscovering // Active status
		err := testRunRepo.Create(ctx, run)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/runs/"+run.ID.String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", run.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Delete(rec, req)

		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "INVALID_STATE", resp.Error.Code)
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/runs/"+uuid.New().String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", uuid.New().String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Delete(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Create_QuotaExceeded", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)
		tenant.Settings.MaxConcurrentRuns = 2 // Set low limit
		err := tenantRepo.Update(ctx, tenant)
		require.NoError(t, err)

		project := createTestProject(t, tenant)

		// Create runs up to the limit
		for i := 0; i < 2; i++ {
			run := domain.NewTestRun(tenant.ID, project.ID, "https://quota.example.com", "test")
			run.Status = domain.RunStatusDiscovering // Active status
			err := testRunRepo.Create(ctx, run)
			require.NoError(t, err)
		}

		// Try to create one more
		body := `{"project_id": "` + project.ID.String() + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusTooManyRequests, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "QUOTA_EXCEEDED", resp.Error.Code)
	})
}
