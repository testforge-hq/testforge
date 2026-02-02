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

// TenantRepository implements domain.TenantRepository with PostgreSQL
type TenantRepository struct {
	db *sqlx.DB
}

// NewTenantRepository creates a new tenant repository
func NewTenantRepository(db *sqlx.DB) *TenantRepository {
	return &TenantRepository{db: db}
}

// tenantRow represents the database row structure
type tenantRow struct {
	ID        uuid.UUID  `db:"id"`
	Name      string     `db:"name"`
	Slug      string     `db:"slug"`
	Plan      string     `db:"plan"`
	Settings  []byte     `db:"settings"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

func (r *tenantRow) toDomain() (*domain.Tenant, error) {
	var settings domain.TenantSettings
	if err := json.Unmarshal(r.Settings, &settings); err != nil {
		return nil, err
	}

	return &domain.Tenant{
		ID:       r.ID,
		Name:     r.Name,
		Slug:     r.Slug,
		Plan:     domain.Plan(r.Plan),
		Settings: settings,
		Timestamps: domain.Timestamps{
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
			DeletedAt: r.DeletedAt,
		},
	}, nil
}

// Create inserts a new tenant
func (r *TenantRepository) Create(ctx context.Context, tenant *domain.Tenant) error {
	settings, err := json.Marshal(tenant.Settings)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO tenants (id, name, slug, plan, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = r.db.ExecContext(ctx, query,
		tenant.ID,
		tenant.Name,
		tenant.Slug,
		string(tenant.Plan),
		settings,
		tenant.CreatedAt,
		tenant.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.AlreadyExistsError("tenant", "slug", tenant.Slug)
		}
		return err
	}

	return nil
}

// GetByID retrieves a tenant by ID
func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	query := `
		SELECT id, name, slug, plan, settings, created_at, updated_at, deleted_at
		FROM tenants
		WHERE id = $1 AND deleted_at IS NULL
	`

	var row tenantRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFoundError("tenant", id)
		}
		return nil, err
	}

	return row.toDomain()
}

// GetBySlug retrieves a tenant by slug
func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	query := `
		SELECT id, name, slug, plan, settings, created_at, updated_at, deleted_at
		FROM tenants
		WHERE slug = $1 AND deleted_at IS NULL
	`

	var row tenantRow
	if err := r.db.GetContext(ctx, &row, query, slug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFoundError("tenant", slug)
		}
		return nil, err
	}

	return row.toDomain()
}

// Update updates an existing tenant
func (r *TenantRepository) Update(ctx context.Context, tenant *domain.Tenant) error {
	settings, err := json.Marshal(tenant.Settings)
	if err != nil {
		return err
	}

	query := `
		UPDATE tenants
		SET name = $2, plan = $3, settings = $4, updated_at = $5
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		tenant.ID,
		tenant.Name,
		string(tenant.Plan),
		settings,
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
		return domain.NotFoundError("tenant", tenant.ID)
	}

	return nil
}

// Delete soft deletes a tenant
func (r *TenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE tenants
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
		return domain.NotFoundError("tenant", id)
	}

	return nil
}

// List retrieves paginated tenants
func (r *TenantRepository) List(ctx context.Context, limit, offset int) ([]*domain.Tenant, int, error) {
	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM tenants WHERE deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery); err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query := `
		SELECT id, name, slug, plan, settings, created_at, updated_at, deleted_at
		FROM tenants
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	var rows []tenantRow
	if err := r.db.SelectContext(ctx, &rows, query, limit, offset); err != nil {
		return nil, 0, err
	}

	tenants := make([]*domain.Tenant, len(rows))
	for i, row := range rows {
		tenant, err := row.toDomain()
		if err != nil {
			return nil, 0, err
		}
		tenants[i] = tenant
	}

	return tenants, total, nil
}

// ExistsBySlug checks if a tenant with the given slug exists
func (r *TenantRepository) ExistsBySlug(ctx context.Context, slug string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM tenants WHERE slug = $1 AND deleted_at IS NULL)`
	var exists bool
	if err := r.db.GetContext(ctx, &exists, query, slug); err != nil {
		return false, err
	}
	return exists, nil
}
