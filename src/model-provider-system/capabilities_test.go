package modelprovider

import (
	"testing"

	"eucli-box/pkg/types"
)

// TestReasoningEffortOptionsMatchVocabulary 守卫"清单 = 词表"这个不变量。
// 两边一旦漂移，就会出现客户端能选、服务端不认的孤岛取值，
// 或者服务端支持、客户端永远选不到的隐形档位。
func TestReasoningEffortOptionsMatchVocabulary(t *testing.T) {
	options := ReasoningEffortOptions()
	if len(options) == 0 {
		t.Fatal("思考档位清单为空")
	}
	seen := map[types.ReasoningEffort]bool{}
	for _, option := range options {
		effort := types.ReasoningEffort(option.Value)
		if seen[effort] {
			t.Fatalf("思考档位清单出现重复项: %q", option.Value)
		}
		seen[effort] = true
		if !types.IsReasoningEffort(effort) {
			t.Fatalf("清单里的 %q 不是词表认可的档位", option.Value)
		}
		if option.Label == "" {
			t.Fatalf("档位 %q 缺少展示名", option.Value)
		}
	}
	// 词表里每一个规范档位都必须在清单里出现，否则客户端永远选不到。
	for _, effort := range []types.ReasoningEffort{
		types.ReasoningEffortAuto,
		types.ReasoningEffortLow,
		types.ReasoningEffortMedium,
		types.ReasoningEffortHigh,
		types.ReasoningEffortXHigh,
		types.ReasoningEffortMax,
	} {
		if !seen[effort] {
			t.Fatalf("词表档位 %q 没有出现在清单里", effort)
		}
	}
}

// TestProtocolOptionsMatchAdapters 守卫"清单 = 适配器取值域"这个不变量：
// 清单里每一项都必须能装配出适配器，装配不了的协议不能对外暴露。
func TestProtocolOptionsMatchAdapters(t *testing.T) {
	options := ProtocolOptions()
	if len(options) == 0 {
		t.Fatal("协议清单为空")
	}
	seen := map[string]bool{}
	for _, option := range options {
		if seen[option.Value] {
			t.Fatalf("协议清单出现重复项: %q", option.Value)
		}
		seen[option.Value] = true
		if _, err := adapterFor(types.ProviderProtocol(option.Value)); err != nil {
			t.Fatalf("清单里的协议 %q 无法装配适配器: %v", option.Value, err)
		}
		if option.Label == "" {
			t.Fatalf("协议 %q 缺少展示名", option.Value)
		}
	}
}

// TestCapabilitiesReturnsCopies 确认清单是副本：
// 调用方改它不该影响下一份，否则一处失误会污染全局事实。
func TestCapabilitiesReturnsCopies(t *testing.T) {
	first := ProtocolOptions()
	first[0].Value = "tampered"
	second := ProtocolOptions()
	if second[0].Value == "tampered" {
		t.Fatal("协议清单被调用方修改后污染了后续读取")
	}

	efforts := ReasoningEffortOptions()
	efforts[0].Label = "tampered"
	if ReasoningEffortOptions()[0].Label == "tampered" {
		t.Fatal("思考档位清单被调用方修改后污染了后续读取")
	}
}

// TestCapabilitiesAggregatesBothLists 确认能力清单把两份清单都带出来，
// 客户端一次请求即可拿到全部下拉项。
func TestCapabilitiesAggregatesBothLists(t *testing.T) {
	capabilities := (&system{}).Capabilities()
	if len(capabilities.Protocols) != len(protocolOptions) {
		t.Fatalf("协议数量 = %d, want %d", len(capabilities.Protocols), len(protocolOptions))
	}
	if len(capabilities.ReasoningEfforts) != len(reasoningEffortOptions) {
		t.Fatalf("档位数量 = %d, want %d", len(capabilities.ReasoningEfforts), len(reasoningEffortOptions))
	}
}
