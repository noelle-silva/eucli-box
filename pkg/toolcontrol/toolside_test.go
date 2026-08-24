package toolcontrol

import (
	"testing"
	"time"
)

func TestClampToolBudgetPrefersRequested(t *testing.T) {
	if got := ClampToolBudget(5_000, 60_000); got != 5*time.Second {
		t.Fatalf("requested budget = %v", got)
	}
}

func TestClampToolBudgetFallsBackToToolDefault(t *testing.T) {
	if got := ClampToolBudget(0, 60_000); got != 60*time.Second {
		t.Fatalf("default budget = %v", got)
	}
}

func TestClampToolBudgetCapsAtUnifiedLimit(t *testing.T) {
	if got := ClampToolBudget(900_000, 60_000); got > 300*time.Second {
		t.Fatalf("budget over cap = %v", got)
	}
	if got := ClampToolBudget(0, 900_000); got > 300*time.Second {
		t.Fatalf("default over cap = %v", got)
	}
}

func TestClampToolBudgetRejectsNonPositiveInput(t *testing.T) {
	if got := ClampToolBudget(-1, 0); got != BudgetMaxMsLimit() {
		t.Fatalf("non-positive budget = %v", got)
	}
}

func BudgetMaxMsLimit() time.Duration {
	return time.Duration(BudgetMaxMs) * time.Millisecond
}
