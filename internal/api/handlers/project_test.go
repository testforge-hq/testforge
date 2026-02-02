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

func TestProjectHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	tenantRepo := postgres.NewTenantRepository(db)
	projectRepo := postgres.NewProjectRepository(db)
	logger := zap.NewNop()
	handler := NewProjectHandler(projectRepo, tenantRepo, logger)
	ctx := context.Background()

	// Helper to create a tenant for tests
	createTestTenant := func(t *testing.T) *domain.Tenant {
		tenant := domain.NewTenant("Test Tenant", uuid.New().String()[:8], domain.PlanFree)
		err := tenantRepo.Create(ctx, tenant)
		require.NoError(t, err)
		return tenant
	}

	t.Run("Create_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		body := `{"name": "Test Project", "description": "A test project", "base_url": "https://example.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID.String()+"/projects", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Create(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "Test Project", data["name"])
		assert.Equal(t, "A test project", data["description"])
		assert.Equal(t, "https://example.com", data["base_url"])
		assert.Equal(t, tenant.ID.String(), data["tenant_id"])
	})

	t.Run("Create_ValidationError_MissingName", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		body := `{"description": "A test project", "base_url": "https://example.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID.String()+"/projects", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Create(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	})

	t.Run("Create_ValidationError_MissingBaseURL", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		body := `{"name": "Test Project", "description": "A test project"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID.String()+"/projects", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Create(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	})

	t.Run("Create_ValidationError_InvalidURL", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		body := `{"name": "Test Project", "base_url": "not-a-valid-url"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID.String()+"/projects", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Create(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	})

	t.Run("Create_TenantNotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		nonexistentID := uuid.New().String()
		body := `{"name": "Test Project", "base_url": "https://example.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+nonexistentID+"/projects", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", nonexistentID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Create(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Create_DuplicateName", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		// Create first project
		project := domain.NewProject(tenant.ID, "Duplicate Name", "First project", "https://first.example.com")
		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		// Try to create another with same name
		body := `{"name": "Duplicate Name", "base_url": "https://second.example.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID.String()+"/projects", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Create(rec, req)

		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "ALREADY_EXISTS", resp.Error.Code)
	})

	t.Run("Get_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := domain.NewProject(tenant.ID, "Get Test", "Test description", "https://get.example.com")
		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID.String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", project.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, project.ID.String(), data["id"])
		assert.Equal(t, "Get Test", data["name"])
		assert.Equal(t, "Test description", data["description"])
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+uuid.New().String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", uuid.New().String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Get_InvalidID", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/invalid-uuid", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "invalid-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("List_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		// Create multiple projects
		for i := 0; i < 5; i++ {
			project := domain.NewProject(tenant.ID, uuid.New().String()[:8], "Test project", "https://example.com")
			err := projectRepo.Create(ctx, project)
			require.NoError(t, err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+tenant.ID.String()+"/projects?page=1&per_page=3", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.List(rec, req)

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

	t.Run("List_InvalidTenantID", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/invalid-uuid/projects", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", "invalid-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.List(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Update_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := domain.NewProject(tenant.ID, "Original Name", "Original description", "https://original.example.com")
		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		body := `{"name": "Updated Name", "description": "Updated description", "base_url": "https://updated.example.com"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID.String(), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", project.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Update(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "Updated Name", data["name"])
		assert.Equal(t, "Updated description", data["description"])
		assert.Equal(t, "https://updated.example.com", data["base_url"])
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		body := `{"name": "Updated"}`
		nonexistentID := uuid.New().String()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+nonexistentID, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", nonexistentID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Update(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Update_EmptyName", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := domain.NewProject(tenant.ID, "Original", "Original description", "https://original.example.com")
		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		body := `{"name": ""}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID.String(), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", project.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Update(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Update_InvalidURL", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := domain.NewProject(tenant.ID, "URL Test", "Test description", "https://original.example.com")
		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		body := `{"base_url": "not-a-valid-url"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID.String(), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", project.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Update(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Delete_Success", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := domain.NewProject(tenant.ID, "To Delete", "Test description", "https://delete.example.com")
		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+project.ID.String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", project.ID.String())
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
		_, err = projectRepo.GetByID(ctx, project.ID)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		nonexistentID := uuid.New().String()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+nonexistentID, nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", nonexistentID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Delete(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Create_WithSettings", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		body := `{
			"name": "Project with Settings",
			"base_url": "https://settings.example.com",
			"settings": {
				"default_browser": "chromium",
				"max_crawl_depth": 5,
				"default_timeout_ms": 60000
			}
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID.String()+"/projects", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("tenant_id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Create(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		settings := data["settings"].(map[string]interface{})
		assert.Equal(t, "chromium", settings["default_browser"])
		assert.Equal(t, float64(5), settings["max_crawl_depth"])
		assert.Equal(t, float64(60000), settings["default_timeout_ms"])
	})
}
