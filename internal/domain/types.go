package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Common types used across domain models

// Plan represents subscription tiers
type Plan string

const (
	PlanFree       Plan = "free"
	PlanPro        Plan = "pro"
	PlanEnterprise Plan = "enterprise"
)

func (p Plan) IsValid() bool {
	switch p {
	case PlanFree, PlanPro, PlanEnterprise:
		return true
	}
	return false
}

// RunStatus represents the current state of a test run
type RunStatus string

const (
	RunStatusPending     RunStatus = "pending"
	RunStatusDiscovering RunStatus = "discovering"
	RunStatusDesigning   RunStatus = "designing"
	RunStatusAutomating  RunStatus = "automating"
	RunStatusExecuting   RunStatus = "executing"
	RunStatusHealing     RunStatus = "healing"
	RunStatusReporting   RunStatus = "reporting"
	RunStatusCompleted   RunStatus = "completed"
	RunStatusFailed      RunStatus = "failed"
	RunStatusCancelled   RunStatus = "cancelled"
)

func (s RunStatus) IsTerminal() bool {
	return s == RunStatusCompleted || s == RunStatusFailed || s == RunStatusCancelled
}

func (s RunStatus) IsValid() bool {
	switch s {
	case RunStatusPending, RunStatusDiscovering, RunStatusDesigning,
		RunStatusAutomating, RunStatusExecuting, RunStatusHealing,
		RunStatusReporting, RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	}
	return false
}

// TestCaseStatus represents the status of individual test cases
type TestCaseStatus string

const (
	TestCaseStatusPending  TestCaseStatus = "pending"
	TestCaseStatusRunning  TestCaseStatus = "running"
	TestCaseStatusPassed   TestCaseStatus = "passed"
	TestCaseStatusFailed   TestCaseStatus = "failed"
	TestCaseStatusSkipped  TestCaseStatus = "skipped"
	TestCaseStatusHealed   TestCaseStatus = "healed"
	TestCaseStatusFlaky    TestCaseStatus = "flaky"
)

// Priority for test cases
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// Timestamps provides common time fields
type Timestamps struct {
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// SetTimestamps sets CreatedAt and UpdatedAt to current time
func (t *Timestamps) SetTimestamps() {
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
}

// JSONB is a wrapper for JSON data stored in PostgreSQL JSONB columns
type JSONB map[string]any

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// NullUUID wraps uuid.UUID for nullable UUID fields
type NullUUID struct {
	UUID  uuid.UUID
	Valid bool
}

func (n NullUUID) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.UUID.String(), nil
}

func (n *NullUUID) Scan(value any) error {
	if value == nil {
		n.UUID, n.Valid = uuid.Nil, false
		return nil
	}
	n.Valid = true
	switch v := value.(type) {
	case string:
		var err error
		n.UUID, err = uuid.Parse(v)
		return err
	case []byte:
		var err error
		n.UUID, err = uuid.Parse(string(v))
		return err
	}
	return errors.New("unsupported type for NullUUID")
}
