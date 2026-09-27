package installsource

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type fakeStore struct {
	config   Config
	saved    Config
	loadErr  error
	saveErr  error
	saveCall int
}

func (s *fakeStore) LoadInstallSource(context.Context) (Config, error) {
	if s.loadErr != nil {
		return Config{}, s.loadErr
	}
	return s.config.Clone(), nil
}

func (s *fakeStore) SaveInstallSource(_ context.Context, config Config) error {
	s.saveCall++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = config.Clone()
	return nil
}

func TestNormalizeConfigRejectsInvalidShapes(t *testing.T) {
	cases := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{name: "缺少来源", config: Config{}, wantErr: "缺少来源选择"},
		{name: "空货架名", config: Config{Source: OfficialSource, Shelves: []Shelf{{Name: "  ", Path: "D:/shelf"}}}, wantErr: "名字不能为空"},
		{name: "保留字", config: Config{Source: OfficialSource, Shelves: []Shelf{{Name: "Official", Path: "D:/shelf"}}}, wantErr: "保留字"},
		{name: "重名", config: Config{Source: OfficialSource, Shelves: []Shelf{{Name: "A", Path: "D:/a"}, {Name: "A", Path: "D:/b"}}}, wantErr: "重复"},
		{name: "空路径", config: Config{Source: OfficialSource, Shelves: []Shelf{{Name: "A", Path: " "}}}, wantErr: "缺少路径"},
		{name: "选择未知货架", config: Config{Source: "B", Shelves: []Shelf{{Name: "A", Path: "D:/a"}}}, wantErr: "不是已注册货架"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := normalizeConfig(item.config); err == nil || !strings.Contains(err.Error(), item.wantErr) {
				t.Fatalf("normalizeConfig() error = %v, want contains %q", err, item.wantErr)
			}
		})
	}
}

func TestNormalizeConfigTrimsAndPreservesOrder(t *testing.T) {
	normalized, err := normalizeConfig(Config{
		Source:  "  货架甲  ",
		Shelves: []Shelf{{Name: " 货架甲 ", Path: " D:/shelf-a "}, {Name: "货架乙", Path: "D:/shelf-b"}},
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	if normalized.Source != "货架甲" || normalized.Shelves[0].Name != "货架甲" || normalized.Shelves[0].Path != "D:/shelf-a" {
		t.Fatalf("normalized = %#v", normalized)
	}
	if normalized.Shelves[1].Name != "货架乙" {
		t.Fatalf("order = %#v", normalized.Shelves)
	}
}

func TestNewStateInvalidInitialEntersProblemMode(t *testing.T) {
	state := NewState(Config{Source: "missing"}, nil)
	if state.Problem() == "" {
		t.Fatal("Problem() = empty, want invalid config problem")
	}
	if state.CurrentSource() != "" || len(state.Shelves()) != 0 {
		t.Fatalf("unavailable state = %q/%#v", state.CurrentSource(), state.Shelves())
	}
	if _, ok := state.Shelf("any"); ok {
		t.Fatal("Shelf() = ok in problem mode")
	}
}

func TestSetSourceRepairsUnavailableConfig(t *testing.T) {
	store := &fakeStore{}
	state := NewState(DefaultConfig(), store)
	if state.Problem() != "" {
		t.Fatalf("Problem() = %q, want empty", state.Problem())
	}
	broken := NewStateUnavailable(errors.New("旧格式无法识别"), store)
	if _, err := broken.SetSource(context.Background(), "旧货架"); err == nil {
		t.Fatal("SetSource(shelf) in problem mode error = nil")
	}
	next, err := broken.SetSource(context.Background(), OfficialSource)
	if err != nil {
		t.Fatalf("SetSource(official) error = %v", err)
	}
	if next != OfficialSource || broken.Problem() != "" {
		t.Fatalf("repaired source = %q problem = %q", next, broken.Problem())
	}
	if store.saveCall != 1 || store.saved.Source != OfficialSource || len(store.saved.Shelves) != 0 {
		t.Fatalf("saved = %#v calls = %d", store.saved, store.saveCall)
	}
}

func TestAddShelfRepairsUnavailableConfig(t *testing.T) {
	state := NewStateUnavailable(errors.New("配置损坏"), nil)
	shelves, err := state.AddShelf(context.Background(), "甲", "D:/a")
	if err != nil {
		t.Fatalf("AddShelf() error = %v", err)
	}
	if len(shelves) != 1 || shelves[0].Name != "甲" || state.Problem() != "" || state.CurrentSource() != OfficialSource {
		t.Fatalf("shelves = %#v source = %q problem = %q", shelves, state.CurrentSource(), state.Problem())
	}
}

func TestUpdateAndRemoveRejectUnavailableConfig(t *testing.T) {
	newName := "乙"
	state := NewStateUnavailable(errors.New("配置损坏"), nil)
	if _, err := state.UpdateShelf(context.Background(), "甲", &newName, nil); err == nil || !strings.Contains(err.Error(), "配置不可用") {
		t.Fatalf("UpdateShelf() error = %v", err)
	}
	if _, err := state.RemoveShelf(context.Background(), "甲"); err == nil || !strings.Contains(err.Error(), "配置不可用") {
		t.Fatalf("RemoveShelf() error = %v", err)
	}
}

func TestSetSourcePersistsAndUpdatesMemory(t *testing.T) {
	store := &fakeStore{}
	state := NewState(Config{Source: OfficialSource, Shelves: []Shelf{{Name: "A", Path: "D:/a"}}}, store)
	next, err := state.SetSource(context.Background(), "A")
	if err != nil {
		t.Fatalf("SetSource() error = %v", err)
	}
	if next != "A" || state.CurrentSource() != "A" {
		t.Fatalf("SetSource() = %q, current = %q", next, state.CurrentSource())
	}
	if store.saveCall != 1 || store.saved.Source != "A" {
		t.Fatalf("store save = %v/%q, want 1 call of A", store.saveCall, store.saved.Source)
	}
}

func TestSetSourceRejectsUnknownShelfAndReservedVariant(t *testing.T) {
	state := NewState(Config{Source: OfficialSource, Shelves: []Shelf{{Name: "A", Path: "D:/a"}}}, nil)
	if _, err := state.SetSource(context.Background(), "B"); err == nil {
		t.Fatal("SetSource(unknown) error = nil")
	}
	if _, err := state.SetSource(context.Background(), "Official"); err == nil {
		t.Fatal("SetSource(Official) error = nil")
	}
	if state.CurrentSource() != OfficialSource {
		t.Fatalf("current = %q, want official", state.CurrentSource())
	}
}

func TestSetSourcePersistFailureKeepsCurrent(t *testing.T) {
	store := &fakeStore{saveErr: errors.New("disk full")}
	state := NewState(Config{Source: OfficialSource, Shelves: []Shelf{{Name: "A", Path: "D:/a"}}}, store)
	if _, err := state.SetSource(context.Background(), "A"); err == nil {
		t.Fatal("SetSource() error = nil, want persist failure")
	}
	if state.CurrentSource() != OfficialSource {
		t.Fatalf("current = %q, want official", state.CurrentSource())
	}
}

func TestAddShelfKeepsOrderAndRejectsInvalid(t *testing.T) {
	store := &fakeStore{}
	state := NewState(DefaultConfig(), store)
	if _, err := state.AddShelf(context.Background(), "甲", "D:/a"); err != nil {
		t.Fatalf("AddShelf(甲) error = %v", err)
	}
	if _, err := state.AddShelf(context.Background(), "乙", "D:/b"); err != nil {
		t.Fatalf("AddShelf(乙) error = %v", err)
	}
	shelves := state.Shelves()
	if len(shelves) != 2 || shelves[0].Name != "甲" || shelves[1].Name != "乙" {
		t.Fatalf("shelves = %#v", shelves)
	}
	if store.saved.Shelves[1].Path != "D:/b" {
		t.Fatalf("saved = %#v", store.saved)
	}
	if _, err := state.AddShelf(context.Background(), "甲", "D:/c"); err == nil {
		t.Fatal("AddShelf(duplicate) error = nil")
	}
	if _, err := state.AddShelf(context.Background(), "official", "D:/d"); err == nil {
		t.Fatal("AddShelf(reserved) error = nil")
	}
	if _, err := state.AddShelf(context.Background(), "丙", " "); err == nil {
		t.Fatal("AddShelf(empty path) error = nil")
	}
}

func TestUpdateShelfRenameFollowsSelection(t *testing.T) {
	store := &fakeStore{}
	state := NewState(Config{Source: "甲", Shelves: []Shelf{{Name: "甲", Path: "D:/a"}, {Name: "乙", Path: "D:/b"}}}, store)
	newName := "甲改"
	shelves, err := state.UpdateShelf(context.Background(), "甲", &newName, nil)
	if err != nil {
		t.Fatalf("UpdateShelf() error = %v", err)
	}
	if shelves[0].Name != "甲改" || state.CurrentSource() != "甲改" {
		t.Fatalf("shelves = %#v, source = %q", shelves, state.CurrentSource())
	}
	if _, err := state.UpdateShelf(context.Background(), "乙", nil, nil); err == nil {
		t.Fatal("UpdateShelf(no change) error = nil")
	}
	if _, err := state.UpdateShelf(context.Background(), "甲改", &newName, nil); err != nil {
		t.Fatalf("UpdateShelf(same name) error = %v", err)
	}
	duplicate := "乙"
	if _, err := state.UpdateShelf(context.Background(), "甲改", &duplicate, nil); err == nil {
		t.Fatal("UpdateShelf(duplicate) error = nil")
	}
	if _, err := state.UpdateShelf(context.Background(), "失踪", &newName, nil); err == nil {
		t.Fatal("UpdateShelf(unknown) error = nil")
	}
}

func TestUpdateShelfPathKeepsSelection(t *testing.T) {
	state := NewState(Config{Source: "甲", Shelves: []Shelf{{Name: "甲", Path: "D:/a"}}}, nil)
	newPath := "D:/moved"
	shelves, err := state.UpdateShelf(context.Background(), "甲", nil, &newPath)
	if err != nil {
		t.Fatalf("UpdateShelf() error = %v", err)
	}
	if shelves[0].Path != "D:/moved" || state.CurrentSource() != "甲" {
		t.Fatalf("shelves = %#v, source = %q", shelves, state.CurrentSource())
	}
}

func TestRemoveShelfSelectedFallsBackToOfficial(t *testing.T) {
	state := NewState(Config{Source: "甲", Shelves: []Shelf{{Name: "甲", Path: "D:/a"}, {Name: "乙", Path: "D:/b"}}}, nil)
	shelves, err := state.RemoveShelf(context.Background(), "甲")
	if err != nil {
		t.Fatalf("RemoveShelf() error = %v", err)
	}
	if len(shelves) != 1 || shelves[0].Name != "乙" {
		t.Fatalf("shelves = %#v", shelves)
	}
	if state.CurrentSource() != OfficialSource {
		t.Fatalf("source = %q, want official", state.CurrentSource())
	}
	if _, err := state.RemoveShelf(context.Background(), "失踪"); err == nil {
		t.Fatal("RemoveShelf(unknown) error = nil")
	}
}

func TestRemoveShelfKeepsUnrelatedSelection(t *testing.T) {
	state := NewState(Config{Source: "甲", Shelves: []Shelf{{Name: "甲", Path: "D:/a"}, {Name: "乙", Path: "D:/b"}}}, nil)
	if _, err := state.RemoveShelf(context.Background(), "乙"); err != nil {
		t.Fatalf("RemoveShelf() error = %v", err)
	}
	if state.CurrentSource() != "甲" {
		t.Fatalf("source = %q, want 甲", state.CurrentSource())
	}
}

type stubCandidate struct {
	called    int
	candidate *releasecheck.ReleaseCandidate
	err       error
}

func (s *stubCandidate) LatestCandidate(context.Context, types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	return s.candidate, nil
}

func TestCandidateSelectorForwardsOfficial(t *testing.T) {
	official := &stubCandidate{}
	state := NewState(DefaultConfig(), nil)
	selector, err := NewCandidateSelector(state, official, types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	if _, err := selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: "tool", ID: "shell_command"}); err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	if official.called != 1 {
		t.Fatalf("official called = %d, want 1", official.called)
	}
}

func TestCandidateSelectorShelfUnavailableFailsFast(t *testing.T) {
	official := &stubCandidate{}
	state := NewState(Config{Source: "甲", Shelves: []Shelf{{Name: "甲", Path: filepath.Join(t.TempDir(), "missing")}}}, nil)
	selector, err := NewCandidateSelector(state, official, types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	_, err = selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: "tool", ID: "shell_command"})
	if err == nil || !strings.Contains(err.Error(), "货架 \"甲\" 不可用") {
		t.Fatalf("LatestCandidate() error = %v, want shelf unavailable", err)
	}
	if official.called != 0 {
		t.Fatalf("official called = %d, want 0 (no fallback)", official.called)
	}
}

func TestCandidateSelectorPropagatesOfficialError(t *testing.T) {
	expected := errors.New("network down")
	official := &stubCandidate{err: expected}
	state := NewState(DefaultConfig(), nil)
	selector, err := NewCandidateSelector(state, official, types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	_, err = selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: "tool", ID: "x"})
	if !errors.Is(err, expected) {
		t.Fatalf("LatestCandidate() error = %v, want %v", err, expected)
	}
}

func TestCandidateSelectorProblemConfigFailsFast(t *testing.T) {
	official := &stubCandidate{}
	state := NewStateUnavailable(errors.New("旧格式无法识别"), nil)
	selector, err := NewCandidateSelector(state, official, types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	_, err = selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: "tool", ID: "shell_command"})
	if err == nil || !strings.Contains(err.Error(), "安装来源配置不可用") {
		t.Fatalf("LatestCandidate() error = %v, want unavailable config", err)
	}
	if official.called != 0 {
		t.Fatalf("official called = %d, want 0 (no fallback)", official.called)
	}
}

func TestNewCandidateSelectorValidation(t *testing.T) {
	if _, err := NewCandidateSelector(nil, &stubCandidate{}, types.ReleaseArtifactKindTool); err == nil {
		t.Fatal("NewCandidateSelector(nil state) error = nil")
	}
	state := NewState(DefaultConfig(), nil)
	if _, err := NewCandidateSelector(state, nil, types.ReleaseArtifactKindTool); err == nil {
		t.Fatal("NewCandidateSelector(nil official) error = nil")
	}
	if _, err := NewCandidateSelector(state, &stubCandidate{}, "盒"); err == nil {
		t.Fatal("NewCandidateSelector(unknown kind) error = nil")
	}
}

func TestCandidateSelectorRejectsForeignKind(t *testing.T) {
	official := &stubCandidate{}
	state := NewState(DefaultConfig(), nil)
	selector, err := NewCandidateSelector(state, official, types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	_, err = selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"})
	if err == nil || !strings.Contains(err.Error(), "不服务发布物类别") {
		t.Fatalf("LatestCandidate(plugin) error = %v, want foreign kind rejection", err)
	}
	if official.called != 0 {
		t.Fatalf("official called = %d, want 0", official.called)
	}
}
