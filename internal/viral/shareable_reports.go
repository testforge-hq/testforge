package viral

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ShareableReport represents a publicly accessible test report
type ShareableReport struct {
	ID          uuid.UUID `db:"id" json:"id"`
	RunID       uuid.UUID `db:"run_id" json:"run_id"`
	TenantID    uuid.UUID `db:"tenant_id" json:"tenant_id"`
	ProjectID   uuid.UUID `db:"project_id" json:"project_id"`
	ShareCode   string    `db:"share_code" json:"share_code"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description,omitempty"`

	// Access control
	IsPublic     bool       `db:"is_public" json:"is_public"`
	Password     *string    `db:"password_hash" json:"-"` // Hashed password for protected reports
	ExpiresAt    *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	MaxViews     *int       `db:"max_views" json:"max_views,omitempty"`
	ViewCount    int        `db:"view_count" json:"view_count"`

	// Report data (cached for performance)
	ReportData  json.RawMessage `db:"report_data" json:"report_data,omitempty"`

	// Metadata
	CreatedBy   uuid.UUID  `db:"created_by" json:"created_by"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	LastViewedAt *time.Time `db:"last_viewed_at" json:"last_viewed_at,omitempty"`
}

// ShareableReportService manages shareable reports
type ShareableReportService struct {
	db      *sqlx.DB
	logger  *zap.Logger
	baseURL string
}

// NewShareableReportService creates a new shareable report service
func NewShareableReportService(db *sqlx.DB, baseURL string, logger *zap.Logger) *ShareableReportService {
	return &ShareableReportService{
		db:      db,
		logger:  logger,
		baseURL: baseURL,
	}
}

// CreateShareableReport creates a new shareable report
func (srs *ShareableReportService) CreateShareableReport(ctx context.Context, req CreateShareRequest) (*ShareableReport, error) {
	// Generate unique share code
	shareCode, err := generateShareCode()
	if err != nil {
		return nil, fmt.Errorf("generating share code: %w", err)
	}

	// Get report data from the test run
	reportData, err := srs.getReportData(ctx, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("getting report data: %w", err)
	}

	report := &ShareableReport{
		ID:          uuid.New(),
		RunID:       req.RunID,
		TenantID:    req.TenantID,
		ProjectID:   req.ProjectID,
		ShareCode:   shareCode,
		Title:       req.Title,
		Description: req.Description,
		IsPublic:    req.IsPublic,
		ExpiresAt:   req.ExpiresAt,
		MaxViews:    req.MaxViews,
		ReportData:  reportData,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   time.Now(),
	}

	// Hash password if provided
	if req.Password != "" {
		// In production, use bcrypt
		hashedPassword := hashPassword(req.Password)
		report.Password = &hashedPassword
	}

	_, err = srs.db.ExecContext(ctx, `
		INSERT INTO shareable_reports (
			id, run_id, tenant_id, project_id, share_code, title, description,
			is_public, password_hash, expires_at, max_views, report_data, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		report.ID, report.RunID, report.TenantID, report.ProjectID, report.ShareCode,
		report.Title, report.Description, report.IsPublic, report.Password,
		report.ExpiresAt, report.MaxViews, report.ReportData, report.CreatedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting shareable report: %w", err)
	}

	return report, nil
}

// CreateShareRequest represents a request to create a shareable report
type CreateShareRequest struct {
	RunID       uuid.UUID  `json:"run_id"`
	TenantID    uuid.UUID  `json:"tenant_id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	IsPublic    bool       `json:"is_public"`
	Password    string     `json:"password,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	MaxViews    *int       `json:"max_views,omitempty"`
	CreatedBy   uuid.UUID  `json:"created_by"`
}

// GetByShareCode retrieves a report by its share code
func (srs *ShareableReportService) GetByShareCode(ctx context.Context, shareCode string) (*ShareableReport, error) {
	var report ShareableReport
	err := srs.db.GetContext(ctx, &report, `
		SELECT * FROM shareable_reports
		WHERE share_code = $1`,
		shareCode,
	)
	if err != nil {
		return nil, fmt.Errorf("getting shareable report: %w", err)
	}

	return &report, nil
}

// ViewReport records a view and returns the report data
func (srs *ShareableReportService) ViewReport(ctx context.Context, shareCode string, password string) (*ShareableReport, error) {
	report, err := srs.GetByShareCode(ctx, shareCode)
	if err != nil {
		return nil, err
	}

	// Check if expired
	if report.ExpiresAt != nil && time.Now().After(*report.ExpiresAt) {
		return nil, fmt.Errorf("report has expired")
	}

	// Check view limit
	if report.MaxViews != nil && report.ViewCount >= *report.MaxViews {
		return nil, fmt.Errorf("report view limit reached")
	}

	// Check password if protected
	if report.Password != nil && !checkPassword(password, *report.Password) {
		return nil, fmt.Errorf("invalid password")
	}

	// Increment view count
	now := time.Now()
	_, err = srs.db.ExecContext(ctx, `
		UPDATE shareable_reports
		SET view_count = view_count + 1, last_viewed_at = $1
		WHERE id = $2`,
		now, report.ID,
	)
	if err != nil {
		srs.logger.Warn("failed to update view count", zap.Error(err))
	}

	report.ViewCount++
	report.LastViewedAt = &now

	return report, nil
}

// GetURL returns the public URL for a shareable report
func (srs *ShareableReportService) GetURL(shareCode string) string {
	return fmt.Sprintf("%s/r/%s", srs.baseURL, shareCode)
}

// RevokeReport revokes access to a shareable report
func (srs *ShareableReportService) RevokeReport(ctx context.Context, reportID uuid.UUID, tenantID uuid.UUID) error {
	result, err := srs.db.ExecContext(ctx, `
		DELETE FROM shareable_reports
		WHERE id = $1 AND tenant_id = $2`,
		reportID, tenantID,
	)
	if err != nil {
		return fmt.Errorf("revoking report: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("report not found or unauthorized")
	}

	return nil
}

// ListReports lists shareable reports for a project
func (srs *ShareableReportService) ListReports(ctx context.Context, projectID uuid.UUID, limit int) ([]ShareableReport, error) {
	var reports []ShareableReport
	err := srs.db.SelectContext(ctx, &reports, `
		SELECT id, run_id, tenant_id, project_id, share_code, title, description,
			   is_public, expires_at, max_views, view_count, created_by, created_at, last_viewed_at
		FROM shareable_reports
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2`,
		projectID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing reports: %w", err)
	}

	return reports, nil
}

// UpdateReport updates a shareable report
func (srs *ShareableReportService) UpdateReport(ctx context.Context, reportID uuid.UUID, tenantID uuid.UUID, update UpdateShareRequest) error {
	_, err := srs.db.ExecContext(ctx, `
		UPDATE shareable_reports
		SET title = COALESCE(NULLIF($3, ''), title),
			description = COALESCE($4, description),
			is_public = COALESCE($5, is_public),
			expires_at = $6,
			max_views = $7
		WHERE id = $1 AND tenant_id = $2`,
		reportID, tenantID, update.Title, update.Description,
		update.IsPublic, update.ExpiresAt, update.MaxViews,
	)
	return err
}

// UpdateShareRequest represents an update to a shareable report
type UpdateShareRequest struct {
	Title       string     `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	IsPublic    *bool      `json:"is_public,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	MaxViews    *int       `json:"max_views,omitempty"`
}

// ShareHandler returns an HTTP handler for viewing shared reports
func (srs *ShareableReportService) ShareHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract share code from URL path
		shareCode := r.PathValue("code")
		if shareCode == "" {
			http.Error(w, "Missing share code", http.StatusBadRequest)
			return
		}

		password := r.URL.Query().Get("password")

		report, err := srs.ViewReport(r.Context(), shareCode, password)
		if err != nil {
			srs.logger.Warn("failed to view report", zap.Error(err))
			if err.Error() == "invalid password" {
				http.Error(w, "Password required", http.StatusUnauthorized)
			} else if err.Error() == "report has expired" || err.Error() == "report view limit reached" {
				http.Error(w, err.Error(), http.StatusGone)
			} else {
				http.Error(w, "Report not found", http.StatusNotFound)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	}
}

// Private methods

func (srs *ShareableReportService) getReportData(ctx context.Context, runID uuid.UUID) (json.RawMessage, error) {
	// Get test run with results
	var data struct {
		ID           uuid.UUID       `db:"id" json:"id"`
		ProjectName  string          `db:"project_name" json:"project_name"`
		Status       string          `db:"status" json:"status"`
		TotalTests   int             `db:"total_tests" json:"total_tests"`
		PassedTests  int             `db:"passed_tests" json:"passed_tests"`
		FailedTests  int             `db:"failed_tests" json:"failed_tests"`
		SkippedTests int             `db:"skipped_tests" json:"skipped_tests"`
		Duration     int64           `db:"duration_ms" json:"duration_ms"`
		StartedAt    time.Time       `db:"started_at" json:"started_at"`
		EndedAt      *time.Time      `db:"ended_at" json:"ended_at,omitempty"`
		Results      json.RawMessage `db:"results" json:"results,omitempty"`
	}

	err := srs.db.GetContext(ctx, &data, `
		SELECT
			tr.id, p.name as project_name, tr.status,
			tr.total_tests, tr.passed_tests, tr.failed_tests, tr.skipped_tests,
			EXTRACT(EPOCH FROM (tr.ended_at - tr.started_at)) * 1000 as duration_ms,
			tr.started_at, tr.ended_at, tr.results
		FROM test_runs tr
		JOIN projects p ON p.id = tr.project_id
		WHERE tr.id = $1`,
		runID,
	)
	if err != nil {
		return nil, err
	}

	return json.Marshal(data)
}

// Helper functions

func generateShareCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:11], nil
}

func hashPassword(password string) string {
	// In production, use bcrypt.GenerateFromPassword
	// This is a placeholder for the actual implementation
	return password // TODO: Use bcrypt
}

func checkPassword(password, hash string) bool {
	// In production, use bcrypt.CompareHashAndPassword
	// This is a placeholder for the actual implementation
	return password == hash // TODO: Use bcrypt
}
