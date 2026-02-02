package viral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// GitHubIntegration provides GitHub integration features
type GitHubIntegration struct {
	db         *sqlx.DB
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

// NewGitHubIntegration creates a new GitHub integration
func NewGitHubIntegration(db *sqlx.DB, baseURL string, logger *zap.Logger) *GitHubIntegration {
	return &GitHubIntegration{
		db:         db,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

// PRCommentData contains data for a PR comment
type PRCommentData struct {
	RunID        uuid.UUID   `json:"run_id"`
	ProjectName  string      `json:"project_name"`
	Status       string      `json:"status"`      // passed, failed, error
	TotalTests   int         `json:"total_tests"`
	PassedTests  int         `json:"passed_tests"`
	FailedTests  int         `json:"failed_tests"`
	SkippedTests int         `json:"skipped_tests"`
	Duration     string      `json:"duration"`
	FailedSuites []FailedSuite `json:"failed_suites,omitempty"`
	ReportURL    string      `json:"report_url"`
	BadgeURL     string      `json:"badge_url"`
}

// FailedSuite represents a failed test suite
type FailedSuite struct {
	Name       string   `json:"name"`
	FailedTests []string `json:"failed_tests"`
}

// GeneratePRComment generates a GitHub PR comment body
func (gh *GitHubIntegration) GeneratePRComment(data PRCommentData) (string, error) {
	tmpl, err := template.New("pr-comment").Parse(prCommentTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	// Calculate status emoji
	data.Status = gh.getStatusEmoji(data.PassedTests, data.TotalTests, data.FailedTests)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// PostPRComment posts a comment to a GitHub PR
func (gh *GitHubIntegration) PostPRComment(ctx context.Context, req PRCommentRequest) error {
	comment, err := gh.GeneratePRComment(req.Data)
	if err != nil {
		return fmt.Errorf("generating comment: %w", err)
	}

	// Check for existing TestForge comment
	existingCommentID, err := gh.findExistingComment(ctx, req)
	if err != nil {
		gh.logger.Warn("failed to find existing comment", zap.Error(err))
	}

	if existingCommentID != "" {
		// Update existing comment
		return gh.updateComment(ctx, req, existingCommentID, comment)
	}

	// Create new comment
	return gh.createComment(ctx, req, comment)
}

// PRCommentRequest represents a request to post a PR comment
type PRCommentRequest struct {
	Owner       string        `json:"owner"`
	Repo        string        `json:"repo"`
	PRNumber    int           `json:"pr_number"`
	AccessToken string        `json:"access_token"`
	Data        PRCommentData `json:"data"`
}

func (gh *GitHubIntegration) findExistingComment(ctx context.Context, req PRCommentRequest) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments",
		req.Owner, req.Repo, req.PRNumber)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Authorization", "Bearer "+req.AccessToken)
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := gh.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var comments []struct {
		ID   int    `json:"id"`
		Body string `json:"body"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return "", err
	}

	// Look for TestForge comment marker
	for _, c := range comments {
		if len(c.Body) > 20 && c.Body[:20] == "## TestForge Results" {
			return fmt.Sprintf("%d", c.ID), nil
		}
	}

	return "", nil
}

func (gh *GitHubIntegration) createComment(ctx context.Context, req PRCommentRequest, body string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments",
		req.Owner, req.Repo, req.PRNumber)

	payload, _ := json.Marshal(map[string]string{"body": body})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Authorization", "Bearer "+req.AccessToken)
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := gh.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	return nil
}

func (gh *GitHubIntegration) updateComment(ctx context.Context, req PRCommentRequest, commentID, body string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments/%s",
		req.Owner, req.Repo, commentID)

	payload, _ := json.Marshal(map[string]string{"body": body})

	httpReq, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Authorization", "Bearer "+req.AccessToken)
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := gh.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	return nil
}

// CreateCheckRun creates a GitHub check run
func (gh *GitHubIntegration) CreateCheckRun(ctx context.Context, req CheckRunRequest) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/check-runs",
		req.Owner, req.Repo)

	conclusion := "success"
	if req.FailedTests > 0 {
		conclusion = "failure"
	}

	payload := map[string]interface{}{
		"name":        "TestForge",
		"head_sha":    req.CommitSHA,
		"status":      "completed",
		"conclusion":  conclusion,
		"started_at":  req.StartedAt.Format(time.RFC3339),
		"completed_at": time.Now().Format(time.RFC3339),
		"output": map[string]interface{}{
			"title":   fmt.Sprintf("Tests: %d passed, %d failed", req.PassedTests, req.FailedTests),
			"summary": gh.generateCheckSummary(req),
		},
		"details_url": req.ReportURL,
	}

	payloadBytes, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Authorization", "Bearer "+req.AccessToken)
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := gh.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	return nil
}

// CheckRunRequest represents a request to create a check run
type CheckRunRequest struct {
	Owner       string    `json:"owner"`
	Repo        string    `json:"repo"`
	CommitSHA   string    `json:"commit_sha"`
	AccessToken string    `json:"access_token"`
	StartedAt   time.Time `json:"started_at"`
	TotalTests  int       `json:"total_tests"`
	PassedTests int       `json:"passed_tests"`
	FailedTests int       `json:"failed_tests"`
	Duration    string    `json:"duration"`
	ReportURL   string    `json:"report_url"`
}

func (gh *GitHubIntegration) generateCheckSummary(req CheckRunRequest) string {
	passRate := float64(req.PassedTests) / float64(req.TotalTests) * 100
	return fmt.Sprintf(`## Test Results

| Metric | Value |
|--------|-------|
| Total Tests | %d |
| Passed | %d (%.1f%%) |
| Failed | %d |
| Duration | %s |

[View Full Report](%s)`,
		req.TotalTests,
		req.PassedTests, passRate,
		req.FailedTests,
		req.Duration,
		req.ReportURL,
	)
}

func (gh *GitHubIntegration) getStatusEmoji(passed, total, failed int) string {
	if failed == 0 {
		return "✅"
	}
	passRate := float64(passed) / float64(total)
	if passRate >= 0.8 {
		return "⚠️"
	}
	return "❌"
}

// GenerateGitHubActionYAML generates a GitHub Action workflow file
func (gh *GitHubIntegration) GenerateGitHubActionYAML(projectID uuid.UUID) string {
	return fmt.Sprintf(`name: TestForge Tests

on:
  push:
    branches: [ main, master ]
  pull_request:
    branches: [ main, master ]

jobs:
  testforge:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run TestForge Tests
        uses: testforge/action@v1
        with:
          api-key: ${{ secrets.TESTFORGE_API_KEY }}
          project-id: %s
          wait-for-results: true
          post-pr-comment: true

      - name: Upload Test Artifacts
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: testforge-results
          path: testforge-results/
`, projectID)
}

const prCommentTemplate = `## TestForge Results {{.Status}}

| Suite | Passed | Failed | Duration |
|-------|--------|--------|----------|
{{- range .FailedSuites}}
| {{.Name}} | - | {{len .FailedTests}} | - |
{{- end}}
| **Total** | **{{.PassedTests}}** | **{{.FailedTests}}** | **{{.Duration}}** |

{{if .FailedSuites}}
### Failed Tests
{{range .FailedSuites}}
<details>
<summary>{{.Name}} ({{len .FailedTests}} failed)</summary>

{{range .FailedTests}}
- ❌ {{.}}
{{end}}

</details>
{{end}}
{{end}}

[View Full Report]({{.ReportURL}})

---
🤖 Generated with [Claude Code](https://claude.ai/code) | [![TestForge]({{.BadgeURL}})]({{.ReportURL}})
`
