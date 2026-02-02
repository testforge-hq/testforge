package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewReport(t *testing.T) {
	testRunID := uuid.New()
	tenantID := uuid.New()
	summary := ReportSummary{
		TestRunID:   testRunID,
		ProjectName: "Test Project",
		TargetURL:   "https://example.com",
		TotalTests:  100,
		Passed:      95,
		Failed:      5,
		PassRate:    95.0,
	}

	report := NewReport(
		testRunID,
		tenantID,
		ReportTypeFull,
		ReportFormatHTML,
		"https://reports.example.com/123",
		12345,
		summary,
	)

	if report.ID == uuid.Nil {
		t.Error("ID should not be nil")
	}
	if report.TestRunID != testRunID {
		t.Errorf("TestRunID = %v, want %v", report.TestRunID, testRunID)
	}
	if report.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", report.TenantID, tenantID)
	}
	if report.Type != ReportTypeFull {
		t.Errorf("Type = %v, want %v", report.Type, ReportTypeFull)
	}
	if report.Format != ReportFormatHTML {
		t.Errorf("Format = %v, want %v", report.Format, ReportFormatHTML)
	}
	if report.URL != "https://reports.example.com/123" {
		t.Errorf("URL = %v, want %v", report.URL, "https://reports.example.com/123")
	}
	if report.Size != 12345 {
		t.Errorf("Size = %v, want %v", report.Size, 12345)
	}
	if report.Summary.TotalTests != 100 {
		t.Errorf("Summary.TotalTests = %v, want %v", report.Summary.TotalTests, 100)
	}
	if report.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
	if report.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if report.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestReportTypes(t *testing.T) {
	types := []ReportType{
		ReportTypeFull,
		ReportTypeSummary,
		ReportTypeFailures,
		ReportTypeHealing,
		ReportTypeComparison,
	}

	for _, rt := range types {
		if rt == "" {
			t.Error("ReportType should not be empty")
		}
	}
}

func TestReportFormats(t *testing.T) {
	formats := []ReportFormat{
		ReportFormatHTML,
		ReportFormatPDF,
		ReportFormatJSON,
		ReportFormatJUnit,
	}

	for _, rf := range formats {
		if rf == "" {
			t.Error("ReportFormat should not be empty")
		}
	}
}

func TestReportSummary(t *testing.T) {
	summary := ReportSummary{
		TestRunID:   uuid.New(),
		ProjectName: "Test Project",
		TargetURL:   "https://example.com",
		ExecutedAt:  time.Now(),
		Duration:    5 * time.Minute,
		TotalTests:  100,
		Passed:      80,
		Failed:      15,
		Skipped:     5,
		Healed:      3,
		PassRate:    80.0,
		ByCategory: map[string]CategoryStats{
			"smoke": {Total: 10, Passed: 10, Failed: 0, PassRate: 100.0},
			"e2e":   {Total: 90, Passed: 70, Failed: 15, PassRate: 77.8},
		},
		ByPriority: map[Priority]PriorityStats{
			PriorityHigh:   {Total: 20, Passed: 18, Failed: 2, PassRate: 90.0},
			PriorityMedium: {Total: 80, Passed: 62, Failed: 13, PassRate: 77.5},
		},
	}

	if summary.TotalTests != 100 {
		t.Errorf("TotalTests = %v, want %v", summary.TotalTests, 100)
	}
	if summary.PassRate != 80.0 {
		t.Errorf("PassRate = %v, want %v", summary.PassRate, 80.0)
	}
	if len(summary.ByCategory) != 2 {
		t.Errorf("ByCategory length = %v, want %v", len(summary.ByCategory), 2)
	}
	if len(summary.ByPriority) != 2 {
		t.Errorf("ByPriority length = %v, want %v", len(summary.ByPriority), 2)
	}
}
