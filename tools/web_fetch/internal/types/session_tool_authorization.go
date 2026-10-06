package types

import "strings"

// 会话级工具放行：用户对某工具选择“本会话内始终同意”后，
// 会话档案里为这个工具记一条放行标记，随会话持久保存；
// 本会话内该工具的所有调用直接执行，不再看路径参数。
const SessionMetadataToolAuthorizationPrefix = "toolAuthorization."

func sessionToolAuthorizationKey(toolID string) string {
	return SessionMetadataToolAuthorizationPrefix + strings.TrimSpace(toolID)
}

// SessionToolAuthorized 判断工具是否已在会话档案中被放行。
func SessionToolAuthorized(metadata map[string]string, toolID string) bool {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" || len(metadata) == 0 {
		return false
	}
	return metadata[sessionToolAuthorizationKey(toolID)] == "true"
}

// PutSessionToolAuthorization 在会话 metadata 上增删某工具的放行标记；
// 关闭即移除键，不留下无意义占位。
func PutSessionToolAuthorization(metadata map[string]string, toolID string, authorized bool) map[string]string {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		return metadata
	}
	out := copySessionMetadata(metadata)
	key := sessionToolAuthorizationKey(toolID)
	if authorized {
		out[key] = "true"
	} else {
		delete(out, key)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
