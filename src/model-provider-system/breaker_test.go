package modelprovider

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// errStreamBroken 模拟流式传输中途断线。
var errStreamBroken = errors.New("connection reset by peer")

// groupFixture 造一个两成员模型组：两个成员分别落在不同主机上，
// 便于按主机安排各自成败，并分辨这一轮究竟打到了谁。
func groupFixture(t *testing.T, strategy types.RotationStrategy) (System, *fakeNetwork, []string) {
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
			Strategy: strategy,
			Members:  members,
		}},
	}}
	network := &fakeNetwork{byHost: map[string]fakeNetworkReply{}}
	return newTestProviderSystem(t, network, storage), network, hosts
}

// concreteSystem 取出内部实现，便于断言熔断状态。
func concreteSystem(t *testing.T, s System) *system {
	t.Helper()
	concrete, ok := s.(*system)
	if !ok {
		t.Fatalf("system = %T, want *system", s)
	}
	return concrete
}

// freezeBreakerClock 把熔断记忆的时钟换成可控时钟，便于验证冷却到期。
func freezeBreakerClock(s System, cooldown time.Duration) *time.Time {
	clock := time.Now()
	breaker := newMemberBreaker(cooldown)
	breaker.now = func() time.Time { return clock }
	s.(*system).breaker = breaker
	return &clock
}

// TestSequentialSkipsCoolingMember 钉住顺序轮询与熔断的配合：
// 刚失败过的成员本轮不再被选中，号码牌在剩下的成员里继续转。
func TestSequentialSkipsCoolingMember(t *testing.T) {
	system, network, _ := groupFixture(t, types.RotationStrategySequential)
	network.byHost["a.example.test"] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost["b.example.test"] = fakeNetworkReply{response: openAIContentResponse("rescued")}

	if _, err := system.Complete(context.Background(), groupRequest()); err == nil {
		t.Fatalf("首次请求应当失败")
	}
	response, err := system.Complete(context.Background(), groupRequest())
	if err != nil {
		t.Fatalf("第二次 Complete() error = %v", err)
	}
	if response.Content != "rescued" {
		t.Fatalf("content = %q, want rescued", response.Content)
	}
	if len(network.calls) != 2 {
		t.Fatalf("上游调用次数 = %d, want 2（冷却中的成员不应再被尝试）", len(network.calls))
	}
	if hostOfURL(network.calls[1].URL) != "b.example.test" {
		t.Fatalf("第二次打到了 %s, want b.example.test", network.calls[1].URL)
	}
}

// TestRandomStrategyNeverPicksCoolingMember 钉住随机加权同样受熔断约束：
// 关进小黑屋的成员不参与抽签，权重只在剩下的成员里分。
func TestRandomStrategyNeverPicksCoolingMember(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategyWeightedRandom)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIContentResponse("first")}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("second")}
	concreteSystem(t, system).breaker.trip(memberKey("provider-0", "gpt-4.1"))

	for round := 0; round < 10; round++ {
		if _, err := system.Complete(context.Background(), groupRequest()); err != nil {
			t.Fatalf("第 %d 次 Complete() error = %v", round+1, err)
		}
	}
	for index, call := range network.calls {
		if hostOfURL(call.URL) != hosts[1] {
			t.Fatalf("第 %d 次打到了 %s, want %s", index+1, call.URL, hosts[1])
		}
	}
}

// TestFallbackStartsFromNextMemberWhileFirstCools 钉住兜底链首冷却后的收益：
// 首选成员被关之后，后续请求不再白敲一次坏门，直接找替补。
func TestFallbackStartsFromNextMemberWhileFirstCools(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("rescued")}

	if _, err := system.Complete(context.Background(), groupRequest()); err != nil {
		t.Fatalf("首次 Complete() error = %v", err)
	}
	if len(network.calls) != 2 {
		t.Fatalf("首次上游调用次数 = %d, want 2", len(network.calls))
	}
	if _, err := system.Complete(context.Background(), groupRequest()); err != nil {
		t.Fatalf("第二次 Complete() error = %v", err)
	}
	if len(network.calls) != 3 {
		t.Fatalf("累计上游调用次数 = %d, want 3（冷却中的链首不应再被尝试）", len(network.calls))
	}
	if hostOfURL(network.calls[2].URL) != hosts[1] {
		t.Fatalf("第三次打到了 %s, want %s", network.calls[2].URL, hosts[1])
	}
}

// TestAllMembersCoolingStillAttempts 钉住兜底底线：
// 全员都在小黑屋里时照样发起请求，不能因为熔断把整组拖成不可用。
func TestAllMembersCoolingStillAttempts(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIFailureResponse(502)}

	if _, err := system.Complete(context.Background(), groupRequest()); err == nil {
		t.Fatalf("全员失败时应当报错")
	}
	callsAfterFailure := len(network.calls)

	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIContentResponse("recovered")}
	response, err := system.Complete(context.Background(), groupRequest())
	if err != nil {
		t.Fatalf("全员冷却时仍应发起请求，error = %v", err)
	}
	if response.Content != "recovered" {
		t.Fatalf("content = %q, want recovered", response.Content)
	}
	if len(network.calls) <= callsAfterFailure {
		t.Fatalf("全员冷却时没有发起任何请求")
	}
}

// TestRequestErrorDoesNotCoolMember 钉住责任划分：
// 请求本身有错不算成员的账，不该因此把它关起来。
func TestRequestErrorDoesNotCoolMember(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIFailureResponse(400)}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("rescued")}

	if _, err := system.Complete(context.Background(), groupRequest()); err == nil {
		t.Fatalf("请求错误时应当直接失败")
	}
	if _, err := system.Complete(context.Background(), groupRequest()); err == nil {
		t.Fatalf("请求错误时仍应直接失败")
	}
	if len(network.calls) != 2 {
		t.Fatalf("上游调用次数 = %d, want 2（两轮都只碰首选成员）", len(network.calls))
	}
	for index, call := range network.calls {
		if hostOfURL(call.URL) != hosts[0] {
			t.Fatalf("第 %d 次打到了 %s, want %s", index+1, call.URL, hosts[0])
		}
	}
}

// TestStreamFailureAfterContentCoolsMember 钉住记录与换人解耦：
// 已经吐过内容时本次不能换人，但这个成员照样要进小黑屋，后续请求不再白试。
func TestStreamFailureAfterContentCoolsMember(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategyFallback)
	network.byHost[hosts[0]] = fakeNetworkReply{
		response:     types.HTTPResponse{StatusCode: 200},
		streamChunks: []string{streamContentChunk("half")},
		streamErr:    errStreamBroken,
	}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("rescued")}

	if _, err := system.CompleteStream(context.Background(), groupRequest(), func(event types.ModelStreamEvent) error { return nil }); err == nil {
		t.Fatalf("已吐内容后断流应当报错")
	}
	if len(network.calls) != 1 {
		t.Fatalf("首次上游调用次数 = %d, want 1（已吐内容不换人）", len(network.calls))
	}
	if !concreteSystem(t, system).breaker.isCooling(memberKey("provider-0", "gpt-4.1")) {
		t.Fatalf("断流成员没有被关进小黑屋")
	}
	response, err := system.Complete(context.Background(), groupRequest())
	if err != nil {
		t.Fatalf("后续 Complete() error = %v", err)
	}
	if response.Content != "rescued" || hostOfURL(network.calls[len(network.calls)-1].URL) != hosts[1] {
		t.Fatalf("后续请求没有绕开冷却成员：%s", network.calls[len(network.calls)-1].URL)
	}
}

// TestMemberReturnsToSelectionAfterCooldown 钉住放行规则：
// 冷却到期后成员重新参与选择，不会被永久拉黑。
func TestMemberReturnsToSelectionAfterCooldown(t *testing.T) {
	system, network, hosts := groupFixture(t, types.RotationStrategySequential)
	clock := freezeBreakerClock(system, time.Minute)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIFailureResponse(500)}
	network.byHost[hosts[1]] = fakeNetworkReply{response: openAIContentResponse("ok")}

	if _, err := system.Complete(context.Background(), groupRequest()); err == nil {
		t.Fatalf("首次请求应当失败")
	}
	if _, err := system.Complete(context.Background(), groupRequest()); err != nil {
		t.Fatalf("冷却期内的请求 error = %v", err)
	}
	if last := network.calls[len(network.calls)-1].URL; hostOfURL(last) != hosts[1] {
		t.Fatalf("冷却期内打到了 %s, want %s", last, hosts[1])
	}

	*clock = clock.Add(2 * time.Minute)
	network.byHost[hosts[0]] = fakeNetworkReply{response: openAIContentResponse("back")}
	callsBefore := len(network.calls)
	for round := 0; round < 2; round++ {
		if _, err := system.Complete(context.Background(), groupRequest()); err != nil {
			t.Fatalf("恢复后的请求 error = %v", err)
		}
	}
	seen := map[string]bool{}
	for _, call := range network.calls[callsBefore:] {
		seen[hostOfURL(call.URL)] = true
	}
	if !seen[hosts[0]] {
		t.Fatalf("冷却到期后成员一没有重新参与选择")
	}
}

// TestMemberBreakerCooldownExpires 钉住小黑屋会自己放人：到期后成员重新可用。
func TestMemberBreakerCooldownExpires(t *testing.T) {
	breaker := newMemberBreaker(time.Minute)
	clock := time.Now()
	breaker.now = func() time.Time { return clock }
	key := memberKey("provider-0", "gpt-4.1")

	breaker.trip(key)
	if !breaker.isCooling(key) {
		t.Fatalf("关进去之后应当处于冷却")
	}
	if breaker.isCooling("") {
		t.Fatalf("空 key 不应参与熔断")
	}
	clock = clock.Add(2 * time.Minute)
	if breaker.isCooling(key) {
		t.Fatalf("冷却到期后应当放出来")
	}
	if _, ok := breaker.openUntil[key]; ok {
		t.Fatalf("到期记录没有被清掉")
	}

	breaker.trip(key)
	breaker.clear(key)
	if breaker.isCooling(key) {
		t.Fatalf("成功之后应当抹掉记录")
	}
}
