package types

// 会话级流式输出事实：缺省（无键）即流式；只有显式关闭才落键 "false"。
const SessionMetadataStreamEnabled = "streamEnabled"

func StreamEnabledFromSessionMetadata(metadata map[string]string) bool {
	return metadata[SessionMetadataStreamEnabled] != "false"
}

func PutStreamEnabledSessionMetadata(metadata map[string]string, enabled bool) map[string]string {
	if enabled {
		if len(metadata) == 0 {
			return nil
		}
		out := copySessionMetadata(metadata)
		delete(out, SessionMetadataStreamEnabled)
		if len(out) == 0 {
			return nil
		}
		return out
	}
	out := copySessionMetadata(metadata)
	out[SessionMetadataStreamEnabled] = "false"
	return out
}

func BoolPtr(value bool) *bool {
	return &value
}
