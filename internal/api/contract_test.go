package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contract tests validate API responses match expected schema

// TenantResponse represents the expected tenant response schema
type TenantResponseSchema struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	Plan      string            `json:"plan"`
	Settings  map[string]any    `json:"settings"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

// ProjectResponseSchema represents the expected project response schema
type ProjectResponseSchema struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	BaseURL     string         `json:"base_url"`
	Settings    map[string]any `json:"settings"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// TestRunResponseSchema represents the expected test run response schema
type TestRunResponseSchema struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenant_id"`
	ProjectID   string  `json:"project_id"`
	Status      string  `json:"status"`
	TargetURL   string  `json:"target_url"`
	TriggeredBy string  `json:"triggered_by"`
	AIEnabled   bool    `json:"ai_enabled"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	StartedAt   *string `json:"started_at,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// APIResponseSchema represents the standard API response wrapper
type APIResponseSchema struct {
	Success bool              `json:"success"`
	Data    json.RawMessage   `json:"data,omitempty"`
	Error   *APIErrorSchema   `json:"error,omitempty"`
	Meta    *APIMetaSchema    `json:"meta,omitempty"`
}

// APIErrorSchema represents the error response schema
type APIErrorSchema struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// APIMetaSchema represents pagination metadata
type APIMetaSchema struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// HealthResponseSchema represents the health endpoint response
type HealthResponseSchema struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// ReadyResponseSchema represents the ready endpoint response
type ReadyResponseSchema struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func TestContractHealthEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	router := setupTestRouter(t, testDB)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	// httputil.JSON wraps all responses in APIResponseSchema
	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err, "Response should be valid JSON matching APIResponseSchema")
	assert.True(t, apiResp.Success)

	var resp HealthResponseSchema
	err = json.Unmarshal(apiResp.Data, &resp)
	require.NoError(t, err, "Data should match HealthResponseSchema")

	// Validate required fields
	assert.NotEmpty(t, resp.Status, "status field is required")
	assert.NotEmpty(t, resp.Service, "service field is required")
	assert.Equal(t, "healthy", resp.Status)
	assert.Equal(t, "testforge-api", resp.Service)
}

func TestContractReadyEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	router := setupTestRouter(t, testDB)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// httputil.JSON wraps all responses in APIResponseSchema
	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err, "Response should be valid JSON matching APIResponseSchema")
	assert.True(t, apiResp.Success)

	var resp ReadyResponseSchema
	err = json.Unmarshal(apiResp.Data, &resp)
	require.NoError(t, err, "Data should match ReadyResponseSchema")

	// Validate required fields
	assert.NotEmpty(t, resp.Status, "status field is required")
	assert.NotNil(t, resp.Checks, "checks field is required")
}

func TestContractTenantCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	testDB.TruncateTables(t)
	router := setupTestRouter(t, testDB)

	createBody := `{"name": "Contract Test", "slug": "contract-test", "plan": "pro"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	// Validate response wrapper
	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err, "Response should be valid JSON matching APIResponseSchema")
	assert.True(t, apiResp.Success, "success should be true")
	assert.Nil(t, apiResp.Error, "error should be nil on success")

	// Validate tenant data
	var tenant TenantResponseSchema
	err = json.Unmarshal(apiResp.Data, &tenant)
	require.NoError(t, err, "Data should match TenantResponseSchema")

	// Required fields
	assert.NotEmpty(t, tenant.ID, "id is required")
	assert.NotEmpty(t, tenant.Name, "name is required")
	assert.NotEmpty(t, tenant.Slug, "slug is required")
	assert.NotEmpty(t, tenant.Plan, "plan is required")
	assert.NotNil(t, tenant.Settings, "settings is required")
	assert.NotEmpty(t, tenant.CreatedAt, "created_at is required")
	assert.NotEmpty(t, tenant.UpdatedAt, "updated_at is required")

	// UUID format validation
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, tenant.ID)

	// Timestamp format validation (ISO 8601)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, tenant.CreatedAt)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, tenant.UpdatedAt)

	// Plan enum validation
	validPlans := []string{"free", "pro", "enterprise"}
	assert.Contains(t, validPlans, tenant.Plan)
}

func TestContractTenantList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	testDB.TruncateTables(t)
	router := setupTestRouter(t, testDB)

	// Create a tenant first
	createBody := `{"name": "List Test", "slug": "list-test", "plan": "free"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// List tenants
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants?page=1&per_page=10", nil)
	// Dev mode bypasses API key auth
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Validate response wrapper with pagination
	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err)
	assert.True(t, apiResp.Success)

	// Validate meta (pagination)
	require.NotNil(t, apiResp.Meta, "meta is required for list endpoints")
	assert.GreaterOrEqual(t, apiResp.Meta.Page, 1)
	assert.GreaterOrEqual(t, apiResp.Meta.PerPage, 1)
	assert.GreaterOrEqual(t, apiResp.Meta.Total, 0)
	assert.GreaterOrEqual(t, apiResp.Meta.TotalPages, 0)

	// Validate data is an array
	var tenants []TenantResponseSchema
	err = json.Unmarshal(apiResp.Data, &tenants)
	require.NoError(t, err, "Data should be an array of TenantResponseSchema")
	assert.Len(t, tenants, 1)
}

func TestContractProjectCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	testDB.TruncateTables(t)
	router := setupTestRouter(t, testDB)

	// Create tenant first
	createTenantBody := `{"name": "Project Contract Test", "slug": "project-contract", "plan": "free"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createTenantBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var tenantResp APIResponseSchema
	json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	var tenant TenantResponseSchema
	json.Unmarshal(tenantResp.Data, &tenant)

	// Create project
	createBody := `{"name": "Contract Project", "description": "Test description", "base_url": "https://contract.example.com"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID+"/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err)
	assert.True(t, apiResp.Success)

	// Validate project data
	var project ProjectResponseSchema
	err = json.Unmarshal(apiResp.Data, &project)
	require.NoError(t, err, "Data should match ProjectResponseSchema")

	// Required fields
	assert.NotEmpty(t, project.ID, "id is required")
	assert.NotEmpty(t, project.TenantID, "tenant_id is required")
	assert.NotEmpty(t, project.Name, "name is required")
	assert.NotEmpty(t, project.BaseURL, "base_url is required")
	assert.NotNil(t, project.Settings, "settings is required")
	assert.NotEmpty(t, project.CreatedAt, "created_at is required")
	assert.NotEmpty(t, project.UpdatedAt, "updated_at is required")

	// UUID format
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, project.ID)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, project.TenantID)

	// URL format
	assert.Regexp(t, `^https?://`, project.BaseURL)
}

func TestContractTestRunCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	testDB.TruncateTables(t)
	router := setupTestRouter(t, testDB)

	// Create tenant
	createTenantBody := `{"name": "Run Contract Test", "slug": "run-contract", "plan": "pro"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createTenantBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var tenantResp APIResponseSchema
	json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	var tenant TenantResponseSchema
	json.Unmarshal(tenantResp.Data, &tenant)

	// Create project
	createProjectBody := `{"name": "Run Contract Project", "base_url": "https://run-contract.example.com"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenant.ID+"/projects", bytes.NewBufferString(createProjectBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var projectResp APIResponseSchema
	json.Unmarshal(rec.Body.Bytes(), &projectResp)
	var project ProjectResponseSchema
	json.Unmarshal(projectResp.Data, &project)

	// Create test run
	createBody := `{"project_id": "` + project.ID + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err)
	assert.True(t, apiResp.Success)

	// Validate test run data
	var testRun TestRunResponseSchema
	err = json.Unmarshal(apiResp.Data, &testRun)
	require.NoError(t, err, "Data should match TestRunResponseSchema")

	// Required fields
	assert.NotEmpty(t, testRun.ID, "id is required")
	assert.NotEmpty(t, testRun.TenantID, "tenant_id is required")
	assert.NotEmpty(t, testRun.ProjectID, "project_id is required")
	assert.NotEmpty(t, testRun.Status, "status is required")
	assert.NotEmpty(t, testRun.TargetURL, "target_url is required")
	assert.NotEmpty(t, testRun.TriggeredBy, "triggered_by is required")
	assert.NotEmpty(t, testRun.CreatedAt, "created_at is required")
	assert.NotEmpty(t, testRun.UpdatedAt, "updated_at is required")

	// Status enum validation
	validStatuses := []string{"pending", "discovering", "designing", "automating", "executing", "healing", "reporting", "completed", "failed", "cancelled"}
	assert.Contains(t, validStatuses, testRun.Status)
}

func TestContractErrorResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	testDB.TruncateTables(t)
	router := setupTestRouter(t, testDB)

	// Request with missing required field
	createBody := `{"slug": "missing-name"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err)

	// Validate error response
	assert.False(t, apiResp.Success, "success should be false on error")
	require.NotNil(t, apiResp.Error, "error is required on error response")
	assert.NotEmpty(t, apiResp.Error.Code, "error.code is required")
	assert.NotEmpty(t, apiResp.Error.Message, "error.message is required")
}

func TestContractNotFoundResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	testDB.TruncateTables(t)
	router := setupTestRouter(t, testDB)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/00000000-0000-0000-0000-000000000000", nil)
	// Dev mode bypasses API key auth
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err)

	assert.False(t, apiResp.Success)
	require.NotNil(t, apiResp.Error)
	assert.Equal(t, "NOT_FOUND", apiResp.Error.Code)
}

func TestContractConflictResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)
	testDB.TruncateTables(t)
	router := setupTestRouter(t, testDB)

	// Create first tenant
	createBody := `{"name": "First", "slug": "conflict-slug", "plan": "free"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Try duplicate
	createBody = `{"name": "Second", "slug": "conflict-slug", "plan": "free"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	// Dev mode bypasses API key auth
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var apiResp APIResponseSchema
	err := json.Unmarshal(rec.Body.Bytes(), &apiResp)
	require.NoError(t, err)

	assert.False(t, apiResp.Success)
	require.NotNil(t, apiResp.Error)
	assert.Equal(t, "ALREADY_EXISTS", apiResp.Error.Code)
}
