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

// ReportRepository implements domain.ReportRepository with PostgreSQL
type ReportRepository struct {
	db *sqlx.DB
}

// NewReportRepository creates a new report repository
func NewReportRepository(db *sqlx.DB) *ReportRepository {
	return &ReportRepository{db: db}
}

// reportRow represents the database row structure
type reportRow struct {
	ID          uuid.UUID  `db:"id"`
	TestRunID   uuid.UUID  `db:"test_run_id"`
	TenantID    uuid.UUID  `db:"tenant_id"`
	Type        string     `db:"type"`
	Format      string     `db:"format"`
	URL         string     `db:"url"`
	SizeBytes   int64      `db:"size_bytes"`
	Summary     []byte     `db:"summary"`
	GeneratedAt time.Time  `db:"generated_at"`
	ExpiresAt   *time.Time `db:"expires_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
	DeletedAt   *time.Time `db:"deleted_at"`
}

func (r *reportRow) toDomain() (*domain.Report, error) {
	var summary domain.ReportSummary
	if err := json.Unmarshal(r.Summary, &summary); err != nil {
		return nil, err
	}

	return &domain.Report{
		ID:          r.ID,
		TestRunID:   r.TestRunID,
		TenantID:    r.TenantID,
		Type:        domain.ReportType(r.Type),
		Format:      domain.ReportFormat(r.Format),
		URL:         r.URL,
		Size:        r.SizeBytes,
		Summary:     summary,
		GeneratedAt: r.GeneratedAt,
		ExpiresAt:   r.ExpiresAt,
		Timestamps: domain.Timestamps{
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
			DeletedAt: r.DeletedAt,
		},
	}, nil
}

// Create inserts a new report
func (r *ReportRepository) Create(ctx context.Context, report *domain.Report) error {
	summary, err := json.Marshal(report.Summary)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO reports (
			id, test_run_id, tenant_id, type, format, url, size_bytes,
			summary, generated_at, expires_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = r.db.ExecContext(ctx, query,
		report.ID,
		report.TestRunID,
		report.TenantID,
		string(report.Type),
		string(report.Format),
		report.URL,
		report.Size,
		summary,
		report.GeneratedAt,
		report.ExpiresAt,
		report.CreatedAt,
		report.UpdatedAt,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return domain.NotFoundError("test_run", report.TestRunID)
		}
		return err
	}

	return nil
}

// GetByID retrieves a report by ID
func (r *ReportRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Report, error) {
	query := `
		SELECT id, test_run_id, tenant_id, type, format, url, size_bytes,
		       summary, generated_at, expires_at, created_at, updated_at, deleted_at
		FROM reports
		WHERE id = $1 AND deleted_at IS NULL
	`

	var row reportRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFoundError("report", id)
		}
		return nil, err
	}

	return row.toDomain()
}

// GetByTestRunID retrieves all reports for a test run
func (r *ReportRepository) GetByTestRunID(ctx context.Context, testRunID uuid.UUID) ([]*domain.Report, error) {
	query := `
		SELECT id, test_run_id, tenant_id, type, format, url, size_bytes,
		       summary, generated_at, expires_at, created_at, updated_at, deleted_at
		FROM reports
		WHERE test_run_id = $1 AND deleted_at IS NULL
		ORDER BY generated_at DESC
	`

	var rows []reportRow
	if err := r.db.SelectContext(ctx, &rows, query, testRunID); err != nil {
		return nil, err
	}

	reports := make([]*domain.Report, len(rows))
	for i, row := range rows {
		report, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		reports[i] = report
	}

	return reports, nil
}

// GetLatestByProject retrieves the latest reports for a project's test runs
func (r *ReportRepository) GetLatestByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*domain.Report, error) {
	query := `
		SELECT r.id, r.test_run_id, r.tenant_id, r.type, r.format, r.url, r.size_bytes,
		       r.summary, r.generated_at, r.expires_at, r.created_at, r.updated_at, r.deleted_at
		FROM reports r
		JOIN test_runs tr ON r.test_run_id = tr.id
		WHERE tr.project_id = $1 AND r.deleted_at IS NULL AND tr.deleted_at IS NULL
		ORDER BY r.generated_at DESC
		LIMIT $2
	`

	var rows []reportRow
	if err := r.db.SelectContext(ctx, &rows, query, projectID, limit); err != nil {
		return nil, err
	}

	reports := make([]*domain.Report, len(rows))
	for i, row := range rows {
		report, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		reports[i] = report
	}

	return reports, nil
}

// Delete soft deletes a report
func (r *ReportRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE reports
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
		return domain.NotFoundError("report", id)
	}

	return nil
}

// DeleteExpired deletes all expired reports
func (r *ReportRepository) DeleteExpired(ctx context.Context) (int, error) {
	query := `
		UPDATE reports
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE deleted_at IS NULL
		  AND expires_at IS NOT NULL
		  AND expires_at < NOW()
	`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(rows), nil
}
