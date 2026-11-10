package installsource

import (
	"context"
	"encoding/json"
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

// shelfConfig 构造只含本机来源的配置，测试里避免重复书写提供方式。
func shelfConfig(source string, shelves ...Shelf) Config {
	sources := make([]Source, 0, len(shelves))
	for _, shelf := range shelves {
		sources = append(sources, Source{Name: shelf.Name, Kind: SourceKindShelf, Path: shelf.Path})
	}
	return Config{Source: source, Sources: sources}
}

func TestConfigReadsLegacyShelvesAsLocalSources(t *testing.T) {
	var config Config
	payload := []byte(`{"source":"甲","shelves":[{"name":"甲","path":"D:/a"},{"name":"乙","path":"D:/b"}]}`)
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if config.Source != "甲" || len(config.Sources) != 2 {
		t.Fatalf("config = %#v", config)
	}
	if config.Sources[1].Kind != SourceKindShelf || config.Sources[1].Path != "D:/b" || config.Sources[1].IndexURL != "" {
		t.Fatalf("legacy source = %#v", config.Sources[1])
	}
}

func TestConfigReadsCurrentSourcesAndRejectsMixedShapes(t *testing.T) {
	var config Config
	payload := []byte(`{"source":"official","sources":[{"name":"甲","kind":"remote","indexUrl":"https://store.example.com/catalog/index.json"}]}`)
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(config.Sources) != 1 || config.Sources[0].Kind != SourceKindRemote {
		t.Fatalf("config = %#v", config)
	}
	mixed := []byte(`{"source":"official","sources":[{"name":"乙","kind":"shelf","path":"D:/b"}],"shelves":[{"name":"甲","path":"D:/a"}]}`)
	if err := json.Unmarshal(mixed, &config); err == nil {
		t.Fatal("mixed shapes error = nil")
	}
}

func TestNormalizeConfigRejectsInvalidShapes(t *testing.T) {
	cases := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{name: "缺少来源", config: Config{}, wantErr: "缺少来源选择"},
		{name: "空名字", config: shelfConfig(OfficialSource, Shelf{Name: "  ", Path: "D:/shelf"}), wantErr: "名字不能为空"},
		{name: "保留字", config: shelfConfig(OfficialSource, Shelf{Name: "Official", Path: "D:/shelf"}), wantErr: "保留字"},
		{name: "重名", config: shelfConfig(OfficialSource, Shelf{Name: "A", Path: "D:/a"}, Shelf{Name: "A", Path: "D:/b"}), wantErr: "重复"},
		{name: "空路径", config: shelfConfig(OfficialSource, Shelf{Name: "A", Path: " "}), wantErr: "缺少路径"},
		{name: "选择未注册来源", config: shelfConfig("B", Shelf{Name: "A", Path: "D:/a"}), wantErr: "不是已注册来源"},
		{name: "本机来源填网址", config: Config{Source: OfficialSource, Sources: []Source{{Name: "A", Kind: SourceKindShelf, Path: "D:/a", IndexURL: "https://a.example.com"}}}, wantErr: "不能填写网络地址"},
		{name: "网络来源填路径", config: Config{Source: OfficialSource, Sources: []Source{{Name: "A", Kind: SourceKindRemote, Path: "D:/a", IndexURL: "https://a.example.com/index.json"}}}, wantErr: "不能填写本机路径"},
		{name: "网络来源明文", config: Config{Source: OfficialSource, Sources: []Source{{Name: "A", Kind: SourceKindRemote, IndexURL: "http://a.example.com/index.json"}}}, wantErr: "HTTPS"},
		{name: "网络来源带账号", config: Config{Source: OfficialSource, Sources: []Source{{Name: "A", Kind: SourceKindRemote, IndexURL: "https://user@a.example.com/index.json"}}}, wantErr: "账号"},
		{name: "网络来源带查询", config: Config{Source: OfficialSource, Sources: []Source{{Name: "A", Kind: SourceKindRemote, IndexURL: "https://a.example.com/index.json?x=1"}}}, wantErr: "查询参数"},
		{name: "未知提供方式", config: Config{Source: OfficialSource, Sources: []Source{{Name: "A", Kind: "cd", Path: "D:/a"}}}, wantErr: "提供方式"},
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
		Source: "  甲  ",
		Sources: []Source{
			{Name: " 甲 ", Kind: SourceKindShelf, Path: " D:/a "},
			{Name: "乙", Kind: SourceKindRemote, IndexURL: " https://b.example.com/index.json "},
		},
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	if normalized.Source != "甲" || normalized.Sources[0].Name != "甲" || normalized.Sources[0].Path != "D:/a" {
		t.Fatalf("normalized = %#v", normalized)
	}
	if normalized.Sources[1].Name != "乙" || normalized.Sources[1].IndexURL != "https://b.example.com/index.json" {
		t.Fatalf("order = %#v", normalized.Sources)
	}
}

func TestNewStateInvalidInitialEntersProblemMode(t *testing.T) {
	state := NewState(Config{Source: "missing"}, nil)
	if state.Problem() == "" {
		t.Fatal("Problem() = empty, want invalid config problem")
	}
	if state.CurrentSource() != "" || len(state.Sources()) != 0 || len(state.Shelves()) != 0 {
		t.Fatalf("unavailable state = %q/%#v", state.CurrentSource(), state.Sources())
	}
	if _, ok := state.Source("any"); ok {
		t.Fatal("Source() = ok in problem mode")
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
		t.Fatal("SetSource(source) in problem mode error = nil")
	}
	next, err := broken.SetSource(context.Background(), OfficialSource)
	if err != nil {
		t.Fatalf("SetSource(official) error = %v", err)
	}
	if next != OfficialSource || broken.Problem() != "" {
		t.Fatalf("repaired source = %q problem = %q", next, broken.Problem())
	}
	if store.saveCall != 1 || store.saved.Source != OfficialSource || len(store.saved.Sources) != 0 {
		t.Fatalf("saved = %#v calls = %d", store.saved, store.saveCall)
	}
}

func TestAddSourceRepairsUnavailableConfig(t *testing.T) {
	state := NewStateUnavailable(errors.New("配置损坏"), nil)
	sources, err := state.AddSource(context.Background(), Source{Name: "甲", Kind: SourceKindShelf, Path: "D:/a"})
	if err != nil {
		t.Fatalf("AddSource() error = %v", err)
	}
	if len(sources) != 1 || sources[0].Name != "甲" || state.Problem() != "" || state.CurrentSource() != OfficialSource {
		t.Fatalf("sources = %#v source = %q problem = %q", sources, state.CurrentSource(), state.Problem())
	}
}

func TestUpdateAndRemoveRejectUnavailableConfig(t *testing.T) {
	newName := "乙"
	state := NewStateUnavailable(errors.New("配置损坏"), nil)
	if _, err := state.UpdateSource(context.Background(), "甲", SourcePatch{NewName: &newName}); err == nil || !strings.Contains(err.Error(), "配置不可用") {
		t.Fatalf("UpdateSource() error = %v", err)
	}
	if _, err := state.RemoveSource(context.Background(), "甲"); err == nil || !strings.Contains(err.Error(), "配置不可用") {
		t.Fatalf("RemoveSource() error = %v", err)
	}
}

func TestSetSourcePersistsAndUpdatesMemory(t *testing.T) {
	store := &fakeStore{}
	state := NewState(shelfConfig(OfficialSource, Shelf{Name: "A", Path: "D:/a"}), store)
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

func TestSetSourceRejectsUnknownAndReservedVariant(t *testing.T) {
	state := NewState(shelfConfig(OfficialSource, Shelf{Name: "A", Path: "D:/a"}), nil)
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
	state := NewState(shelfConfig(OfficialSource, Shelf{Name: "A", Path: "D:/a"}), store)
	if _, err := state.SetSource(context.Background(), "A"); err == nil {
		t.Fatal("SetSource() error = nil, want persist failure")
	}
	if state.CurrentSource() != OfficialSource {
		t.Fatalf("current = %q, want official", state.CurrentSource())
	}
}

func TestAddSourceKeepsOrderAndRejectsInvalid(t *testing.T) {
	store := &fakeStore{}
	state := NewState(DefaultConfig(), store)
	if _, err := state.AddSource(context.Background(), Source{Name: "甲", Kind: SourceKindShelf, Path: "D:/a"}); err != nil {
		t.Fatalf("AddSource(甲) error = %v", err)
	}
	if _, err := state.AddSource(context.Background(), Source{Name: "乙", Kind: SourceKindRemote, IndexURL: "https://b.example.com/index.json"}); err != nil {
		t.Fatalf("AddSource(乙) error = %v", err)
	}
	sources := state.Sources()
	if len(sources) != 2 || sources[0].Name != "甲" || sources[1].Name != "乙" {
		t.Fatalf("sources = %#v", sources)
	}
	if store.saved.Sources[1].IndexURL != "https://b.example.com/index.json" {
		t.Fatalf("saved = %#v", store.saved)
	}
	if shelves := state.Shelves(); len(shelves) != 1 || shelves[0].Name != "甲" {
		t.Fatalf("shelves = %#v", shelves)
	}
	if _, err := state.AddSource(context.Background(), Source{Name: "甲", Kind: SourceKindShelf, Path: "D:/c"}); err == nil {
		t.Fatal("AddSource(duplicate) error = nil")
	}
	if _, err := state.AddSource(context.Background(), Source{Name: "official", Kind: SourceKindShelf, Path: "D:/d"}); err == nil {
		t.Fatal("AddSource(reserved) error = nil")
	}
	if _, err := state.AddSource(context.Background(), Source{Name: "丙", Kind: SourceKindShelf, Path: " "}); err == nil {
		t.Fatal("AddSource(empty path) error = nil")
	}
}

func TestUpdateSourceRenameFollowsSelection(t *testing.T) {
	store := &fakeStore{}
	state := NewState(shelfConfig("甲", Shelf{Name: "甲", Path: "D:/a"}, Shelf{Name: "乙", Path: "D:/b"}), store)
	newName := "甲改"
	sources, err := state.UpdateSource(context.Background(), "甲", SourcePatch{NewName: &newName})
	if err != nil {
		t.Fatalf("UpdateSource() error = %v", err)
	}
	if sources[0].Name != "甲改" || state.CurrentSource() != "甲改" {
		t.Fatalf("sources = %#v, source = %q", sources, state.CurrentSource())
	}
	if _, err := state.UpdateSource(context.Background(), "乙", SourcePatch{}); err == nil {
		t.Fatal("UpdateSource(no change) error = nil")
	}
	duplicate := "乙"
	if _, err := state.UpdateSource(context.Background(), "甲改", SourcePatch{NewName: &duplicate}); err == nil {
		t.Fatal("UpdateSource(duplicate) error = nil")
	}
	if _, err := state.UpdateSource(context.Background(), "失踪", SourcePatch{NewName: &newName}); err == nil {
		t.Fatal("UpdateSource(unknown) error = nil")
	}
}

func TestUpdateSourceKindSwitchesLocationField(t *testing.T) {
	state := NewState(shelfConfig("甲", Shelf{Name: "甲", Path: "D:/a"}), nil)
	kind := SourceKindRemote
	indexURL := "https://store.example.com/catalog/index.json"
	sources, err := state.UpdateSource(context.Background(), "甲", SourcePatch{NewKind: &kind, NewIndexURL: &indexURL})
	if err != nil {
		t.Fatalf("UpdateSource(remote) error = %v", err)
	}
	if sources[0].Kind != SourceKindRemote || sources[0].Path != "" || sources[0].IndexURL != indexURL {
		t.Fatalf("source = %#v", sources[0])
	}
	shelfKind := SourceKindShelf
	path := "D:/moved"
	sources, err = state.UpdateSource(context.Background(), "甲", SourcePatch{NewKind: &shelfKind, NewPath: &path})
	if err != nil {
		t.Fatalf("UpdateSource(shelf) error = %v", err)
	}
	if sources[0].Kind != SourceKindShelf || sources[0].IndexURL != "" || sources[0].Path != "D:/moved" {
		t.Fatalf("source = %#v", sources[0])
	}
}

func TestUpdateShelfRejectsRemoteSource(t *testing.T) {
	state := NewState(Config{Source: OfficialSource, Sources: []Source{{Name: "远程", Kind: SourceKindRemote, IndexURL: "https://a.example.com/index.json"}}}, nil)
	path := "D:/x"
	if _, err := state.UpdateShelf(context.Background(), "远程", nil, &path); err == nil {
		t.Fatal("UpdateShelf(remote) error = nil")
	}
	if _, err := state.RemoveShelf(context.Background(), "远程"); err == nil {
		t.Fatal("RemoveShelf(remote) error = nil")
	}
}

func TestRemoveSourceSelectedFallsBackToOfficial(t *testing.T) {
	state := NewState(shelfConfig("甲", Shelf{Name: "甲", Path: "D:/a"}, Shelf{Name: "乙", Path: "D:/b"}), nil)
	sources, err := state.RemoveSource(context.Background(), "甲")
	if err != nil {
		t.Fatalf("RemoveSource() error = %v", err)
	}
	if len(sources) != 1 || sources[0].Name != "乙" {
		t.Fatalf("sources = %#v", sources)
	}
	if state.CurrentSource() != OfficialSource {
		t.Fatalf("source = %q, want official", state.CurrentSource())
	}
	if _, err := state.RemoveSource(context.Background(), "失踪"); err == nil {
		t.Fatal("RemoveSource(unknown) error = nil")
	}
}

func TestRemoveSourceKeepsUnrelatedSelection(t *testing.T) {
	state := NewState(shelfConfig("甲", Shelf{Name: "甲", Path: "D:/a"}, Shelf{Name: "乙", Path: "D:/b"}), nil)
	if _, err := state.RemoveSource(context.Background(), "乙"); err != nil {
		t.Fatalf("RemoveSource() error = %v", err)
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
	selector, err := NewCandidateSelector(state, official, nil, types.ReleaseArtifactKindTool)
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
	state := NewState(shelfConfig("甲", Shelf{Name: "甲", Path: filepath.Join(t.TempDir(), "missing")}), nil)
	selector, err := NewCandidateSelector(state, official, nil, types.ReleaseArtifactKindTool)
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

func TestCandidateSelectorRemoteRequiresNetwork(t *testing.T) {
	official := &stubCandidate{}
	state := NewState(Config{Source: "远程", Sources: []Source{{Name: "远程", Kind: SourceKindRemote, IndexURL: "https://a.example.com/index.json"}}}, nil)
	selector, err := NewCandidateSelector(state, official, nil, types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("NewCandidateSelector() error = %v", err)
	}
	_, err = selector.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: "tool", ID: "shell_command"})
	if err == nil || !strings.Contains(err.Error(), "需要网络请求能力") {
		t.Fatalf("LatestCandidate() error = %v, want network requirement", err)
	}
	if official.called != 0 {
		t.Fatalf("official called = %d, want 0 (no fallback)", official.called)
	}
}

func TestCandidateSelectorPropagatesOfficialError(t *testing.T) {
	expected := errors.New("network down")
	official := &stubCandidate{err: expected}
	state := NewState(DefaultConfig(), nil)
	selector, err := NewCandidateSelector(state, official, nil, types.ReleaseArtifactKindTool)
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
	selector, err := NewCandidateSelector(state, official, nil, types.ReleaseArtifactKindTool)
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
	if _, err := NewCandidateSelector(nil, &stubCandidate{}, nil, types.ReleaseArtifactKindTool); err == nil {
		t.Fatal("NewCandidateSelector(nil state) error = nil")
	}
	state := NewState(DefaultConfig(), nil)
	if _, err := NewCandidateSelector(state, nil, nil, types.ReleaseArtifactKindTool); err == nil {
		t.Fatal("NewCandidateSelector(nil official) error = nil")
	}
	if _, err := NewCandidateSelector(state, &stubCandidate{}, nil, "盒"); err == nil {
		t.Fatal("NewCandidateSelector(unknown kind) error = nil")
	}
}

func TestCandidateSelectorRejectsForeignKind(t *testing.T) {
	official := &stubCandidate{}
	state := NewState(DefaultConfig(), nil)
	selector, err := NewCandidateSelector(state, official, nil, types.ReleaseArtifactKindTool)
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
