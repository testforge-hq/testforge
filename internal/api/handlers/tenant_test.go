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

func TestTenantHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	repo := postgres.NewTenantRepository(db)
	logger := zap.NewNop()
	handler := NewTenantHandler(repo, nil, logger)
	ctx := context.Background()

	t.Run("Create_Success", func(t *testing.T) {
		testDB.TruncateTables(t)

		body := `{"name": "Test Tenant", "slug": "test-tenant", "plan": "free"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "Test Tenant", data["name"])
		assert.Equal(t, "test-tenant", data["slug"])
		assert.Equal(t, "free", data["plan"])
	})

	t.Run("Create_ValidationError_MissingName", func(t *testing.T) {
		testDB.TruncateTables(t)

		body := `{"slug": "test-tenant", "plan": "free"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	})

	t.Run("Create_ValidationError_InvalidSlug", func(t *testing.T) {
		testDB.TruncateTables(t)

		body := `{"name": "Test", "slug": "AB", "plan": "free"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	})

	t.Run("Create_DuplicateSlug", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create first tenant
		tenant := domain.NewTenant("First", "duplicate-slug", domain.PlanFree)
		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		// Try to create another with same slug
		body := `{"name": "Second", "slug": "duplicate-slug", "plan": "free"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

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

		// Create a tenant
		tenant := domain.NewTenant("Get Test", "get-test", domain.PlanPro)
		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		// Create request with chi URL param
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+tenant.ID.String(), nil)
		rec := httptest.NewRecorder()

		// Use chi context for URL params
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, tenant.ID.String(), data["id"])
		assert.Equal(t, "Get Test", data["name"])
		assert.Equal(t, "pro", data["plan"])
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+uuid.New().String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", uuid.New().String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Get_InvalidID", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/invalid-uuid", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "invalid-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Get(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("GetBySlug_Success", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := domain.NewTenant("Slug Test", "slug-test", domain.PlanEnterprise)
		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/slug/slug-test", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("slug", "slug-test")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.GetBySlug(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "slug-test", data["slug"])
	})

	t.Run("GetBySlug_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/slug/nonexistent", nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("slug", "nonexistent")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.GetBySlug(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("List_Success", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create multiple tenants
		for i := 0; i < 5; i++ {
			tenant := domain.NewTenant("Tenant", uuid.New().String()[:8], domain.PlanFree)
			err := repo.Create(ctx, tenant)
			require.NoError(t, err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants?page=1&per_page=3", nil)
		rec := httptest.NewRecorder()

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

	t.Run("Update_Success", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := domain.NewTenant("Original Name", "update-test", domain.PlanFree)
		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		body := `{"name": "Updated Name", "plan": "pro"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/"+tenant.ID.String(), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Update(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "Updated Name", data["name"])
		assert.Equal(t, "pro", data["plan"])
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		body := `{"name": "Updated"}`
		nonexistentID := uuid.New().String()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/"+nonexistentID, bytes.NewBufferString(body))
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

		tenant := domain.NewTenant("Original", "empty-name-test", domain.PlanFree)
		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		body := `{"name": ""}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/"+tenant.ID.String(), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Update(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Update_InvalidPlan", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := domain.NewTenant("Plan Test", "plan-test", domain.PlanFree)
		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		body := `{"plan": "invalid"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/"+tenant.ID.String(), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", tenant.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Update(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Delete_Success", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := domain.NewTenant("To Delete", "delete-test", domain.PlanFree)
		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/"+tenant.ID.String(), nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", tenant.ID.String())
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
		_, err = repo.GetByID(ctx, tenant.ID)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		nonexistentID := uuid.New().String()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/"+nonexistentID, nil)
		rec := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", nonexistentID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.Delete(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Create_DefaultPlan", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create without specifying plan
		body := `{"name": "No Plan", "slug": "no-plan-test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "free", data["plan"]) // Should default to free
	})
}
