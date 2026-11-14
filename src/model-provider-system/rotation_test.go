package modelprovider

import (
	"context"
	"strconv"
	"testing"

	"eucli-box/pkg/types"
)

// groupRotationFixture 造一个按顺序轮询的模型组，并返回按成员顺序排列的供应商 ID。
func groupRotationFixture(t *testing.T, memberCount int) (System, *fakeProviderStorage, []string) {
	t.Helper()
	storage := newFakeProviderStorage()
	members := make([]types.ModelGroupMember, 0, memberCount)
	providerIDs := make([]string, 0, memberCount)
	for index := 0; index < memberCount; index++ {
		providerID := "provider-" + strconv.Itoa(index)
		provider := testOpenAIProvider()
		provider.ID = providerID
		provider.Name = providerID
		provider.RegisteredModels = []types.ProviderRegisteredModel{{
			ID:            "exposed",
			SourceModelID: "source-" + strconv.Itoa(index),
		}}
		storage.providers[providerID] = provider
		members = append(members, types.ModelGroupMember{ProviderID: providerID, ModelID: "exposed", Weight: 1})
		providerIDs = append(providerIDs, providerID)
	}
	storage.modelGroups = []types.ModelGroup{{
		ID:   "group",
		Name: "Group",
		Models: []types.ModelGroupModel{{
			ID:       "exposed",
			Strategy: types.RotationStrategySequential,
			Members:  members,
		}},
	}}
	return newTestProviderSystem(t, &fakeNetwork{}, storage), storage, providerIDs
}

func resolveGroupProviderID(t *testing.T, system System) string {
	t.Helper()
	provider, _, err := system.ResolveModel(context.Background(), types.ModelCoordinate{Kind: types.ModelCoordinateKindGroup, GroupID: "group", ModelID: "exposed"})
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	return provider.ID
}

// TestResolveModelGroupSequentialRotationCyclesMembers 钉住顺序轮询：
// 每次解析前进一个成员，走完一圈回到第一个。
func TestResolveModelGroupSequentialRotationCyclesMembers(t *testing.T) {
	system, _, providerIDs := groupRotationFixture(t, 3)
	want := []string{providerIDs[0], providerIDs[1], providerIDs[2], providerIDs[0]}
	for round, expected := range want {
		if got := resolveGroupProviderID(t, system); got != expected {
			t.Fatalf("第 %d 次解析 = %s, want %s", round+1, got, expected)
		}
	}
}

// TestValidateCoordinateDoesNotAdvanceGroupRotation 钉住校验无副作用：
// 配置校验不得吃掉轮询顺序，首次真正的解析仍从第一个成员开始。
func TestValidateCoordinateDoesNotAdvanceGroupRotation(t *testing.T) {
	system, _, providerIDs := groupRotationFixture(t, 3)
	coordinate := types.ModelCoordinate{Kind: types.ModelCoordinateKindGroup, GroupID: "group", ModelID: "exposed"}
	for attempt := 0; attempt < 3; attempt++ {
		if err := system.ValidateCoordinate(context.Background(), coordinate); err != nil {
			t.Fatalf("ValidateCoordinate() error = %v", err)
		}
	}
	if got := resolveGroupProviderID(t, system); got != providerIDs[0] {
		t.Fatalf("校验后首次解析 = %s, want %s", got, providerIDs[0])
	}
}

// TestValidateCoordinateRejectsUnavailableGroupMember 钉住组校验覆盖到成员：
// 任一成员模型不可用即拒绝，不静默放过。
func TestValidateCoordinateRejectsUnavailableGroupMember(t *testing.T) {
	system, storage, providerIDs := groupRotationFixture(t, 2)
	broken := storage.providers[providerIDs[1]]
	broken.RegisteredModels = nil
	storage.providers[providerIDs[1]] = broken

	err := system.ValidateCoordinate(context.Background(), types.ModelCoordinate{Kind: types.ModelCoordinateKindGroup, GroupID: "group", ModelID: "exposed"})
	if err == nil {
		t.Fatalf("ValidateCoordinate() error = nil, want member unavailable")
	}
	assertAppErrorCode(t, err, "provider.invalid_request")
}

// TestCompleteRecordsResolvedRequestModelForModelGroup 钉住：坐标指向模型组时，
// 响应记录的是实际发出去的那一家供应商与模型，而不是组本身。
func TestCompleteRecordsResolvedRequestModelForModelGroup(t *testing.T) {
	storage := newFakeProviderStorage()
	provider := testOpenAIProvider()
	provider.ID = "provider-0"
	provider.Name = "主供应商"
	provider.RegisteredModels = []types.ProviderRegisteredModel{{ID: "exposed", SourceModelID: "gpt-4.1"}}
	storage.providers["provider-0"] = provider
	storage.modelGroups = []types.ModelGroup{{
		ID:   "group",
		Name: "Group",
		Models: []types.ModelGroupModel{{
			ID:       "exposed",
			Strategy: types.RotationStrategySequential,
			Members:  []types.ModelGroupMember{{ProviderID: "provider-0", ModelID: "exposed", Weight: 1}},
		}},
	}}
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-group","choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)}}
	system := newTestProviderSystem(t, network, storage)

	response, err := system.Complete(context.Background(), types.ModelRequest{
		Coordinate: types.ModelCoordinate{Kind: types.ModelCoordinateKindGroup, GroupID: "group", ModelID: "exposed"},
		Messages:   []types.PromptMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.RequestModel == nil {
		t.Fatalf("request model = nil")
	}
	if response.RequestModel.ProviderID != "provider-0" || response.RequestModel.ProviderName != "主供应商" || response.RequestModel.ModelID == "" {
		t.Fatalf("request model = %#v", response.RequestModel)
	}
}
