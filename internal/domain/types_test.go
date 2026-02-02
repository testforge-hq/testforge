package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestPlan_IsValid(t *testing.T) {
	tests := []struct {
		plan  Plan
		valid bool
	}{
		{PlanFree, true},
		{PlanPro, true},
		{PlanEnterprise, true},
		{Plan("invalid"), false},
		{Plan(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.plan), func(t *testing.T) {
			if got := tt.plan.IsValid(); got != tt.valid {
				t.Errorf("Plan(%q).IsValid() = %v, want %v", tt.plan, got, tt.valid)
			}
		})
	}
}

func TestRunStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   RunStatus
		terminal bool
	}{
		{RunStatusPending, false},
		{RunStatusDiscovering, false},
		{RunStatusDesigning, false},
		{RunStatusAutomating, false},
		{RunStatusExecuting, false},
		{RunStatusHealing, false},
		{RunStatusReporting, false},
		{RunStatusCompleted, true},
		{RunStatusFailed, true},
		{RunStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.terminal {
				t.Errorf("RunStatus(%q).IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
			}
		})
	}
}

func TestRunStatus_IsValid(t *testing.T) {
	tests := []struct {
		status RunStatus
		valid  bool
	}{
		{RunStatusPending, true},
		{RunStatusDiscovering, true},
		{RunStatusDesigning, true},
		{RunStatusAutomating, true},
		{RunStatusExecuting, true},
		{RunStatusHealing, true},
		{RunStatusReporting, true},
		{RunStatusCompleted, true},
		{RunStatusFailed, true},
		{RunStatusCancelled, true},
		{RunStatus("invalid"), false},
		{RunStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.valid {
				t.Errorf("RunStatus(%q).IsValid() = %v, want %v", tt.status, got, tt.valid)
			}
		})
	}
}

func TestJSONB_Value(t *testing.T) {
	t.Run("nil JSONB", func(t *testing.T) {
		var j JSONB
		val, err := j.Value()
		if err != nil {
			t.Errorf("Value() error = %v", err)
		}
		if val != nil {
			t.Errorf("Value() = %v, want nil", val)
		}
	})

	t.Run("non-nil JSONB", func(t *testing.T) {
		j := JSONB{"key": "value", "num": 42}
		val, err := j.Value()
		if err != nil {
			t.Errorf("Value() error = %v", err)
		}
		if val == nil {
			t.Error("Value() should not be nil")
		}
	})
}

func TestJSONB_Scan(t *testing.T) {
	t.Run("nil value", func(t *testing.T) {
		var j JSONB
		err := j.Scan(nil)
		if err != nil {
			t.Errorf("Scan(nil) error = %v", err)
		}
		if j != nil {
			t.Errorf("Scan(nil) should result in nil JSONB")
		}
	})

	t.Run("valid JSON bytes", func(t *testing.T) {
		var j JSONB
		err := j.Scan([]byte(`{"key": "value"}`))
		if err != nil {
			t.Errorf("Scan() error = %v", err)
		}
		if j["key"] != "value" {
			t.Errorf("Scan() key = %v, want 'value'", j["key"])
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		var j JSONB
		err := j.Scan(123)
		if err == nil {
			t.Error("Scan(int) should return error")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		var j JSONB
		err := j.Scan([]byte(`{invalid json}`))
		if err == nil {
			t.Error("Scan(invalid JSON) should return error")
		}
	})
}

func TestNullUUID_Value(t *testing.T) {
	t.Run("invalid NullUUID", func(t *testing.T) {
		n := NullUUID{Valid: false}
		val, err := n.Value()
		if err != nil {
			t.Errorf("Value() error = %v", err)
		}
		if val != nil {
			t.Errorf("Value() = %v, want nil", val)
		}
	})

	t.Run("valid NullUUID", func(t *testing.T) {
		id := uuid.New()
		n := NullUUID{UUID: id, Valid: true}
		val, err := n.Value()
		if err != nil {
			t.Errorf("Value() error = %v", err)
		}
		if val != id.String() {
			t.Errorf("Value() = %v, want %v", val, id.String())
		}
	})
}

func TestNullUUID_Scan(t *testing.T) {
	t.Run("nil value", func(t *testing.T) {
		var n NullUUID
		err := n.Scan(nil)
		if err != nil {
			t.Errorf("Scan(nil) error = %v", err)
		}
		if n.Valid {
			t.Error("Scan(nil) should set Valid to false")
		}
	})

	t.Run("string value", func(t *testing.T) {
		id := uuid.New()
		var n NullUUID
		err := n.Scan(id.String())
		if err != nil {
			t.Errorf("Scan(string) error = %v", err)
		}
		if !n.Valid {
			t.Error("Scan(string) should set Valid to true")
		}
		if n.UUID != id {
			t.Errorf("Scan(string) UUID = %v, want %v", n.UUID, id)
		}
	})

	t.Run("bytes value", func(t *testing.T) {
		id := uuid.New()
		var n NullUUID
		err := n.Scan([]byte(id.String()))
		if err != nil {
			t.Errorf("Scan([]byte) error = %v", err)
		}
		if !n.Valid {
			t.Error("Scan([]byte) should set Valid to true")
		}
		if n.UUID != id {
			t.Errorf("Scan([]byte) UUID = %v, want %v", n.UUID, id)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		var n NullUUID
		err := n.Scan(123)
		if err == nil {
			t.Error("Scan(int) should return error")
		}
	})

	t.Run("invalid UUID string", func(t *testing.T) {
		var n NullUUID
		err := n.Scan("not-a-uuid")
		if err == nil {
			t.Error("Scan(invalid UUID) should return error")
		}
	})
}
