package shellcommand

import (
	"fmt"
	"regexp"
	"strings"

	"eucli-box/pkg/types"
)

type hardlineBlock struct {
	Rule   string
	Reason string
}

type hardlineRule struct {
	name     string
	reason   string
	patterns []*regexp.Regexp
}

var hardlineRules = []hardlineRule{
	{
		name:   "recursive-delete-critical-path",
		reason: "the command recursively deletes a critical system, root, or home directory",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?:^|[;&|]\s*)rm\s+-[^\s]*r[^\s]*\s+(?:--\s+)?(?:/|/(?:etc|bin|sbin|usr|var|home|root)(?:\s|/|$)|~(?:\s|/|$)|\$home(?:\s|/|$)|[a-z]:[\\/](?:\s|$)|[a-z]:[\\/](?:windows|users)(?:[\\/\s]|$))`),
			regexp.MustCompile(`(?:^|[;&|]\s*)(?:rd|rmdir)\s+/(?:s|q)\s+/(?:s|q)\s+(?:[a-z]:\\(?:\s|$)|[a-z]:\\(?:windows|users)(?:\\|\s|$)|%systemroot%|%windir%)`),
		},
	},
	{
		name:   "format-filesystem",
		reason: "the command formats or creates a filesystem",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?:^|[;&|]\s*)(?:mkfs(?:\.[a-z0-9_-]+)?|mke2fs|format)(?:\s|$)`),
		},
	},
	{
		name:   "raw-disk-write",
		reason: "the command writes directly to a raw disk device",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?:^|[;&|]\s*)dd\s+.*\bof=(?:/dev/(?:sd[a-z]\d*|hd[a-z]\d*|vd[a-z]\d*|nvme\d+n\d+(?:p\d+)?|mmcblk\d+(?:p\d+)?)|\\\\\.\\physicaldrive\d+)`),
			regexp.MustCompile(`>\s*(?:/dev/(?:sd[a-z]\d*|hd[a-z]\d*|vd[a-z]\d*|nvme\d+n\d+(?:p\d+)?|mmcblk\d+(?:p\d+)?)|\\\\\.\\physicaldrive\d+)`),
		},
	},
	{
		name:   "fork-bomb",
		reason: "the command matches a fork bomb pattern",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*}\s*;\s*:`),
		},
	},
	{
		name:   "kill-all-processes",
		reason: "the command attempts to terminate all processes",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?:^|[;&|]\s*)kill\s+-(?:9|kill)\s+-1(?:\s|$)`),
			regexp.MustCompile(`(?:^|[;&|]\s*)taskkill\s+/f\s+/im\s+\*(?:\s|$)`),
		},
	},
	{
		name:   "shutdown-or-reboot",
		reason: "the command shuts down, restarts, or powers off the machine",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?:^|[;&|]\s*)(?:shutdown|reboot|poweroff|halt|restart-computer|stop-computer)(?:\s|$)`),
		},
	},
}

func checkHardlineCommand(command string) (hardlineBlock, bool) {
	normalized := normalizeCommandForHardline(command)
	if normalized == "" {
		return hardlineBlock{}, false
	}
	for _, rule := range hardlineRules {
		if matchesAnyHardlinePattern(normalized, rule.patterns) {
			return hardlineBlock{Rule: rule.name, Reason: rule.reason}, true
		}
	}
	return hardlineBlock{}, false
}

func normalizeCommandForHardline(command string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(command)))
	return strings.Join(fields, " ")
}

func matchesAnyHardlinePattern(command string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

func hardlineDeniedOutput(block hardlineBlock) types.ToolExecutionOutput {
	message := fmt.Sprintf("shell_command refused to run this command because it matches a hardline safety rule: %s. The command was not executed. If you intentionally want to run it, use your own terminal manually.", block.Reason)
	metadata := map[string]any{
		"error":           message,
		"denied":          true,
		"hardlineBlocked": true,
		"hardlineRule":    block.Rule,
		"reason":          block.Reason,
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusDenied, Content: message, Error: message, Metadata: metadata}
}
