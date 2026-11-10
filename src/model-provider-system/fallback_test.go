package modelprovider

import (
	"context"
	"testing"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func groupCoordinate() types.ModelCoordinate {
	return types.ModelCoordinate{Kind: types.ModelCoordinateKindGroup, GroupID: "group", ModelID: "exposed"}
}

func groupRequest() types.ModelRequest {
	return types.ModelRequest{
		Coordinate: groupCoordinate(),
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
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("hello")}

	response, err := system.Complete(context.Background(), groupRequest())
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content != "hello" {
		t.Fatalf("content = %q, want hello", response.Content)
	}
	if len(network.calls) != 2 {
		t.Fatalf("上游调用次数 = %d, want 2", len(network.calls))
	}
	if hostOfURL(network.calls[0].URL) != hosts[0] || hostOfURL(network.calls[1].URL) != hosts[1] {
		t.Fatalf("调用顺序 = %s → %s", network.calls[0].URL, network.calls[1].URL)
	}
}

// TestCompleteFallbackStopsOnRequestError 钉住不该换的情况：
// 请求本身有问题（400）时换谁都会一样错，直接如实失败，不去烧后面的成员。
func TestCompleteFallbackStopsOnRequestError(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIFailureResponse(400)}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("hello")}

	_, err := system.Complete(context.Background(), groupRequest())
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
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIFailureResponse(502)}

	_, err := system.Complete(context.Background(), groupRequest())
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
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIContentResponse("first")}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("backup")}

	for round := 0; round < 3; round++ {
		response, err := system.Complete(context.Background(), groupRequest())
		if err != nil {
			t.Fatalf("第 %d 次 Complete() error = %v", round+1, err)
		}
		if response.Content != "first" {
			t.Fatalf("第 %d 次 content = %q, want first", round+1, response.Content)
		}
	}
	for index, call := range network.calls {
		if hostOfURL(call.URL) != hosts[0] {
			t.Fatalf("第 %d 次打到了 %s, want %s", index+1, call.URL, hosts[0])
		}
	}
}

// TestCompleteStreamFallbackSwitchesBeforeFirstEvent 钉住流式可换的边界：
// 还没向调用方吐出任何内容时，换人不会造成重放，可以顺位兜底。
func TestCompleteStreamFallbackSwitchesBeforeFirstEvent(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{err: apperrors.Wrap("network-request-system", "network.connection_failed", "dial failed", nil)}
	network.byHost[hosts[1]] = fakeNetworkReply{
		response:     types.HTTPResponse{StatusCode: 200},
		streamChunks: []string{streamContentChunk("hi"), "data: [DONE]\n\n"},
	}

	var contents []string
	response, err := system.CompleteStream(context.Background(), groupRequest(), func(event types.ModelStreamEvent) error {
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
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{
		response:     types.HTTPResponse{StatusCode: 200},
		streamChunks: []string{streamContentChunk("half")},
		streamErr:    errStreamBroken,
	}
	network.byHost[hosts[1]] = fakeNetworkReply{
		response:     types.HTTPResponse{StatusCode: 200},
		streamChunks: []string{streamContentChunk("hi"), "data: [DONE]\n\n"},
	}

	var contents []string
	_, err := system.CompleteStream(context.Background(), groupRequest(), func(event types.ModelStreamEvent) error {
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
