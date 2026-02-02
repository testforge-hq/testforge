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

// TestRunRepository implements domain.TestRunRepository with PostgreSQL
type TestRunRepository struct {
	db *sqlx.DB
}

// NewTestRunRepository creates a new test run repository
func NewTestRunRepository(db *sqlx.DB) *TestRunRepository {
	return &TestRunRepository{db: db}
}

// testRunRow represents the database row structure
type testRunRow struct {
	ID              uuid.UUID  `db:"id"`
	TenantID        uuid.UUID  `db:"tenant_id"`
	ProjectID       uuid.UUID  `db:"project_id"`
	Status          string     `db:"status"`
	TargetURL       string     `db:"target_url"`
	WorkflowID      string     `db:"workflow_id"`
	WorkflowRunID   string     `db:"workflow_run_id"`
	DiscoveryResult []byte     `db:"discovery_result"`
	TestPlan        []byte     `db:"test_plan"`
	Summary         []byte     `db:"summary"`
	ReportURL       *string    `db:"report_url"`
	TriggeredBy     string     `db:"triggered_by"`
	StartedAt       *time.Time `db:"started_at"`
	CompletedAt     *time.Time `db:"completed_at"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	DeletedAt       *time.Time `db:"deleted_at"`
}

func (r *testRunRow) toDomain() (*domain.TestRun, error) {
	run := &domain.TestRun{
		ID:            r.ID,
		TenantID:      r.TenantID,
		ProjectID:     r.ProjectID,
		Status:        domain.RunStatus(r.Status),
		TargetURL:     r.TargetURL,
		WorkflowID:    r.WorkflowID,
		WorkflowRunID: r.WorkflowRunID,
		TriggeredBy:   r.TriggeredBy,
		StartedAt:     r.StartedAt,
		CompletedAt:   r.CompletedAt,
		Timestamps: domain.Timestamps{
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
			DeletedAt: r.DeletedAt,
		},
	}

	if r.ReportURL != nil {
		run.ReportURL = *r.ReportURL
	}

	if r.DiscoveryResult != nil {
		var discovery domain.DiscoveryResult
		if err := json.Unmarshal(r.DiscoveryResult, &discovery); err != nil {
			return nil, err
		}
		run.DiscoveryResult = &discovery
	}

	if r.TestPlan != nil {
		var plan domain.TestPlan
		if err := json.Unmarshal(r.TestPlan, &plan); err != nil {
			return nil, err
		}
		run.TestPlan = &plan
	}

	if r.Summary != nil {
		var summary domain.RunSummary
		if err := json.Unmarshal(r.Summary, &summary); err != nil {
			return nil, err
		}
		run.Summary = &summary
	}

	return run, nil
}

// Create inserts a new test run
func (r *TestRunRepository) Create(ctx context.Context, run *domain.TestRun) error {
	// Handle JSONB fields - use interface{} to properly pass NULL
	var discoveryResult, testPlan, summary interface{}

	if run.DiscoveryResult != nil {
		data, err := json.Marshal(run.DiscoveryResult)
		if err != nil {
			return err
		}
		discoveryResult = data
	}

	if run.TestPlan != nil {
		data, err := json.Marshal(run.TestPlan)
		if err != nil {
			return err
		}
		testPlan = data
	}

	if run.Summary != nil {
		data, err := json.Marshal(run.Summary)
		if err != nil {
			return err
		}
		summary = data
	}

	var reportURL *string
	if run.ReportURL != "" {
		reportURL = &run.ReportURL
	}

	query := `
		INSERT INTO test_runs (
			id, tenant_id, project_id, status, target_url, workflow_id, workflow_run_id,
			discovery_result, test_plan, summary, report_url, triggered_by,
			started_at, completed_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`

	_, err := r.db.ExecContext(ctx, query,
		run.ID,
		run.TenantID,
		run.ProjectID,
		string(run.Status),
		run.TargetURL,
		run.WorkflowID,
		run.WorkflowRunID,
		discoveryResult,
		testPlan,
		summary,
		reportURL,
		run.TriggeredBy,
		run.StartedAt,
		run.CompletedAt,
		run.CreatedAt,
		run.UpdatedAt,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return domain.NotFoundError("project", run.ProjectID)
		}
		return err
	}

	return nil
}

// GetByID retrieves a test run by ID
func (r *TestRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.TestRun, error) {
	query := `
		SELECT id, tenant_id, project_id, status, target_url, workflow_id, workflow_run_id,
		       discovery_result, test_plan, summary, report_url, triggered_by,
		       started_at, completed_at, created_at, updated_at, deleted_at
		FROM test_runs
		WHERE id = $1 AND deleted_at IS NULL
	`

	var row testRunRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFoundError("test_run", id)
		}
		return nil, err
	}

	return row.toDomain()
}

// GetByWorkflowID retrieves a test run by Temporal workflow ID
func (r *TestRunRepository) GetByWorkflowID(ctx context.Context, workflowID string) (*domain.TestRun, error) {
	query := `
		SELECT id, tenant_id, project_id, status, target_url, workflow_id, workflow_run_id,
		       discovery_result, test_plan, summary, report_url, triggered_by,
		       started_at, completed_at, created_at, updated_at, deleted_at
		FROM test_runs
		WHERE workflow_id = $1 AND deleted_at IS NULL
	`

	var row testRunRow
	if err := r.db.GetContext(ctx, &row, query, workflowID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFoundError("test_run", workflowID)
		}
		return nil, err
	}

	return row.toDomain()
}

// GetByProjectID retrieves paginated test runs for a project
func (r *TestRunRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]*domain.TestRun, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM test_runs WHERE project_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, projectID); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, tenant_id, project_id, status, target_url, workflow_id, workflow_run_id,
		       discovery_result, test_plan, summary, report_url, triggered_by,
		       started_at, completed_at, created_at, updated_at, deleted_at
		FROM test_runs
		WHERE project_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var rows []testRunRow
	if err := r.db.SelectContext(ctx, &rows, query, projectID, limit, offset); err != nil {
		return nil, 0, err
	}

	runs := make([]*domain.TestRun, len(rows))
	for i, row := range rows {
		run, err := row.toDomain()
		if err != nil {
			return nil, 0, err
		}
		runs[i] = run
	}

	return runs, total, nil
}

// GetByTenantID retrieves paginated test runs for a tenant
func (r *TestRunRepository) GetByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.TestRun, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM test_runs WHERE tenant_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, tenant_id, project_id, status, target_url, workflow_id, workflow_run_id,
		       discovery_result, test_plan, summary, report_url, triggered_by,
		       started_at, completed_at, created_at, updated_at, deleted_at
		FROM test_runs
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var rows []testRunRow
	if err := r.db.SelectContext(ctx, &rows, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	runs := make([]*domain.TestRun, len(rows))
	for i, row := range rows {
		run, err := row.toDomain()
		if err != nil {
			return nil, 0, err
		}
		runs[i] = run
	}

	return runs, total, nil
}

// Update updates an existing test run
func (r *TestRunRepository) Update(ctx context.Context, run *domain.TestRun) error {
	// Handle JSONB fields - use interface{} to properly pass NULL
	var discoveryResult, testPlan, summary interface{}

	if run.DiscoveryResult != nil {
		data, err := json.Marshal(run.DiscoveryResult)
		if err != nil {
			return err
		}
		discoveryResult = data
	}

	if run.TestPlan != nil {
		data, err := json.Marshal(run.TestPlan)
		if err != nil {
			return err
		}
		testPlan = data
	}

	if run.Summary != nil {
		data, err := json.Marshal(run.Summary)
		if err != nil {
			return err
		}
		summary = data
	}

	var reportURL *string
	if run.ReportURL != "" {
		reportURL = &run.ReportURL
	}

	query := `
		UPDATE test_runs
		SET status = $2, workflow_id = $3, workflow_run_id = $4,
		    discovery_result = $5, test_plan = $6, summary = $7, report_url = $8,
		    started_at = $9, completed_at = $10, updated_at = $11
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		run.ID,
		string(run.Status),
		run.WorkflowID,
		run.WorkflowRunID,
		discoveryResult,
		testPlan,
		summary,
		reportURL,
		run.StartedAt,
		run.CompletedAt,
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
		return domain.NotFoundError("test_run", run.ID)
	}

	return nil
}

// UpdateStatus updates only the status of a test run
func (r *TestRunRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.RunStatus) error {
	query := `
		UPDATE test_runs
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
		return domain.NotFoundError("test_run", id)
	}

	return nil
}

// Delete soft deletes a test run
func (r *TestRunRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE test_runs
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
		return domain.NotFoundError("test_run", id)
	}

	return nil
}

// CountActiveByTenant counts active (non-terminal) runs for a tenant
func (r *TestRunRepository) CountActiveByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM test_runs
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND status NOT IN ('completed', 'failed', 'cancelled')
	`

	var count int
	if err := r.db.GetContext(ctx, &count, query, tenantID); err != nil {
		return 0, err
	}

	return count, nil
}
