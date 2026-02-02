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

// ProjectRepository implements domain.ProjectRepository with PostgreSQL
type ProjectRepository struct {
	db *sqlx.DB
}

// NewProjectRepository creates a new project repository
func NewProjectRepository(db *sqlx.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// projectRow represents the database row structure
type projectRow struct {
	ID          uuid.UUID  `db:"id"`
	TenantID    uuid.UUID  `db:"tenant_id"`
	Name        string     `db:"name"`
	Description string     `db:"description"`
	BaseURL     string     `db:"base_url"`
	Settings    []byte     `db:"settings"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
	DeletedAt   *time.Time `db:"deleted_at"`
}

func (r *projectRow) toDomain() (*domain.Project, error) {
	var settings domain.ProjectSettings
	if err := json.Unmarshal(r.Settings, &settings); err != nil {
		return nil, err
	}

	return &domain.Project{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Description: r.Description,
		BaseURL:     r.BaseURL,
		Settings:    settings,
		Timestamps: domain.Timestamps{
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
			DeletedAt: r.DeletedAt,
		},
	}, nil
}

// Create inserts a new project
func (r *ProjectRepository) Create(ctx context.Context, project *domain.Project) error {
	settings, err := json.Marshal(project.Settings)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO projects (id, tenant_id, name, description, base_url, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = r.db.ExecContext(ctx, query,
		project.ID,
		project.TenantID,
		project.Name,
		project.Description,
		project.BaseURL,
		settings,
		project.CreatedAt,
		project.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.AlreadyExistsError("project", "name", project.Name)
		}
		if isForeignKeyViolation(err) {
			return domain.NotFoundError("tenant", project.TenantID)
		}
		return err
	}

	return nil
}

// GetByID retrieves a project by ID
func (r *ProjectRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Project, error) {
	query := `
		SELECT id, tenant_id, name, description, base_url, settings, created_at, updated_at, deleted_at
		FROM projects
		WHERE id = $1 AND deleted_at IS NULL
	`

	var row projectRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFoundError("project", id)
		}
		return nil, err
	}

	return row.toDomain()
}

// GetByTenantID retrieves paginated projects for a tenant
func (r *ProjectRepository) GetByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.Project, int, error) {
	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM projects WHERE tenant_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query := `
		SELECT id, tenant_id, name, description, base_url, settings, created_at, updated_at, deleted_at
		FROM projects
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var rows []projectRow
	if err := r.db.SelectContext(ctx, &rows, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	projects := make([]*domain.Project, len(rows))
	for i, row := range rows {
		project, err := row.toDomain()
		if err != nil {
			return nil, 0, err
		}
		projects[i] = project
	}

	return projects, total, nil
}

// Update updates an existing project
func (r *ProjectRepository) Update(ctx context.Context, project *domain.Project) error {
	settings, err := json.Marshal(project.Settings)
	if err != nil {
		return err
	}

	query := `
		UPDATE projects
		SET name = $2, description = $3, base_url = $4, settings = $5, updated_at = $6
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		project.ID,
		project.Name,
		project.Description,
		project.BaseURL,
		settings,
		time.Now().UTC(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.AlreadyExistsError("project", "name", project.Name)
		}
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.NotFoundError("project", project.ID)
	}

	return nil
}

// Delete soft deletes a project
func (r *ProjectRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE projects
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
		return domain.NotFoundError("project", id)
	}

	return nil
}

// ExistsByNameAndTenant checks if a project with the given name exists for a tenant
func (r *ProjectRepository) ExistsByNameAndTenant(ctx context.Context, name string, tenantID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM projects WHERE name = $1 AND tenant_id = $2 AND deleted_at IS NULL)`
	var exists bool
	if err := r.db.GetContext(ctx, &exists, query, name, tenantID); err != nil {
		return false, err
	}
	return exists, nil
}
