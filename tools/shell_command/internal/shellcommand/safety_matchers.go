package shellcommand

import (
	"strings"
	"unicode"
)

func matchesCriticalRecursiveDelete(command string) bool {
	for _, segment := range hardlineCommandSegments(command) {
		tokens := tokenizeHardlineSegment(segment)
		if len(tokens) < 2 || !isCriticalDeleteCommand(tokens[0]) {
			continue
		}

		recursive, force := false, false
		criticalPath := false
		for _, token := range tokens[1:] {
			if isCriticalPathToken(token) {
				criticalPath = true
			}
			if isRecursiveDeleteFlag(token) {
				recursive = true
			}
			if isForceDeleteFlag(token) {
				force = true
			}
		}
		if criticalPath && recursive && force {
			return true
		}
	}
	return false
}

func isCriticalDeleteCommand(token string) bool {
	switch hardlineCommandName(token) {
	case "remove-item", "ri", "rm", "del", "erase", "rd", "rmdir":
		return true
	default:
		return false
	}
}

func isRecursiveDeleteFlag(token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "-r" || token == "--recursive" || token == "-recurse" || token == "/s" {
		return true
	}
	if strings.HasPrefix(token, "-recurse:") {
		return powerShellSwitchEnabled(token, "-recurse:")
	}
	if strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--") {
		return containsShortDeleteFlag(strings.TrimPrefix(token, "-"), 'r')
	}
	return false
}

func isForceDeleteFlag(token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "-f" || token == "--force" || token == "-force" || token == "/f" || token == "/q" {
		return true
	}
	if strings.HasPrefix(token, "-force:") {
		return powerShellSwitchEnabled(token, "-force:")
	}
	if strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--") {
		return containsShortDeleteFlag(strings.TrimPrefix(token, "-"), 'f')
	}
	return false
}

func powerShellSwitchEnabled(token string, prefix string) bool {
	value := strings.TrimSpace(strings.TrimPrefix(token, prefix))
	return value != "$false" && value != "false" && value != "0"
}

func containsShortDeleteFlag(flags string, target rune) bool {
	if flags == "" || len(flags) > 4 || !strings.ContainsRune(flags, target) {
		return false
	}
	for _, flag := range flags {
		if !strings.ContainsRune("rRfFiIv", flag) {
			return false
		}
	}
	return true
}

func isCriticalPathToken(token string) bool {
	path := hardlinePathValue(token)
	if path == "" {
		return false
	}
	if path == "/" || path == "~" || path == "$home" || path == "$env:windir" || path == "%windir%" || path == "%systemroot%" {
		return true
	}
	if strings.HasPrefix(path, "$home/") || strings.HasPrefix(path, "$home\\") ||
		strings.HasPrefix(path, "$env:windir/") || strings.HasPrefix(path, "$env:windir\\") ||
		strings.HasPrefix(path, "%windir%/") || strings.HasPrefix(path, "%windir%\\") ||
		strings.HasPrefix(path, "%systemroot%/") || strings.HasPrefix(path, "%systemroot%\\") {
		return true
	}

	if strings.HasPrefix(path, "/") {
		first := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)[0]
		switch first {
		case "bin", "etc", "home", "root", "sbin", "usr", "var":
			return true
		}
	}

	if len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\') {
		remainder := strings.Trim(path[3:], "/\\")
		if remainder == "" {
			return true
		}
		first := strings.FieldsFunc(remainder, func(r rune) bool { return r == '/' || r == '\\' })[0]
		return first == "windows" || first == "users"
	}
	return false
}

func hardlinePathValue(token string) string {
	token = strings.Trim(strings.TrimSpace(token), "\"'")
	if key, value, ok := strings.Cut(token, "="); ok && strings.HasPrefix(key, "-") {
		token = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return token
}

func matchesSystemShutdownCommand(command string) bool {
	for _, segment := range hardlineCommandSegments(command) {
		tokens := tokenizeHardlineSegment(segment)
		if len(tokens) < 2 {
			continue
		}
		switch hardlineCommandName(tokens[0]) {
		case "shutdown", "reboot", "poweroff", "halt", "restart-computer", "stop-computer":
			return true
		case "systemctl":
			for _, token := range tokens[1:] {
				if token == "--user" {
					break
				}
				if strings.HasPrefix(token, "-") {
					continue
				}
				if token == "halt" || token == "poweroff" || token == "reboot" {
					return true
				}
				break
			}
		case "init", "telinit":
			for _, token := range tokens[1:] {
				if token == "--" || strings.HasPrefix(token, "-") {
					continue
				}
				return token == "0" || token == "6"
			}
		}
	}
	return false
}

func matchesShredDevice(command string) bool {
	for _, segment := range hardlineCommandSegments(command) {
		tokens := tokenizeHardlineSegment(segment)
		if len(tokens) < 2 || hardlineCommandName(tokens[0]) != "shred" {
			continue
		}
		for _, token := range tokens[1:] {
			if isRawDiskDeviceToken(token) {
				return true
			}
		}
	}
	return false
}

func matchesWipefsDevice(command string) bool {
	for _, segment := range hardlineCommandSegments(command) {
		tokens := tokenizeHardlineSegment(segment)
		if len(tokens) < 3 || hardlineCommandName(tokens[0]) != "wipefs" {
			continue
		}
		all := false
		device := false
		for _, token := range tokens[1:] {
			if isAllWipefsFlag(token) {
				all = true
			}
			if isRawDiskDeviceToken(token) {
				device = true
			}
		}
		if all && device {
			return true
		}
	}
	return false
}

func matchesPartedMklabel(command string) bool {
	for _, segment := range hardlineCommandSegments(command) {
		tokens := tokenizeHardlineSegment(segment)
		if len(tokens) < 4 || hardlineCommandName(tokens[0]) != "parted" {
			continue
		}
		script := false
		device := false
		mklabel := false
		for _, token := range tokens[1:] {
			switch token {
			case "-s", "--script":
				script = true
			case "mklabel":
				mklabel = true
			}
			if isRawDiskDeviceToken(token) {
				device = true
			}
		}
		if script && device && mklabel {
			return true
		}
	}
	return false
}

func isAllWipefsFlag(token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	return token == "-a" || token == "--all" || (strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--") && strings.Contains(strings.TrimPrefix(token, "-"), "a"))
}

func isRawDiskDeviceToken(token string) bool {
	device := hardlinePathValue(token)
	if strings.HasPrefix(device, `\\.\physicaldrive`) {
		return true
	}
	if !strings.HasPrefix(device, "/dev/") {
		return false
	}
	name := strings.TrimPrefix(device, "/dev/")
	return strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "hd") ||
		strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") ||
		strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "mmcblk")
}

func hardlineCommandName(token string) string {
	name := strings.ToLower(strings.Trim(strings.TrimSpace(token), "\"'"))
	if index := strings.LastIndexAny(name, "/\\"); index >= 0 {
		name = name[index+1:]
	}
	return strings.TrimSuffix(name, ".exe")
}

func splitHardlineCommandSegments(command string) []string {
	runes := []rune(command)
	segments := make([]string, 0, 1)
	start := 0
	var quote rune
	for index, current := range runes {
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == ';' || current == '|' || current == '&' {
			segments = append(segments, string(runes[start:index]))
			start = index + 1
		}
	}
	segments = append(segments, string(runes[start:]))
	return segments
}

const maxHardlineWrapperDepth = 8

func hardlineCommandSegments(command string) []string {
	var segments []string
	collectHardlineCommandSegments(command, 0, &segments)
	return segments
}

func collectHardlineCommandSegments(command string, depth int, segments *[]string) {
	for _, segment := range splitHardlineCommandSegments(command) {
		*segments = append(*segments, segment)
		if depth >= maxHardlineWrapperDepth {
			continue
		}
		tokens := tokenizeHardlineSegment(segment)
		if inner, ok := unwrapHardlineCommand(tokens); ok {
			collectHardlineCommandSegments(inner, depth+1, segments)
		}
	}
}

func unwrapHardlineCommand(tokens []string) (string, bool) {
	if len(tokens) < 2 {
		return "", false
	}
	switch hardlineCommandName(tokens[0]) {
	case "sudo":
		return unwrapSudoCommand(tokens)
	case "exec", "command", "nohup":
		return strings.Join(tokens[1:], " "), true
	case "env":
		index := 1
		for index < len(tokens) {
			if tokens[index] == "--" {
				index++
				break
			}
			if tokens[index] == "-i" || tokens[index] == "--ignore-environment" || strings.Contains(tokens[index], "=") {
				index++
				continue
			}
			break
		}
		if index < len(tokens) {
			return strings.Join(tokens[index:], " "), true
		}
	case "trap":
		index := 1
		if index < len(tokens) && tokens[index] == "--" {
			index++
		}
		if index < len(tokens) && !strings.HasPrefix(tokens[index], "-") {
			return tokens[index], true
		}
	case "bash", "dash", "ksh", "sh", "zsh", "pwsh", "powershell", "cmd":
		return unwrapShellScript(tokens)
	}
	return "", false
}

func unwrapSudoCommand(tokens []string) (string, bool) {
	index := 1
	for index < len(tokens) {
		argument := tokens[index]
		if argument == "--" {
			index++
			break
		}
		if argument == "-u" || argument == "--user" || argument == "-g" || argument == "--group" ||
			argument == "-h" || argument == "--host" || argument == "-p" || argument == "--prompt" ||
			argument == "-C" || argument == "--chdir" {
			index += 2
			continue
		}
		if strings.HasPrefix(argument, "-") {
			index++
			continue
		}
		break
	}
	if index < len(tokens) {
		return strings.Join(tokens[index:], " "), true
	}
	return "", false
}

func unwrapShellScript(tokens []string) (string, bool) {
	for index := 1; index < len(tokens); index++ {
		argument := strings.ToLower(tokens[index])
		switch argument {
		case "-c", "-lc", "-cl", "/c", "-command", "/command":
			if index+1 < len(tokens) {
				return tokens[index+1], true
			}
		case "-command:", "/command:":
			if index+1 < len(tokens) {
				return tokens[index+1], true
			}
		default:
			for _, prefix := range []string{"-command:", "/command:"} {
				if strings.HasPrefix(argument, prefix) && len(tokens[index]) > len(prefix) {
					return tokens[index][len(prefix):], true
				}
			}
		}
	}
	return "", false
}

func tokenizeHardlineSegment(segment string) []string {
	var tokens []string
	var token strings.Builder
	var quote rune
	flush := func() {
		if token.Len() > 0 {
			tokens = append(tokens, token.String())
			token.Reset()
		}
	}
	for _, current := range segment {
		if quote != 0 {
			if current == quote {
				quote = 0
			} else {
				token.WriteRune(current)
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if unicode.IsSpace(current) {
			flush()
			continue
		}
		token.WriteRune(current)
	}
	flush()
	return tokens
}
