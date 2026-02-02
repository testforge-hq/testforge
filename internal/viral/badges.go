// Package viral provides viral growth features for TestForge
package viral

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// BadgeStatus represents the status shown on a badge
type BadgeStatus string

const (
	BadgeStatusPassing BadgeStatus = "passing"
	BadgeStatusFailing BadgeStatus = "failing"
	BadgeStatusPending BadgeStatus = "pending"
	BadgeStatusUnknown BadgeStatus = "unknown"
)

// Badge represents a project status badge
type Badge struct {
	ProjectID   uuid.UUID   `json:"project_id"`
	Status      BadgeStatus `json:"status"`
	PassRate    float64     `json:"pass_rate"`
	TestCount   int         `json:"test_count"`
	LastUpdated time.Time   `json:"last_updated"`
}

// BadgeService generates status badges for projects
type BadgeService struct {
	db     *sqlx.DB
	logger *zap.Logger

	// Cache badges for performance
	cache    map[string]*cachedBadge
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
}

type cachedBadge struct {
	badge     *Badge
	svg       []byte
	createdAt time.Time
}

// NewBadgeService creates a new badge service
func NewBadgeService(db *sqlx.DB, logger *zap.Logger) *BadgeService {
	return &BadgeService{
		db:       db,
		logger:   logger,
		cache:    make(map[string]*cachedBadge),
		cacheTTL: 5 * time.Minute,
	}
}

// GetBadge returns badge data for a project
func (bs *BadgeService) GetBadge(ctx context.Context, projectID uuid.UUID) (*Badge, error) {
	// Check cache
	bs.cacheMu.RLock()
	if cached, ok := bs.cache[projectID.String()]; ok {
		if time.Since(cached.createdAt) < bs.cacheTTL {
			bs.cacheMu.RUnlock()
			return cached.badge, nil
		}
	}
	bs.cacheMu.RUnlock()

	// Query latest run results
	var result struct {
		TotalTests  int       `db:"total_tests"`
		PassedTests int       `db:"passed_tests"`
		LastRun     time.Time `db:"last_run"`
	}

	err := bs.db.GetContext(ctx, &result, `
		SELECT
			COALESCE(SUM(total_tests), 0) as total_tests,
			COALESCE(SUM(passed_tests), 0) as passed_tests,
			COALESCE(MAX(ended_at), NOW()) as last_run
		FROM test_runs
		WHERE project_id = $1
		  AND ended_at > NOW() - INTERVAL '30 days'
		  AND status = 'completed'`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying project stats: %w", err)
	}

	var status BadgeStatus
	var passRate float64

	if result.TotalTests == 0 {
		status = BadgeStatusUnknown
		passRate = 0
	} else {
		passRate = float64(result.PassedTests) / float64(result.TotalTests)
		if passRate >= 0.95 {
			status = BadgeStatusPassing
		} else if passRate >= 0.50 {
			status = BadgeStatusFailing
		} else {
			status = BadgeStatusFailing
		}
	}

	badge := &Badge{
		ProjectID:   projectID,
		Status:      status,
		PassRate:    passRate,
		TestCount:   result.TotalTests,
		LastUpdated: result.LastRun,
	}

	// Update cache
	bs.cacheMu.Lock()
	bs.cache[projectID.String()] = &cachedBadge{
		badge:     badge,
		createdAt: time.Now(),
	}
	bs.cacheMu.Unlock()

	return badge, nil
}

// GenerateSVG generates an SVG badge for a project
func (bs *BadgeService) GenerateSVG(ctx context.Context, projectID uuid.UUID) ([]byte, error) {
	// Check cache for pre-generated SVG
	bs.cacheMu.RLock()
	if cached, ok := bs.cache[projectID.String()]; ok {
		if time.Since(cached.createdAt) < bs.cacheTTL && len(cached.svg) > 0 {
			bs.cacheMu.RUnlock()
			return cached.svg, nil
		}
	}
	bs.cacheMu.RUnlock()

	badge, err := bs.GetBadge(ctx, projectID)
	if err != nil {
		return nil, err
	}

	svg := bs.renderSVG(badge)

	// Update cache with SVG
	bs.cacheMu.Lock()
	if cached, ok := bs.cache[projectID.String()]; ok {
		cached.svg = svg
	}
	bs.cacheMu.Unlock()

	return svg, nil
}

// renderSVG renders a badge as SVG
func (bs *BadgeService) renderSVG(badge *Badge) []byte {
	var statusText, statusColor string

	switch badge.Status {
	case BadgeStatusPassing:
		statusText = "passing"
		statusColor = "#4c1" // green
	case BadgeStatusFailing:
		statusText = "failing"
		statusColor = "#e05d44" // red
	case BadgeStatusPending:
		statusText = "pending"
		statusColor = "#dfb317" // yellow
	default:
		statusText = "unknown"
		statusColor = "#9f9f9f" // gray
	}

	// Add pass rate if available
	if badge.TestCount > 0 {
		statusText = fmt.Sprintf("%.0f%%", badge.PassRate*100)
	}

	data := struct {
		Label       string
		Status      string
		LabelColor  string
		StatusColor string
		LabelWidth  int
		StatusWidth int
		TotalWidth  int
	}{
		Label:       "tests",
		Status:      statusText,
		LabelColor:  "#555",
		StatusColor: statusColor,
		LabelWidth:  40,
		StatusWidth: 50,
		TotalWidth:  90,
	}

	// Calculate widths based on text length
	data.StatusWidth = len(statusText)*7 + 10
	data.TotalWidth = data.LabelWidth + data.StatusWidth

	tmpl := template.Must(template.New("badge").Parse(badgeSVGTemplate))
	var buf bytes.Buffer
	tmpl.Execute(&buf, data)

	return buf.Bytes()
}

// BadgeHandler returns an HTTP handler for badge requests
func (bs *BadgeService) BadgeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract project ID from URL
		projectIDStr := r.URL.Query().Get("project")
		if projectIDStr == "" {
			http.Error(w, "Missing project ID", http.StatusBadRequest)
			return
		}

		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			http.Error(w, "Invalid project ID", http.StatusBadRequest)
			return
		}

		svg, err := bs.GenerateSVG(r.Context(), projectID)
		if err != nil {
			bs.logger.Error("failed to generate badge", zap.Error(err))
			svg = bs.renderSVG(&Badge{Status: BadgeStatusUnknown})
		}

		// Set caching headers
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("ETag", fmt.Sprintf(`"%s"`, projectID.String()))

		w.Write(svg)
	}
}

// GenerateMarkdown returns markdown for embedding a badge
func (bs *BadgeService) GenerateMarkdown(baseURL string, projectID uuid.UUID) string {
	return fmt.Sprintf("[![TestForge](%s/badges/%s.svg)](%s/projects/%s)",
		baseURL, projectID, baseURL, projectID)
}

// GenerateHTML returns HTML for embedding a badge
func (bs *BadgeService) GenerateHTML(baseURL string, projectID uuid.UUID) string {
	return fmt.Sprintf(`<a href="%s/projects/%s"><img src="%s/badges/%s.svg" alt="TestForge Status"></a>`,
		baseURL, projectID, baseURL, projectID)
}

const badgeSVGTemplate = `<svg xmlns="http://www.w3.org/2000/svg" width="{{.TotalWidth}}" height="20">
  <linearGradient id="b" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="a">
    <rect width="{{.TotalWidth}}" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#a)">
    <path fill="{{.LabelColor}}" d="M0 0h{{.LabelWidth}}v20H0z"/>
    <path fill="{{.StatusColor}}" d="M{{.LabelWidth}} 0h{{.StatusWidth}}v20H{{.LabelWidth}}z"/>
    <path fill="url(#b)" d="M0 0h{{.TotalWidth}}v20H0z"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="{{printf "%.0f" (divf .LabelWidth 2)}}" y="15" fill="#010101" fill-opacity=".3">{{.Label}}</text>
    <text x="{{printf "%.0f" (divf .LabelWidth 2)}}" y="14">{{.Label}}</text>
    <text x="{{printf "%.0f" (addf (divf .LabelWidth 1) (divf .StatusWidth 2))}}" y="15" fill="#010101" fill-opacity=".3">{{.Status}}</text>
    <text x="{{printf "%.0f" (addf (divf .LabelWidth 1) (divf .StatusWidth 2))}}" y="14">{{.Status}}</text>
  </g>
</svg>`

func init() {
	// Register template functions
	template.Must(template.New("").Funcs(template.FuncMap{
		"divf": func(a, b int) float64 { return float64(a) / float64(b) },
		"addf": func(a, b float64) float64 { return a + b },
	}).Parse(""))
}
