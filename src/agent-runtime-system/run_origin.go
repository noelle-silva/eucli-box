package agentruntime

import (
	"strings"

	"eucli-box/pkg/types"
)

// 运行起点统一：调用方只声明起点种类，具体锚点一律由运行在载入会话的同一次读取内解析。
// 追加新消息、从指定用户消息、从指定消息、从会话末条四种输入形态收敛到同一分类来源，
// 是否新建回复、是否标记输入消息等标志统一由起点种类派生。

// runOriginFromRequest 解析运行起点种类：显式声明的种类优先，缺省时按既有输入字段回推。
func runOriginFromRequest(request types.RunRequest) types.RunOrigin {
	if origin := types.NormalizeRunOrigin(request.Origin); origin != "" {
		return origin
	}
	switch {
	case strings.TrimSpace(request.UserMessageID) != "":
		return types.RunOriginUserMessage
	case strings.TrimSpace(request.ContextMessageID) != "":
		return types.RunOriginContextMessage
	default:
		return types.RunOriginMessage
	}
}

// runOriginForcesNewAssistantReply 表示该起点是否总是新起一条助手回复，而不是续写既有助手消息。
func runOriginForcesNewAssistantReply(origin types.RunOrigin) bool {
	return origin == types.RunOriginContextMessage || origin == types.RunOriginSessionTail
}

// runOriginMarksInputMessage 表示该起点是否把锚点当作本次运行输入的用户消息来标记归属。
func runOriginMarksInputMessage(origin types.RunOrigin) bool {
	return origin == types.RunOriginMessage
}
