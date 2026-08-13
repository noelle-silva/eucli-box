package datamigration

import (
	"context"
	"errors"
	"testing"

	apperrors "eucli-box/pkg/errors"
)

func noopFunc(ctx context.Context, dataDir string) error { return nil }

func resetRegistry() {
	registry = map[string]Step{}
}

func validStep() Step {
	return Step{
		ID:          "1.0.0-to-1.1.0",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Scope:       []string{"meta/counter.json"},
		Precheck:    noopFunc,
		Apply:       noopFunc,
		Verify:      noopFunc,
	}
}

func registerForTest(t *testing.T, step Step) {
	t.Helper()
	if err := Register(step); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func assertMigrationErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not AppError", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
}

func TestRegisterAcceptsValidStep(t *testing.T) {
	resetRegistry()
	registerForTest(t, validStep())
}

func TestRegisterRejectsEmptyID(t *testing.T) {
	step := validStep()
	step.ID = "  "
	err := Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")
}

func TestRegisterRejectsDuplicateID(t *testing.T) {
	resetRegistry()
	registerForTest(t, validStep())
	err := Register(validStep())
	assertMigrationErrorCode(t, err, "migration.invalid_request")
}

func TestRegisterRejectsInvalidVersions(t *testing.T) {
	step := validStep()
	step.ID = "bad-from"
	step.FromVersion = "1.0"
	err := Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")

	step = validStep()
	step.ID = "bad-to"
	step.ToVersion = "1.1.x"
	err = Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")
}

func TestRegisterRejectsNonForwardStep(t *testing.T) {
	step := validStep()
	step.ID = "backwards"
	step.FromVersion = "1.1.0"
	step.ToVersion = "1.0.0"
	err := Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")
}

func TestRegisterRejectsEmptyScope(t *testing.T) {
	step := validStep()
	step.ID = "no-scope"
	step.Scope = nil
	err := Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")
}

func TestRegisterRejectsUnsafeScopeEntries(t *testing.T) {
	for _, entry := range []string{"../meta/counter.json", "meta/../counter.json", "meta//counter.json", "/meta/counter.json", "meta/counter.json/"} {
		step := validStep()
		step.ID = "scope-" + entry
		step.Scope = []string{entry}
		err := Register(step)
		if err == nil {
			t.Fatalf("scope entry %q was accepted", entry)
		}
		assertMigrationErrorCode(t, err, "migration.invalid_request")
	}
}

func TestRegisterNormalizesBackslashScope(t *testing.T) {
	step := validStep()
	step.ID = "backslash-scope"
	step.Scope = []string{`meta\counter.json`}
	registerForTest(t, step)
	steps := registeredSteps()
	for _, registered := range steps {
		if registered.ID == step.ID && registered.Scope[0] != "meta/counter.json" {
			t.Fatalf("scope was not normalized: %#v", registered.Scope)
		}
	}
}

func TestRegisterRejectsNilFunctions(t *testing.T) {
	step := validStep()
	step.ID = "nil-precheck"
	step.Precheck = nil
	err := Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")

	step = validStep()
	step.ID = "nil-apply"
	step.Apply = nil
	err = Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")

	step = validStep()
	step.ID = "nil-verify"
	step.Verify = nil
	err = Register(step)
	assertMigrationErrorCode(t, err, "migration.invalid_request")
}

func TestStateVocabularyIsFixed(t *testing.T) {
	expected := map[State]bool{
		StateDataUnchanged:  true,
		StateMigrated:       true,
		StateRecovered:      true,
		StateRecoveryFailed: true,
	}
	if len(expected) != 4 {
		t.Fatalf("state vocabulary must have exactly four words")
	}
	for _, state := range []State{StateDataUnchanged, StateMigrated, StateRecovered, StateRecoveryFailed} {
		if string(state) == "" {
			t.Fatalf("state word must not be empty")
		}
	}
}

func TestRegisteredStepsAreOrderedByFromVersion(t *testing.T) {
	resetRegistry()
	first := validStep()
	first.ID = "1.0.0-to-1.1.0"
	first.FromVersion = "1.0.0"
	first.ToVersion = "1.1.0"
	registerForTest(t, first)
	second := validStep()
	second.ID = "1.1.0-to-1.2.0"
	second.FromVersion = "1.1.0"
	second.ToVersion = "1.2.0"
	registerForTest(t, second)
	steps := registeredSteps()
	if len(steps) < 2 || steps[len(steps)-2].ID != "1.0.0-to-1.1.0" || steps[len(steps)-1].ID != "1.1.0-to-1.2.0" {
		t.Fatalf("steps are not ordered by from version: %#v", steps)
	}
}
