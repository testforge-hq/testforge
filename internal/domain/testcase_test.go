package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewTestCase(t *testing.T) {
	testRunID := uuid.New()
	tenantID := uuid.New()
	steps := []TestStep{
		{Order: 1, Type: "given", Action: "I am on the login page"},
		{Order: 2, Type: "when", Action: "I enter valid credentials"},
		{Order: 3, Type: "then", Action: "I should see the dashboard"},
	}

	tc := NewTestCase(
		testRunID,
		tenantID,
		"Login Test",
		"Test user login functionality",
		"smoke",
		PriorityHigh,
		steps,
	)

	if tc.ID == uuid.Nil {
		t.Error("ID should not be nil")
	}
	if tc.TestRunID != testRunID {
		t.Errorf("TestRunID = %v, want %v", tc.TestRunID, testRunID)
	}
	if tc.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", tc.TenantID, tenantID)
	}
	if tc.Name != "Login Test" {
		t.Errorf("Name = %v, want %v", tc.Name, "Login Test")
	}
	if tc.Description != "Test user login functionality" {
		t.Errorf("Description = %v, want %v", tc.Description, "Test user login functionality")
	}
	if tc.Category != "smoke" {
		t.Errorf("Category = %v, want %v", tc.Category, "smoke")
	}
	if tc.Priority != PriorityHigh {
		t.Errorf("Priority = %v, want %v", tc.Priority, PriorityHigh)
	}
	if tc.Status != TestCaseStatusPending {
		t.Errorf("Status = %v, want %v", tc.Status, TestCaseStatusPending)
	}
	if len(tc.Steps) != 3 {
		t.Errorf("Steps length = %v, want %v", len(tc.Steps), 3)
	}
	if tc.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestTestCase_MarkPassed(t *testing.T) {
	tc := NewTestCase(
		uuid.New(),
		uuid.New(),
		"Test",
		"Description",
		"smoke",
		PriorityMedium,
		nil,
	)

	startTime := time.Now().Add(-5 * time.Second)
	endTime := time.Now()
	result := &ExecutionResult{
		Passed:      true,
		StartedAt:   startTime,
		CompletedAt: endTime,
	}

	tc.MarkPassed(result)

	if tc.Status != TestCaseStatusPassed {
		t.Errorf("Status = %v, want %v", tc.Status, TestCaseStatusPassed)
	}
	if tc.ExecutionResult != result {
		t.Error("ExecutionResult should be set")
	}
	if tc.Duration < 4*time.Second || tc.Duration > 6*time.Second {
		t.Errorf("Duration = %v, expected around 5s", tc.Duration)
	}
}

func TestTestCase_MarkFailed(t *testing.T) {
	tc := NewTestCase(
		uuid.New(),
		uuid.New(),
		"Test",
		"Description",
		"e2e",
		PriorityHigh,
		nil,
	)

	startTime := time.Now().Add(-10 * time.Second)
	endTime := time.Now()
	failedStep := 2
	result := &ExecutionResult{
		Passed:       false,
		ErrorMessage: "Element not found",
		FailedStep:   &failedStep,
		StartedAt:    startTime,
		CompletedAt:  endTime,
	}

	tc.MarkFailed(result)

	if tc.Status != TestCaseStatusFailed {
		t.Errorf("Status = %v, want %v", tc.Status, TestCaseStatusFailed)
	}
	if tc.ExecutionResult.ErrorMessage != "Element not found" {
		t.Errorf("ErrorMessage = %v, want %v", tc.ExecutionResult.ErrorMessage, "Element not found")
	}
	if *tc.ExecutionResult.FailedStep != 2 {
		t.Errorf("FailedStep = %v, want %v", *tc.ExecutionResult.FailedStep, 2)
	}
}

func TestTestCase_MarkHealed(t *testing.T) {
	tc := NewTestCase(
		uuid.New(),
		uuid.New(),
		"Test",
		"Description",
		"regression",
		PriorityMedium,
		nil,
	)

	startTime := time.Now().Add(-3 * time.Second)
	endTime := time.Now()
	result := &ExecutionResult{
		Passed:      true,
		StartedAt:   startTime,
		CompletedAt: endTime,
	}

	healing := HealingRecord{
		Timestamp:        time.Now(),
		OriginalSelector: "#old-button",
		HealedSelector:   "[data-testid='button']",
		Strategy:         "attribute",
		Confidence:       0.95,
		Successful:       true,
	}

	tc.MarkHealed(result, healing)

	if tc.Status != TestCaseStatusHealed {
		t.Errorf("Status = %v, want %v", tc.Status, TestCaseStatusHealed)
	}
	if len(tc.HealingHistory) != 1 {
		t.Errorf("HealingHistory length = %v, want %v", len(tc.HealingHistory), 1)
	}
	if tc.HealingHistory[0].OriginalSelector != "#old-button" {
		t.Errorf("OriginalSelector = %v, want %v", tc.HealingHistory[0].OriginalSelector, "#old-button")
	}
	if tc.HealingHistory[0].Confidence != 0.95 {
		t.Errorf("Confidence = %v, want %v", tc.HealingHistory[0].Confidence, 0.95)
	}
}

func TestTestStep(t *testing.T) {
	step := TestStep{
		Order:      1,
		Type:       "when",
		Action:     "I click the submit button",
		Target:     "submit button",
		Selector:   "#submit",
		Screenshot: true,
		Assertion: &Assertion{
			Type:     "visible",
			Expected: "true",
		},
	}

	if step.Order != 1 {
		t.Errorf("Order = %v, want %v", step.Order, 1)
	}
	if step.Type != "when" {
		t.Errorf("Type = %v, want %v", step.Type, "when")
	}
	if step.Selector != "#submit" {
		t.Errorf("Selector = %v, want %v", step.Selector, "#submit")
	}
	if !step.Screenshot {
		t.Error("Screenshot should be true")
	}
	if step.Assertion == nil {
		t.Error("Assertion should not be nil")
	}
}

func TestExecutionResult(t *testing.T) {
	screenshots := []Screenshot{
		{Name: "step1", URL: "https://example.com/1.png", Step: 1, Timestamp: time.Now()},
	}

	result := ExecutionResult{
		Passed:       true,
		Screenshots:  screenshots,
		VideoURL:     "https://example.com/video.mp4",
		TraceURL:     "https://example.com/trace.zip",
		StartedAt:    time.Now().Add(-5 * time.Second),
		CompletedAt:  time.Now(),
	}

	if !result.Passed {
		t.Error("Passed should be true")
	}
	if len(result.Screenshots) != 1 {
		t.Errorf("Screenshots length = %v, want %v", len(result.Screenshots), 1)
	}
	if result.VideoURL != "https://example.com/video.mp4" {
		t.Errorf("VideoURL = %v, want %v", result.VideoURL, "https://example.com/video.mp4")
	}
}

func TestHealingRecord(t *testing.T) {
	record := HealingRecord{
		Timestamp:        time.Now(),
		OriginalSelector: ".old-class",
		HealedSelector:   "[data-id='new']",
		Strategy:         "visual",
		Confidence:       0.87,
		Successful:       true,
	}

	if record.OriginalSelector != ".old-class" {
		t.Errorf("OriginalSelector = %v, want %v", record.OriginalSelector, ".old-class")
	}
	if record.Strategy != "visual" {
		t.Errorf("Strategy = %v, want %v", record.Strategy, "visual")
	}
	if record.Confidence != 0.87 {
		t.Errorf("Confidence = %v, want %v", record.Confidence, 0.87)
	}
}
