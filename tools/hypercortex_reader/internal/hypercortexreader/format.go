package hypercortexreader

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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

// composeContent 组合正文与信息条：预算只约束正文，信息条不计入预算、永久完整；
// 正文按上限截断（优先落在行边界）；返回标志报告正文是否被截断。
func composeContent(payload string, action string, repoID string, facts []resultFact, maxOutput int) (string, bool) {
	envelope := envelopeLine(action, repoID, facts)
	body, truncated := truncateBody(payload, maxOutput)
	if truncated {
		envelope = envelopeLine(action, repoID, withFact(facts, "truncated", "true"))
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return envelope, truncated
	}
	return body + "\n" + envelope, truncated
}

// composeFailure 组合失败正文与信息条：失败同样携带仓库与动作身份，
// 并把机器可读错误码（如有）放进信息条，供调用方程序化区分错误类型。
func composeFailure(message string, action string, repoID string, metadata map[string]any) string {
	facts := make([]resultFact, 0, 6)
	if dir, ok := metadata["dir"].(string); ok && strings.TrimSpace(dir) != "" {
		facts = append(facts, textFact("dir", dir))
	}
	if noteID, ok := metadata["noteId"].(string); ok && strings.TrimSpace(noteID) != "" {
		facts = append(facts, textFact("noteId", noteID))
	}
	if faceID, ok := metadata["faceId"].(string); ok && strings.TrimSpace(faceID) != "" {
		facts = append(facts, textFact("faceId", faceID))
	}
	if code, ok := metadata["code"].(string); ok && strings.TrimSpace(code) != "" {
		facts = append(facts, textFact("code", code))
	}
	envelope := envelopeLine(action, repoID, facts)
	if strings.TrimSpace(message) == "" {
		return envelope
	}
	return message + "\n" + envelope
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

// truncateBody 在正文字符上限内截断：取预算内最后一个换行处（整行延后，不产生残段）；
// 仅当预算内没有任何换行（首行本身就超限）时才按字符硬切；绝不越过上限。
func truncateBody(text string, limit int) (string, bool) {
	if limit <= 0 {
		return "", strings.TrimRight(text, "\n") != ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text, false
	}
	head := string(runes[:limit])
	if cut := strings.LastIndexByte(head, '\n'); cut >= 0 {
		return strings.TrimRight(head[:cut], "\n"), true
	}
	return head, true
}

// linePacker 在字符预算内装行：整行入预算，装不下的行原样延后，不产生残段。
type linePacker struct {
	lines  []string
	used   int
	budget int
}

func newLinePacker(budget int) *linePacker {
	return &linePacker{budget: budget}
}

// tryAppend 尝试装一行；预算不足时返回 false 且不改变已装内容。
func (p *linePacker) tryAppend(line string) bool {
	need := utf8.RuneCountInString(line)
	if len(p.lines) > 0 {
		need++
	}
	if p.used+need > p.budget {
		return false
	}
	p.lines = append(p.lines, line)
	p.used += need
	return true
}

func (p *linePacker) text() string {
	return strings.Join(p.lines, "\n")
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

// nonEmptyStrings 过滤并修剪字符串列表中的空白项。
func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
