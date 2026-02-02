package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/testforge/testforge/internal/config"
)

// DB wraps sqlx.DB with additional functionality
type DB struct {
	*sqlx.DB
}

// New creates a new database connection
func New(cfg config.DatabaseConfig) (*DB, error) {
	db, err := sqlx.Connect("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &DB{DB: db}, nil
}

// NewFromDSN creates a new database connection from a DSN string
func NewFromDSN(dsn string) (*DB, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	return &DB{DB: db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// Health checks database connectivity
func (db *DB) Health(ctx context.Context) error {
	return db.PingContext(ctx)
}

// isUniqueViolation checks if the error is a unique constraint violation
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

// isForeignKeyViolation checks if the error is a foreign key violation
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "violates foreign key constraint")
}

// Transaction executes a function within a transaction
func (db *DB) Transaction(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rolling back transaction: %w (original error: %v)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// Repositories holds all repository instances
type Repositories struct {
	Tenants   *TenantRepository
	Projects  *ProjectRepository
	TestRuns  *TestRunRepository
	TestCases *TestCaseRepository
	Reports   *ReportRepository
}

// NewRepositories creates all repository instances
func NewRepositories(db *sqlx.DB) *Repositories {
	return &Repositories{
		Tenants:   NewTenantRepository(db),
		Projects:  NewProjectRepository(db),
		TestRuns:  NewTestRunRepository(db),
		TestCases: NewTestCaseRepository(db),
		Reports:   NewReportRepository(db),
	}
}
