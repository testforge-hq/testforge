package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/testforge/testforge/internal/repository/postgres"
	"github.com/testforge/testforge/pkg/httputil"
	"go.uber.org/zap"
)

// TestDB holds the test database connection and container
type TestDB struct {
	Container testcontainers.Container
	DB        *sql.DB
	ConnStr   string
}

// SetupTestDB creates a PostgreSQL container for testing
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testforge_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Failed to start postgres container: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get connection string: %v", err)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Wait for DB to be ready
	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	testDB := &TestDB{
		Container: container,
		DB:        db,
		ConnStr:   connStr,
	}

	// Run migrations
	if err := testDB.RunMigrations(t); err != nil {
		testDB.Cleanup(t)
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return testDB
}

// RunMigrations applies all SQL migrations
func (td *TestDB) RunMigrations(t *testing.T) error {
	t.Helper()

	// Find migrations directory
	migrationsDir := findMigrationsDir()
	if migrationsDir == "" {
		return fmt.Errorf("migrations directory not found")
	}

	// Get all migration files
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return fmt.Errorf("failed to list migrations: %w", err)
	}

	// Sort to ensure order
	sort.Strings(files)

	// Apply each migration
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", file, err)
		}

		_, err = td.DB.Exec(string(content))
		if err != nil {
			// Log but continue - some statements may fail if already applied
			t.Logf("Warning applying %s: %v", filepath.Base(file), err)
		}
	}

	return nil
}

// findMigrationsDir locates the migrations directory
func findMigrationsDir() string {
	candidates := []string{
		"../../migrations",
		"../../../migrations",
		"../../../../migrations",
		"migrations",
	}

	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	return ""
}

// Cleanup terminates the container and closes connections
func (td *TestDB) Cleanup(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	if td.DB != nil {
		td.DB.Close()
	}
	if td.Container != nil {
		td.Container.Terminate(ctx)
	}
}

// TruncateTables clears all data from tables for test isolation
func (td *TestDB) TruncateTables(t *testing.T) {
	t.Helper()

	tables := []string{
		"test_cases",
		"reports",
		"test_runs",
		"api_keys",
		"projects",
		"tenants",
	}

	for _, table := range tables {
		_, err := td.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Logf("Warning truncating %s: %v", table, err)
		}
	}
}

// setupTestRouter creates a router with test database
func setupTestRouter(t *testing.T, testDB *TestDB) *Router {
	// Enable dev mode for auth bypass
	os.Setenv("TESTFORGE_DEV_MODE", "true")

	db := sqlx.NewDb(testDB.DB, "postgres")
	// Use NewRepositories to properly initialize the db field for health checks
	repos := postgres.NewRepositories(db)

	logger := zap.NewNop()

	return NewRouter(RouterConfig{
		Repos:       repos,
		Logger:      logger,
		Development: true,
	})
}

func TestAPIIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	router := setupTestRouter(t, testDB)

	t.Run("HealthEndpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// httputil.JSON wraps data in Response struct
		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "healthy", data["status"])
		assert.Equal(t, "testforge-api", data["service"])
	})

	t.Run("ReadyEndpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// httputil.JSON wraps data in Response struct
		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "ready", data["status"])
	})

	t.Run("TenantCRUD", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create tenant
		createBody := `{"name": "API Test Tenant", "slug": "api-test", "plan": "pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var createResp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &createResp)
		require.NoError(t, err)
		assert.True(t, createResp.Success)

		data := createResp.Data.(map[string]interface{})
		tenantID := data["id"].(string)
		assert.Equal(t, "API Test Tenant", data["name"])
		assert.Equal(t, "api-test", data["slug"])
		assert.Equal(t, "pro", data["plan"])

		// Get tenant by ID
		req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+tenantID, nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var getResp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &getResp)
		require.NoError(t, err)
		assert.True(t, getResp.Success)

		// Get tenant by slug
		req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/slug/api-test", nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Update tenant
		updateBody := `{"name": "Updated API Tenant", "plan": "enterprise"}`
		req = httptest.NewRequest(http.MethodPut, "/api/v1/tenants/"+tenantID, bytes.NewBufferString(updateBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var updateResp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &updateResp)
		require.NoError(t, err)
		data = updateResp.Data.(map[string]interface{})
		assert.Equal(t, "Updated API Tenant", data["name"])
		assert.Equal(t, "enterprise", data["plan"])

		// List tenants
		req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants?page=1&per_page=10", nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var listResp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &listResp)
		require.NoError(t, err)
		assert.NotNil(t, listResp.Meta)
		assert.Equal(t, 1, listResp.Meta.Total)

		// Delete tenant
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/"+tenantID, nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify deleted
		req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+tenantID, nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("ProjectCRUD", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create tenant first
		createTenantBody := `{"name": "Project Test Tenant", "slug": "project-test", "plan": "free"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createTenantBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var tenantResp httputil.Response
		json.Unmarshal(rec.Body.Bytes(), &tenantResp)
		tenantID := tenantResp.Data.(map[string]interface{})["id"].(string)

		// Create project
		createBody := `{"name": "Test Project", "description": "A test project", "base_url": "https://example.com"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenantID+"/projects", bytes.NewBufferString(createBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var createResp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &createResp)
		require.NoError(t, err)
		assert.True(t, createResp.Success)

		data := createResp.Data.(map[string]interface{})
		projectID := data["id"].(string)
		assert.Equal(t, "Test Project", data["name"])
		assert.Equal(t, "https://example.com", data["base_url"])

		// Get project
		req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID, nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Update project
		updateBody := `{"name": "Updated Project", "base_url": "https://updated.example.com"}`
		req = httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID, bytes.NewBufferString(updateBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// List projects
		req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+tenantID+"/projects", nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Delete project
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectID, nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("TestRunCRUD", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create tenant
		createTenantBody := `{"name": "Run Test Tenant", "slug": "run-test", "plan": "pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createTenantBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var tenantResp httputil.Response
		json.Unmarshal(rec.Body.Bytes(), &tenantResp)
		tenantID := tenantResp.Data.(map[string]interface{})["id"].(string)

		// Create project
		createProjectBody := `{"name": "Run Test Project", "base_url": "https://run.example.com"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/"+tenantID+"/projects", bytes.NewBufferString(createProjectBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var projectResp httputil.Response
		json.Unmarshal(rec.Body.Bytes(), &projectResp)
		projectID := projectResp.Data.(map[string]interface{})["id"].(string)

		// Create test run
		createBody := `{"project_id": "` + projectID + `"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewBufferString(createBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var createResp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &createResp)
		require.NoError(t, err)
		assert.True(t, createResp.Success)

		data := createResp.Data.(map[string]interface{})
		runID := data["id"].(string)
		assert.Equal(t, projectID, data["project_id"])
		assert.Equal(t, "pending", data["status"])

		// Get test run
		req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+runID, nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// List test runs by project
		req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/runs", nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var listResp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &listResp)
		require.NoError(t, err)
		assert.NotNil(t, listResp.Meta)
		assert.Equal(t, 1, listResp.Meta.Total)

		// Cancel test run
		req = httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+runID+"/cancel", nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var cancelResp httputil.Response
		err = json.Unmarshal(rec.Body.Bytes(), &cancelResp)
		require.NoError(t, err)
		data = cancelResp.Data.(map[string]interface{})
		assert.Equal(t, "cancelled", data["status"])

		// Delete test run (now that it's cancelled/terminal)
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/runs/"+runID, nil)
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Missing required field
		createBody := `{"slug": "no-name"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	})

	t.Run("NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/00000000-0000-0000-0000-000000000000", nil)
		// Dev mode bypasses API key auth
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "NOT_FOUND", resp.Error.Code)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		testDB.TruncateTables(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/invalid-uuid", nil)
		// Dev mode bypasses API key auth
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("DuplicateSlug", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create first tenant
		createBody := `{"name": "First Tenant", "slug": "duplicate-slug", "plan": "free"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		// Try to create second with same slug
		createBody = `{"name": "Second Tenant", "slug": "duplicate-slug", "plan": "free"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(createBody))
		req.Header.Set("Content-Type", "application/json")
		// Dev mode bypasses API key auth
		rec = httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp httputil.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "ALREADY_EXISTS", resp.Error.Code)
	})
}
