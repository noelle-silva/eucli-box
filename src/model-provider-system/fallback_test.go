package modelprovider

import (
	"context"
	"errors"
	"strconv"
	"testing"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

// fallbackGroupFixture 造一个两成员的 fallback 模型组，成员分别落在两个主机上，
// 便于按主机名安排各自成败。成员顺序即兜底顺序。
func fallbackGroupFixture(t *testing.T) (System, *fakeNetwork) {
	t.Helper()
	storage := newFakeProviderStorage()
	hosts := []string{"a.example.test", "b.example.test"}
	members := make([]types.ModelGroupMember, 0, len(hosts))
	for index, host := range hosts {
		providerID := "provider-" + strconv.Itoa(index)
		provider := testOpenAIProvider()
		provider.ID = providerID
		provider.Name = providerID
		provider.BaseURL = "https://" + host + "/v1"
		storage.providers[providerID] = provider
		members = append(members, types.ModelGroupMember{ProviderID: providerID, ModelID: "gpt-4.1", Weight: 1})
	}
	storage.modelGroups = []types.ModelGroup{{
		ID:   "group",
		Name: "Group",
		Models: []types.ModelGroupModel{{
			ID:       "exposed",
			Name:     "Exposed",
			Strategy: types.RotationStrategyFallback,
			Members:  members,
		}},
	}}
	network := &fakeNetwork{byHost: map[string]fakeNetworkReply{}}
	return newTestProviderSystem(t, network, storage), network
}

func fallbackCoordinate() types.ModelCoordinate {
	return types.ModelCoordinate{Kind: types.ModelCoordinateKindGroup, GroupID: "group", ModelID: "exposed"}
}

func fallbackRequest() types.ModelRequest {
	return types.ModelRequest{
		Coordinate: fallbackCoordinate(),
		Messages:   []types.PromptMessage{{Role: "user", Content: "hi"}},
	}
}

func openAIContentResponse(content string) types.HTTPResponse {
	return types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"` + content + `"}}]}`)}
}

func openAIFailureResponse(status int) types.HTTPResponse {
	return types.HTTPResponse{StatusCode: status, Body: []byte(`{"error":{"message":"boom","type":"server_error"}}`)}
}

func streamContentChunk(content string) string {
	return `data: {"choices":[{"delta":{"content":"` + content + `"}}]}` + "\n\n"
}

// TestCompleteFallbackSwitchesToNextMemberOnServerError 钉住兜底主线：
// 首选成员上游 5xx 时自动顺位到下一个成员，调用方拿到的是成功的那个结果。
func TestCompleteFallbackSwitchesToNextMemberOnServerError(t *testing.T) {
	system, network := fallbackGroupFixture(t)
	network.byHost["a.example.test"] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost["b.example.test"] = fakeNetworkReply{response: openAIContentResponse("hello")}

	response, err := system.Complete(context.Background(), fallbackRequest())
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content != "hello" {
		t.Fatalf("content = %q, want hello", response.Content)
	}
	if len(network.calls) != 2 {
		t.Fatalf("上游调用次数 = %d, want 2", len(network.calls))
	}
	if hostOfURL(network.calls[0].URL) != "a.example.test" || hostOfURL(network.calls[1].URL) != "b.example.test" {
		t.Fatalf("调用顺序 = %s → %s", network.calls[0].URL, network.calls[1].URL)
	}
}

// TestCompleteFallbackStopsOnRequestError 钉住不该换的情况：
// 请求本身有问题（400）时换谁都会一样错，直接如实失败，不去烧后面的成员。
func TestCompleteFallbackStopsOnRequestError(t *testing.T) {
	system, network := fallbackGroupFixture(t)
	network.byHost["a.example.test"] = fakeNetworkReply{response: openAIFailureResponse(400)}
	network.byHost["b.example.test"] = fakeNetworkReply{response: openAIContentResponse("hello")}

	_, err := system.Complete(context.Background(), fallbackRequest())
	if err == nil {
		t.Fatalf("Complete() error = nil, want upstream failure")
	}
	if len(network.calls) != 1 {
		t.Fatalf("上游调用次数 = %d, want 1（请求错误不换人）", len(network.calls))
	}
	payload := apperrors.BuildErrorPayload(err)
	if payload == nil || payload.Code != apperrors.CodeProviderServiceFailed {
		t.Fatalf("错误码 = %v, want %s", payload, apperrors.CodeProviderServiceFailed)
	}
}

// TestCompleteFallbackAggregatesWhenAllMembersFail 钉住链尾行为：
// 全部成员都失败时报"链已试完"，并把每个成员的失败留成并列原因。
func TestCompleteFallbackAggregatesWhenAllMembersFail(t *testing.T) {
	system, network := fallbackGroupFixture(t)
	network.byHost["a.example.test"] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost["b.example.test"] = fakeNetworkReply{response: openAIFailureResponse(502)}

	_, err := system.Complete(context.Background(), fallbackRequest())
	if err == nil {
		t.Fatalf("Complete() error = nil, want exhausted failure")
	}
	if len(network.calls) != 2 {
		t.Fatalf("上游调用次数 = %d, want 2", len(network.calls))
	}
	payload := apperrors.BuildErrorPayload(err)
	if payload == nil || payload.Code != apperrors.CodeProviderModelGroupExhausted {
		t.Fatalf("错误码 = %v, want %s", payload, apperrors.CodeProviderModelGroupExhausted)
	}
	if len(payload.Causes) != 2 {
		t.Fatalf("并列原因 = %d, want 2", len(payload.Causes))
	}
}

// TestCompleteFallbackAlwaysStartsFromFirstMember 钉住 fallback 与轮询的区别：
// 每次都从首选成员开始，上一个请求用了谁不影响这次。
func TestCompleteFallbackAlwaysStartsFromFirstMember(t *testing.T) {
	system, network := fallbackGroupFixture(t)
	network.byHost["a.example.test"] = fakeNetworkReply{response: openAIContentResponse("first")}
	network.byHost["b.example.test"] = fakeNetworkReply{response: openAIContentResponse("backup")}

	for round := 0; round < 3; round++ {
		response, err := system.Complete(context.Background(), fallbackRequest())
		if err != nil {
			t.Fatalf("第 %d 次 Complete() error = %v", round+1, err)
		}
		if response.Content != "first" {
			t.Fatalf("第 %d 次 content = %q, want first", round+1, response.Content)
		}
	}
	for index, call := range network.calls {
		if hostOfURL(call.URL) != "a.example.test" {
			t.Fatalf("第 %d 次打到了 %s, want a.example.test", index+1, call.URL)
		}
	}
}

// TestCompleteStreamFallbackSwitchesBeforeFirstEvent 钉住流式可换的边界：
// 还没向调用方吐出任何内容时，换人不会造成重放，可以顺位兜底。
func TestCompleteStreamFallbackSwitchesBeforeFirstEvent(t *testing.T) {
	system, network := fallbackGroupFixture(t)
	network.byHost["a.example.test"] = fakeNetworkReply{err: apperrors.Wrap("network-request-system", "network.connection_failed", "dial failed", nil)}
	network.byHost["b.example.test"] = fakeNetworkReply{
		response:     types.HTTPResponse{StatusCode: 200},
		streamChunks: []string{streamContentChunk("hi"), "data: [DONE]\n\n"},
	}

	var contents []string
	response, err := system.CompleteStream(context.Background(), fallbackRequest(), func(event types.ModelStreamEvent) error {
		contents = append(contents, event.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteStream() error = %v", err)
	}
	if response.Content != "hi" || len(contents) != 1 {
		t.Fatalf("content = %q, 事件数 = %d", response.Content, len(contents))
	}
	if len(network.calls) != 2 {
		t.Fatalf("上游调用次数 = %d, want 2", len(network.calls))
	}
}

// TestCompleteStreamFallbackStopsAfterContentDispatched 钉住流式不可换的边界：
// 内容已经交给调用方后再断线，换人只会变成重放，必须如实报错。
func TestCompleteStreamFallbackStopsAfterContentDispatched(t *testing.T) {
	system, network := fallbackGroupFixture(t)
	network.byHost["a.example.test"] = fakeNetworkReply{
		response:     types.HTTPResponse{StatusCode: 200},
		streamChunks: []string{streamContentChunk("half")},
		streamErr:    errors.New("connection reset by peer"),
	}
	network.byHost["b.example.test"] = fakeNetworkReply{
		response:     types.HTTPResponse{StatusCode: 200},
		streamChunks: []string{streamContentChunk("hi"), "data: [DONE]\n\n"},
	}

	var contents []string
	_, err := system.CompleteStream(context.Background(), fallbackRequest(), func(event types.ModelStreamEvent) error {
		contents = append(contents, event.Content)
		return nil
	})
	if err == nil {
		t.Fatalf("CompleteStream() error = nil, want stream failure")
	}
	if len(network.calls) != 1 {
		t.Fatalf("上游调用次数 = %d, want 1（已吐内容不换人）", len(network.calls))
	}
	if len(contents) != 1 || contents[0] != "half" {
		t.Fatalf("已分发内容 = %#v, want [\"half\"]", contents)
	}
}
