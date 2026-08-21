package installsource

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type fakeStore struct {
	current  Kind
	saved    Kind
	loadErr  error
	saveErr  error
	saveCall int
}

func (s *fakeStore) LoadInstallSource(context.Context) (Kind, error) {
	if s.loadErr != nil {
		return "", s.loadErr
	}
	return s.current, nil
}

func (s *fakeStore) SaveInstallSource(_ context.Context, kind Kind) error {
	s.saveCall++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = kind
	return nil
}

func TestParseKind(t *testing.T) {
	cases := []struct {
		input    string
		want     Kind
		wantErr  bool
	}{
		{input: "official", want: KindOfficial},
		{input: "development", want: KindDevelopment},
		{input: "  official  ", want: KindOfficial},
		{input: "", wantErr: true},
		{input: "internal", wantErr: true},
		{input: "official; drop tables", wantErr: true},
	}
	for _, item := range cases {
		kind, err := ParseKind(item.input)
		if item.wantErr {
			if err == nil {
				t.Fatalf("ParseKind(%q) error = nil, want error", item.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseKind(%q) error = %v", item.input, err)
		}
		if kind != item.want {
			t.Fatalf("ParseKind(%q) = %q, want %q", item.input, kind, item.want)
		}
	}
}

func TestNewStateInvalidInitial(t *testing.T) {
	if _, err := NewState(Kind("whatever"), true, nil); err == nil {
		t.Fatal("NewState() error = nil, want invalid initial error")
	}
}

func TestStateSetPersistsAndUpdatesMemory(t *testing.T) {
	store := &fakeStore{current: KindOfficial}
	state, err := NewState(KindOfficial, true, store)
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	next, err := state.Set(context.Background(), KindDevelopment)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if next != KindDevelopment {
		t.Fatalf("Set() = %q, want %q", next, KindDevelopment)
	}
	if store.saveCall != 1 || store.saved != KindDevelopment {
		t.Fatalf("store save = %v/%q, want 1 call of %q", store.saveCall, store.saved, KindDevelopment)
	}
	if state.Current() != KindDevelopment {
		t.Fatalf("Current() = %q, want %q", state.Current(), KindDevelopment)
	}
}

func TestStateSetInvalidKind(t *testing.T) {
	state, err := NewState(KindOfficial, true, &fakeStore{})
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	if _, err := state.Set(context.Background(), Kind("internal")); err == nil {
		t.Fatal("Set() error = nil, want invalid kind error")
	}
	if state.Current() != KindOfficial {
		t.Fatalf("Current() = %q, want %q (unchanged)", state.Current(), KindOfficial)
	}
}

func TestStateSetImmutableRejects(t *testing.T) {
	store := &fakeStore{}
	state, err := NewState(KindOfficial, false, store)
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	_, err = state.Set(context.Background(), KindDevelopment)
	if err == nil {
		t.Fatal("Set() error = nil, want immutable rejection")
	}
	if store.saveCall != 0 {
		t.Fatalf("store save called %d times, want 0", store.saveCall)
	}
}

func TestStateSetPersistFailureKeepsCurrent(t *testing.T) {
	store := &fakeStore{saveErr: errors.New("disk full")}
	state, err := NewState(KindOfficial, true, store)
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	if _, err := state.Set(context.Background(), KindDevelopment); err == nil {
		t.Fatal("Set() error = nil, want persist failure")
	}
	if state.Current() != KindOfficial {
		t.Fatalf("Current() = %q, want %q (unchanged on persist failure)", state.Current(), KindOfficial)
	}
}

type stubCandidate struct {
	kind     string
	called   int
	candidate *releasecheck.ReleaseCandidate
	err      error
}

func (s *stubCandidate) LatestCandidate(context.Context, types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	return s.candidate, nil
}

func TestCandidateSelectorForwardsByCurrentState(t *testing.T) {
	official := &stubCandidate{kind: "official"}
	dev := &stubCandidate{kind: "dev"}
	current := KindOfficial
	selector, err := NewCandidateSelector(func() Kind { return current }, official, dev)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: "tool", ID: "shell_command"}
	if _, err := selector.LatestCandidate(context.Background(), identity); err != nil {
		t.Fatalf("official LatestCandidate() error = %v", err)
	}
	if official.called != 1 || dev.called != 0 {
		t.Fatalf("called official=%d dev=%d, want 1/0", official.called, dev.called)
	}
	current = KindDevelopment
	if _, err := selector.LatestCandidate(context.Background(), identity); err != nil {
		t.Fatalf("development LatestCandidate() error = %v", err)
	}
	if official.called != 1 || dev.called != 1 {
		t.Fatalf("called official=%d dev=%d, want 1/1", official.called, dev.called)
	}
}

func TestCandidateSelectorDevelopmentMissingReaderFailsFast(t *testing.T) {
	official := &stubCandidate{}
	selector, err := NewCandidateSelector(func() Kind { return KindDevelopment }, official, nil)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	_, err = selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: "plugin", ID: "weather"})
	if err == nil {
		t.Fatal("LatestCandidate() error = nil, want development missing error")
	}
	if official.called != 0 {
		t.Fatalf("official reader called %d times, want 0 (no fallback)", official.called)
	}
}

func TestCandidateSelectorPropagatesReaderError(t *testing.T) {
	expected := errors.New("network down")
	official := &stubCandidate{err: expected}
	selector, err := NewCandidateSelector(func() Kind { return KindOfficial }, official, nil)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	_, err = selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: "tool", ID: "x"})
	if !errors.Is(err, expected) {
		t.Fatalf("LatestCandidate() error = %v, want %v", err, expected)
	}
}

func TestNewCandidateSelectorValidation(t *testing.T) {
	if _, err := NewCandidateSelector(nil, &stubCandidate{}, nil); err == nil {
		t.Fatal("NewCandidateSelector(nil current) error = nil, want error")
	}
	if _, err := NewCandidateSelector(func() Kind { return KindOfficial }, nil, nil); err == nil {
		t.Fatal("NewCandidateSelector(nil official) error = nil, want error")
	}
}

func TestStateView(t *testing.T) {
	state, err := NewState(KindOfficial, false, nil)
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	if state.View().Kind != KindOfficial {
		t.Fatalf("View() = %+v, want kind %q", state.View(), KindOfficial)
	}
	if got := fmt.Sprintf("%s", state.Current()); got != "official" {
		t.Fatalf("String() = %q", got)
	}
}
