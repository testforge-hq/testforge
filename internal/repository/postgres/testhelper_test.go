package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
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

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testforge_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
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
	// Try relative paths from test location
	candidates := []string{
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
