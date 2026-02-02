package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewTestRun(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	run := NewTestRun(tenantID, projectID, "https://target.example.com", "user:123")

	if run.ID.String() == "" {
		t.Error("ID should not be empty")
	}
	if run.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", run.TenantID, tenantID)
	}
	if run.ProjectID != projectID {
		t.Errorf("ProjectID = %v, want %v", run.ProjectID, projectID)
	}
	if run.TargetURL != "https://target.example.com" {
		t.Errorf("TargetURL = %q, want 'https://target.example.com'", run.TargetURL)
	}
	if run.TriggeredBy != "user:123" {
		t.Errorf("TriggeredBy = %q, want 'user:123'", run.TriggeredBy)
	}
	if run.Status != RunStatusPending {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusPending)
	}
	if run.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if run.StartedAt != nil {
		t.Error("StartedAt should be nil")
	}
	if run.CompletedAt != nil {
		t.Error("CompletedAt should be nil")
	}
}

func TestTestRun_SetWorkflowInfo(t *testing.T) {
	run := NewTestRun(uuid.New(), uuid.New(), "https://example.com", "api")
	oldUpdatedAt := run.UpdatedAt

	time.Sleep(time.Millisecond) // Ensure time difference
	run.SetWorkflowInfo("workflow-123", "run-456")

	if run.WorkflowID != "workflow-123" {
		t.Errorf("WorkflowID = %q, want 'workflow-123'", run.WorkflowID)
	}
	if run.WorkflowRunID != "run-456" {
		t.Errorf("WorkflowRunID = %q, want 'run-456'", run.WorkflowRunID)
	}
	if !run.UpdatedAt.After(oldUpdatedAt) {
		t.Error("UpdatedAt should be updated")
	}
}

func TestTestRun_Start(t *testing.T) {
	run := NewTestRun(uuid.New(), uuid.New(), "https://example.com", "api")
	oldUpdatedAt := run.UpdatedAt

	if run.StartedAt != nil {
		t.Error("StartedAt should be nil before Start()")
	}

	time.Sleep(time.Millisecond) // Ensure time difference
	run.Start()

	if run.StartedAt == nil {
		t.Error("StartedAt should not be nil after Start()")
	}
	if !run.UpdatedAt.After(oldUpdatedAt) {
		t.Error("UpdatedAt should be updated")
	}
}

func TestTestRun_Complete(t *testing.T) {
	run := NewTestRun(uuid.New(), uuid.New(), "https://example.com", "api")
	run.Status = RunStatusExecuting
	oldUpdatedAt := run.UpdatedAt

	summary := &RunSummary{
		TotalTests: 10,
		Passed:     8,
		Failed:     2,
		Duration:   time.Minute,
		PassRate:   0.8,
	}

	time.Sleep(time.Millisecond) // Ensure time difference
	run.Complete(summary, "https://reports.example.com/123")

	if run.Status != RunStatusCompleted {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusCompleted)
	}
	if run.Summary != summary {
		t.Error("Summary should be set")
	}
	if run.ReportURL != "https://reports.example.com/123" {
		t.Errorf("ReportURL = %q, want 'https://reports.example.com/123'", run.ReportURL)
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt should not be nil after Complete()")
	}
	if !run.UpdatedAt.After(oldUpdatedAt) {
		t.Error("UpdatedAt should be updated")
	}
}

func TestTestRun_Fail(t *testing.T) {
	run := NewTestRun(uuid.New(), uuid.New(), "https://example.com", "api")
	run.Status = RunStatusExecuting
	oldUpdatedAt := run.UpdatedAt

	time.Sleep(time.Millisecond) // Ensure time difference
	run.Fail("Execution error occurred")

	if run.Status != RunStatusFailed {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusFailed)
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt should not be nil after Fail()")
	}
	if !run.UpdatedAt.After(oldUpdatedAt) {
		t.Error("UpdatedAt should be updated")
	}
}

func TestTestRun_Lifecycle(t *testing.T) {
	// Simulate full lifecycle
	run := NewTestRun(uuid.New(), uuid.New(), "https://example.com", "schedule")

	// Initial state
	if run.Status != RunStatusPending {
		t.Errorf("Initial status = %v, want %v", run.Status, RunStatusPending)
	}

	// Set workflow info
	run.SetWorkflowInfo("wf-123", "run-123")
	if run.WorkflowID != "wf-123" {
		t.Error("WorkflowID not set")
	}

	// Start
	run.Start()
	if run.StartedAt == nil {
		t.Error("StartedAt not set after Start()")
	}

	// Complete
	summary := &RunSummary{TotalTests: 5, Passed: 5, PassRate: 1.0}
	run.Complete(summary, "https://report.url")

	if run.Status != RunStatusCompleted {
		t.Errorf("Final status = %v, want %v", run.Status, RunStatusCompleted)
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt not set after Complete()")
	}
	if run.Summary == nil {
		t.Error("Summary not set")
	}
}

func TestRunSummary(t *testing.T) {
	summary := &RunSummary{
		TotalTests: 100,
		Passed:     85,
		Failed:     10,
		Skipped:    3,
		Healed:     2,
		Flaky:      1,
		Duration:   5 * time.Minute,
		PassRate:   0.85,
		HealRate:   0.02,
	}

	if summary.TotalTests != 100 {
		t.Errorf("TotalTests = %d, want 100", summary.TotalTests)
	}
	if summary.Passed != 85 {
		t.Errorf("Passed = %d, want 85", summary.Passed)
	}
	if summary.Failed != 10 {
		t.Errorf("Failed = %d, want 10", summary.Failed)
	}
	if summary.PassRate != 0.85 {
		t.Errorf("PassRate = %f, want 0.85", summary.PassRate)
	}
}
