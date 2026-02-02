package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/testforge/testforge/internal/domain"
)

// TestCaseRepository implements domain.TestCaseRepository with PostgreSQL
type TestCaseRepository struct {
	db *sqlx.DB
}

// NewTestCaseRepository creates a new test case repository
func NewTestCaseRepository(db *sqlx.DB) *TestCaseRepository {
	return &TestCaseRepository{db: db}
}

// testCaseRow represents the database row structure
type testCaseRow struct {
	ID              uuid.UUID  `db:"id"`
	TestRunID       uuid.UUID  `db:"test_run_id"`
	TenantID        uuid.UUID  `db:"tenant_id"`
	Name            string     `db:"name"`
	Description     string     `db:"description"`
	Category        string     `db:"category"`
	Priority        string     `db:"priority"`
	Status          string     `db:"status"`
	Steps           []byte     `db:"steps"`
	Script          string     `db:"script"`
	OriginalScript  *string    `db:"original_script"`
	ExecutionResult []byte     `db:"execution_result"`
	HealingHistory  []byte     `db:"healing_history"`
	RetryCount      int        `db:"retry_count"`
	DurationMs      int64      `db:"duration_ms"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	DeletedAt       *time.Time `db:"deleted_at"`
}

func (r *testCaseRow) toDomain() (*domain.TestCase, error) {
	tc := &domain.TestCase{
		ID:          r.ID,
		TestRunID:   r.TestRunID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Description: r.Description,
		Category:    r.Category,
		Priority:    domain.Priority(r.Priority),
		Status:      domain.TestCaseStatus(r.Status),
		Script:      r.Script,
		RetryCount:  r.RetryCount,
		Duration:    time.Duration(r.DurationMs) * time.Millisecond,
		Timestamps: domain.Timestamps{
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
			DeletedAt: r.DeletedAt,
		},
	}

	if r.OriginalScript != nil {
		tc.OriginalScript = *r.OriginalScript
	}

	if r.Steps != nil {
		if err := json.Unmarshal(r.Steps, &tc.Steps); err != nil {
			return nil, err
		}
	}

	if r.ExecutionResult != nil {
		var result domain.ExecutionResult
		if err := json.Unmarshal(r.ExecutionResult, &result); err != nil {
			return nil, err
		}
		tc.ExecutionResult = &result
	}

	if r.HealingHistory != nil {
		if err := json.Unmarshal(r.HealingHistory, &tc.HealingHistory); err != nil {
			return nil, err
		}
	}

	return tc, nil
}

// Create inserts a new test case
func (r *TestCaseRepository) Create(ctx context.Context, tc *domain.TestCase) error {
	// Handle nil slices by defaulting to empty arrays for JSONB columns
	steps := tc.Steps
	if steps == nil {
		steps = []domain.TestStep{}
	}
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return err
	}

	// executionResult can be null in the DB
	var executionResult interface{} = nil
	if tc.ExecutionResult != nil {
		executionResult, err = json.Marshal(tc.ExecutionResult)
		if err != nil {
			return err
		}
	}

	// healing_history defaults to empty array
	healingHistory := tc.HealingHistory
	if healingHistory == nil {
		healingHistory = []domain.HealingRecord{}
	}
	healingHistoryJSON, err := json.Marshal(healingHistory)
	if err != nil {
		return err
	}

	var originalScript *string
	if tc.OriginalScript != "" {
		originalScript = &tc.OriginalScript
	}

	query := `
		INSERT INTO test_cases (
			id, test_run_id, tenant_id, name, description, category, priority, status,
			steps, script, original_script, execution_result, healing_history,
			retry_count, duration_ms, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	_, err = r.db.ExecContext(ctx, query,
		tc.ID,
		tc.TestRunID,
		tc.TenantID,
		tc.Name,
		tc.Description,
		tc.Category,
		string(tc.Priority),
		string(tc.Status),
		stepsJSON,
		tc.Script,
		originalScript,
		executionResult,
		healingHistoryJSON,
		tc.RetryCount,
		tc.Duration.Milliseconds(),
		tc.CreatedAt,
		tc.UpdatedAt,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return domain.NotFoundError("test_run", tc.TestRunID)
		}
		return err
	}

	return nil
}

// CreateBatch inserts multiple test cases in a single transaction
func (r *TestCaseRepository) CreateBatch(ctx context.Context, tcs []*domain.TestCase) error {
	if len(tcs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO test_cases (
			id, test_run_id, tenant_id, name, description, category, priority, status,
			steps, script, original_script, execution_result, healing_history,
			retry_count, duration_ms, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	stmt, err := tx.PreparexContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, tc := range tcs {
		// Handle nil slices by defaulting to empty arrays for JSONB columns
		steps := tc.Steps
		if steps == nil {
			steps = []domain.TestStep{}
		}
		stepsJSON, err := json.Marshal(steps)
		if err != nil {
			return err
		}

		// executionResult can be null in the DB
		var executionResult interface{} = nil
		if tc.ExecutionResult != nil {
			executionResult, err = json.Marshal(tc.ExecutionResult)
			if err != nil {
				return err
			}
		}

		// healing_history defaults to empty array
		healingHistory := tc.HealingHistory
		if healingHistory == nil {
			healingHistory = []domain.HealingRecord{}
		}
		healingHistoryJSON, err := json.Marshal(healingHistory)
		if err != nil {
			return err
		}

		var originalScript *string
		if tc.OriginalScript != "" {
			originalScript = &tc.OriginalScript
		}

		_, err = stmt.ExecContext(ctx,
			tc.ID,
			tc.TestRunID,
			tc.TenantID,
			tc.Name,
			tc.Description,
			tc.Category,
			string(tc.Priority),
			string(tc.Status),
			stepsJSON,
			tc.Script,
			originalScript,
			executionResult,
			healingHistoryJSON,
			tc.RetryCount,
			tc.Duration.Milliseconds(),
			tc.CreatedAt,
			tc.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetByID retrieves a test case by ID
func (r *TestCaseRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.TestCase, error) {
	query := `
		SELECT id, test_run_id, tenant_id, name, description, category, priority, status,
		       steps, script, original_script, execution_result, healing_history,
		       retry_count, duration_ms, created_at, updated_at, deleted_at
		FROM test_cases
		WHERE id = $1 AND deleted_at IS NULL
	`

	var row testCaseRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFoundError("test_case", id)
		}
		return nil, err
	}

	return row.toDomain()
}

// GetByTestRunID retrieves all test cases for a test run
func (r *TestCaseRepository) GetByTestRunID(ctx context.Context, testRunID uuid.UUID) ([]*domain.TestCase, error) {
	query := `
		SELECT id, test_run_id, tenant_id, name, description, category, priority, status,
		       steps, script, original_script, execution_result, healing_history,
		       retry_count, duration_ms, created_at, updated_at, deleted_at
		FROM test_cases
		WHERE test_run_id = $1 AND deleted_at IS NULL
		ORDER BY created_at
	`

	var rows []testCaseRow
	if err := r.db.SelectContext(ctx, &rows, query, testRunID); err != nil {
		return nil, err
	}

	cases := make([]*domain.TestCase, len(rows))
	for i, row := range rows {
		tc, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		cases[i] = tc
	}

	return cases, nil
}

// Update updates an existing test case
func (r *TestCaseRepository) Update(ctx context.Context, tc *domain.TestCase) error {
	// Handle nil slices by defaulting to empty arrays for JSONB columns
	steps := tc.Steps
	if steps == nil {
		steps = []domain.TestStep{}
	}
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return err
	}

	// executionResult can be null in the DB
	var executionResult interface{} = nil
	if tc.ExecutionResult != nil {
		executionResult, err = json.Marshal(tc.ExecutionResult)
		if err != nil {
			return err
		}
	}

	// healing_history defaults to empty array
	healingHistory := tc.HealingHistory
	if healingHistory == nil {
		healingHistory = []domain.HealingRecord{}
	}
	healingHistoryJSON, err := json.Marshal(healingHistory)
	if err != nil {
		return err
	}

	var originalScript *string
	if tc.OriginalScript != "" {
		originalScript = &tc.OriginalScript
	}

	query := `
		UPDATE test_cases
		SET status = $2, script = $3, original_script = $4, execution_result = $5,
		    healing_history = $6, retry_count = $7, duration_ms = $8, steps = $9,
		    updated_at = $10
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		tc.ID,
		string(tc.Status),
		tc.Script,
		originalScript,
		executionResult,
		healingHistoryJSON,
		tc.RetryCount,
		tc.Duration.Milliseconds(),
		stepsJSON,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.NotFoundError("test_case", tc.ID)
	}

	return nil
}

// UpdateStatus updates only the status of a test case
func (r *TestCaseRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.TestCaseStatus) error {
	query := `
		UPDATE test_cases
		SET status = $2, updated_at = $3
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, string(status), time.Now().UTC())
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.NotFoundError("test_case", id)
	}

	return nil
}

// Delete soft deletes a test case
func (r *TestCaseRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE test_cases
		SET deleted_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now().UTC())
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.NotFoundError("test_case", id)
	}

	return nil
}

// GetFailedByTestRunID retrieves failed test cases for a test run
func (r *TestCaseRepository) GetFailedByTestRunID(ctx context.Context, testRunID uuid.UUID) ([]*domain.TestCase, error) {
	query := `
		SELECT id, test_run_id, tenant_id, name, description, category, priority, status,
		       steps, script, original_script, execution_result, healing_history,
		       retry_count, duration_ms, created_at, updated_at, deleted_at
		FROM test_cases
		WHERE test_run_id = $1 AND status = 'failed' AND deleted_at IS NULL
		ORDER BY priority DESC, created_at
	`

	var rows []testCaseRow
	if err := r.db.SelectContext(ctx, &rows, query, testRunID); err != nil {
		return nil, err
	}

	cases := make([]*domain.TestCase, len(rows))
	for i, row := range rows {
		tc, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		cases[i] = tc
	}

	return cases, nil
}
