package hypercortexreader

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// resultFact is one model-visible fact appended to a tool result.
type resultFact struct {
	Key   string
	Value string
}

func textFact(key string, value string) resultFact {
	return resultFact{Key: key, Value: value}
}

func intFact(key string, value int) resultFact {
	return resultFact{Key: key, Value: strconv.Itoa(value)}
}

// envelopeLine renders the fixed fact line that ends every result payload.
func envelopeLine(action string, repoID string, facts []resultFact) string {
	parts := []string{"[hypercortex_reader]", "action=" + action, "repo=" + repoID}
	for _, fact := range facts {
		if strings.TrimSpace(fact.Value) == "" {
			continue
		}
		parts = append(parts, fact.Key+"="+fact.Value)
	}
	return strings.Join(parts, " ")
}

// composeContent joins payload and envelope, reserving room for the envelope
// so the model-visible facts survive output truncation. The returned flag
// reports whether the payload itself was cut.
func composeContent(payload string, action string, repoID string, facts []resultFact, maxOutput int) (string, bool) {
	envelope := envelopeLine(action, repoID, facts)
	body, truncated := truncateWithin(payload, maxOutput-len([]rune(envelope))-1)
	if truncated {
		envelope = envelopeLine(action, repoID, withFact(facts, "truncated", "true"))
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return envelope, truncated
	}
	return body + "\n" + envelope, truncated
}

func withFact(facts []resultFact, key string, value string) []resultFact {
	out := make([]resultFact, len(facts))
	copy(out, facts)
	for index := range out {
		if out[index].Key == key {
			out[index].Value = value
			return out
		}
	}
	return append(out, resultFact{Key: key, Value: value})
}

func truncateWithin(text string, limit int) (string, bool) {
	if limit <= 0 {
		return "", strings.TrimRight(text, "\n") != ""
	}
	return truncateRunes(text, limit)
}

// truncateRunes 按字符截断（不切断多字节字符），并报告是否发生截断。
func truncateRunes(text string, limit int) (string, bool) {
	if limit <= 0 {
		return "", text != ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text, false
	}
	return string(runes[:limit]), true
}

// displayTime 把毫秒时间戳渲染为本地可读时间；无效时间返回空串。
func displayTime(ms float64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(int64(ms)).Format("2006-01-02 15:04")
}

// displaySize 把字节数渲染为人类可读的大小。
func displaySize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(bytes)
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PB", value/1024)
}

// joinNonEmpty 连接非空文本，用中文顿号分隔。
func joinNonEmpty(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "、")
}

// firstNonEmpty 返回第一个非空文本。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// oneLine 把多行文本压成单行（连续空白折叠为一个空格），用于列表中的简介与摘要。
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
