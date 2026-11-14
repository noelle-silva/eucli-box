package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"eucli-box/pkg/types"
)

// modelGroupStrategies 是策略清单接口该给出的三项，顺序即客户端下拉的展示顺序。
func modelGroupStrategies() []types.RotationStrategyOption {
	return []types.RotationStrategyOption{
		{Value: types.RotationStrategySequential, Label: "顺序轮询", Description: "按成员顺序轮流使用，均摊负载"},
		{Value: types.RotationStrategyWeightedRandom, Label: "权重随机", Description: "按成员权重随机抽取"},
		{Value: types.RotationStrategyFallback, Label: "故障转移", Description: "始终从第一位开始，失败才顺位换下一个"},
	}
}

// TestModelGroupStrategyRoute 钉住策略清单接口的对外契约：
// 响应包成 {"data":[...]}，逐项带 value/label/description，顺序与事实源一致，
// 且必含 fallback——客户端靠它渲染兜底选项。
func TestModelGroupStrategyRoute(t *testing.T) {
	fakes := newGatewayFakes()
	// 清单的事实源在供应商系统，网关只转发；这里按真实清单装配，验证接线与序列化。
	fakes.providers.strategies = modelGroupStrategies()
	system := newTestGateway(t, fakes)

	recorder := httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/model-groups/strategies", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data []types.RotationStrategyOption `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	want := modelGroupStrategies()
	if len(payload.Data) != len(want) {
		t.Fatalf("策略数量 = %d, want %d", len(payload.Data), len(want))
	}
	for index, expect := range want {
		if payload.Data[index] != expect {
			t.Fatalf("第 %d 项 = %#v, want %#v", index+1, payload.Data[index], expect)
		}
	}
	if payload.Data[len(want)-1].Value != types.RotationStrategyFallback {
		t.Fatalf("清单末项 = %q, want %q", payload.Data[len(want)-1].Value, types.RotationStrategyFallback)
	}
}

// TestModelGroupStrategyRouteRequiresGatewayKey 钉住鉴权与既有 /api/model-groups 一致：
// 配了网关固定 Key 之后，没带 Key 的请求拿不到策略清单，带了才给。
func TestModelGroupStrategyRouteRequiresGatewayKey(t *testing.T) {
	const key = "model-group-strategies-key"
	fakes := newGatewayFakes()
	fakes.providers.strategies = modelGroupStrategies()
	system := newTestGatewayWithConfig(t, fakes, Config{Addr: "127.0.0.1:0", Key: key, SessionChanges: fakes.sessionChanges})

	anonymous := httptest.NewRecorder()
	system.Handler().ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/model-groups/strategies", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("没有 Key 时 status = %d, want 401 body=%s", anonymous.Code, anonymous.Body.String())
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/model-groups/strategies", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer "+key)
	authorized := httptest.NewRecorder()
	system.Handler().ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("带 Key 时 status = %d, want 200 body=%s", authorized.Code, authorized.Body.String())
	}
}
