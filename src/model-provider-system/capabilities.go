package modelprovider

import (
	"eucli-box/pkg/types"
)

// reasoningEffortOptions 是思考档位清单的唯一事实源。
//
// label 与 description 只用于客户端渲染，value 是服务端唯一认的提交值。
// 顺序即客户端展示顺序。auto 排在首位：它表示不表态、完全交给模型自决，
// 是新模型最省心的默认选择。
//
// 这份清单必须与 types 包的档位词表是同一集合，由
// TestReasoningEffortOptionsMatchVocabulary 守护，因此加档位时两处都要动，
// 不会出现"客户端能选但服务端不认"的孤岛取值。
var reasoningEffortOptions = []types.CapabilityOption{
	{
		Value:       string(types.ReasoningEffortAuto),
		Label:       "Auto",
		Description: "不指定思考程度，完全不发送思考参数，由模型自身默认行为决定",
	},
	{
		Value:       string(types.ReasoningEffortLow),
		Label:       "Low",
		Description: "轻量思考，适合简单问答",
	},
	{
		Value:       string(types.ReasoningEffortMedium),
		Label:       "Medium",
		Description: "默认档位，兼顾质量与耗时",
	},
	{
		Value:       string(types.ReasoningEffortHigh),
		Label:       "High",
		Description: "深度思考，适合复杂推理与编码",
	},
	{
		Value:       string(types.ReasoningEffortXHigh),
		Label:       "X High",
		Description: "更高强度思考，耗时明显增加",
	},
	{
		Value:       string(types.ReasoningEffortMax),
		Label:       "Max",
		Description: "最大思考强度，适合极难问题",
	},
}

// ReasoningEffortOptions 返回思考档位清单。
// 返回副本：清单是全局共用的事实，调用方改它不该影响别人拿到的下一份。
func ReasoningEffortOptions() []types.CapabilityOption {
	options := make([]types.CapabilityOption, len(reasoningEffortOptions))
	copy(options, reasoningEffortOptions)
	return options
}

// ProtocolOptions 返回协议清单。
// 返回副本的理由同 ReasoningEffortOptions。
func ProtocolOptions() []types.CapabilityOption {
	options := make([]types.CapabilityOption, len(protocolOptions))
	copy(options, protocolOptions)
	return options
}

// Capabilities 汇总服务端对外声明的完整能力清单：
// 客户端据此渲染协议下拉与思考档位下拉，不必复制任何枚举。
func (s *system) Capabilities() types.Capabilities {
	return types.Capabilities{
		Protocols:        ProtocolOptions(),
		ReasoningEfforts: ReasoningEffortOptions(),
	}
}
