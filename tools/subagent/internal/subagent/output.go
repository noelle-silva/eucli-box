package subagent

import (
	"fmt"
	"strings"

	"eucli-box/tools/subagent/internal/types"
)

// sessionHeader 是「会话身份」这一行的唯一出处。
//
// 只要会话已经建立，它就必须出现在正文最前面：成功、失败、取消都一样，
// 拿到结果的人第一时间就能凭它回到那个会话，不必去正文里翻找。
func sessionHeader(sessionID string) string {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return ""
	}
	return "会话 " + id
}

// establishedMetadata 是「会话已建立」这类结果的公共元数据：
// 机器读取的一方也能直接拿到身份，不必解析正文。
func establishedMetadata(sessionID string, roleID string) map[string]any {
	metadata := map[string]any{}
	if id := strings.TrimSpace(sessionID); id != "" {
		metadata["sessionId"] = id
	}
	if id := strings.TrimSpace(roleID); id != "" {
		metadata["roleId"] = id
	}
	return metadata
}

// successOutput 落定一次成功的发送：会话身份打头，随后是角色答复。
func successOutput(answer types.ToolMessageResult, extra map[string]any) types.ToolExecutionOutput {
	metadata := establishedMetadata(answer.SessionID, answer.RoleID)
	metadata = mergeMetadata(metadata, extra)
	metadata["runId"] = strings.TrimSpace(answer.RunID)
	metadata["messageId"] = strings.TrimSpace(answer.MessageID)
	metadata["message"] = answer.Message
	body := fmt.Sprintf("角色 %s 已回复（消息 %s）：\n%s", answer.RoleID, answer.MessageID, answer.Message)
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusSuccess,
		Content:  appendHeader(sessionHeader(answer.SessionID), body),
		Metadata: metadata,
	}
}

// failedOutput 落定一次失败的发送：会话身份同样打头，原因跟在后面。
// 失败不改变「会话已经建立」这个既成事实，所以正文第一行照样是会话身份。
func failedOutput(sessionID string, roleID string, message string, err error, extra map[string]any) types.ToolExecutionOutput {
	errorMessage := message
	if err != nil {
		errorMessage = message + ": " + err.Error()
	}
	metadata := mergeMetadata(establishedMetadata(sessionID, roleID), extra)
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusFailed,
		Content:  appendHeader(sessionHeader(sessionID), errorMessage),
		Error:    errorMessage,
		Metadata: metadata,
	}
}

// mergeMetadata 把补充事实并进公共元数据；事实字段随后写入，不会被补充项盖掉。
func mergeMetadata(metadata map[string]any, extra map[string]any) map[string]any {
	for key, value := range extra {
		if _, exists := metadata[key]; exists {
			continue
		}
		metadata[key] = value
	}
	return metadata
}

// appendHeader 把会话身份那一行接在正文前面；没有会话身份时正文保持原样。
func appendHeader(header string, body string) string {
	if header == "" {
		return body
	}
	return header + "\n" + body
}
