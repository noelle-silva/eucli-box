package fileoperator

import (
	"path/filepath"
	"regexp"
	"strings"
)

func globMatches(pattern string, relPath string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	relPath = filepath.ToSlash(relPath)
	if pattern == "" || pattern == "**" || pattern == "**/*" {
		return true
	}
	if ok, err := filepath.Match(pattern, relPath); err == nil && ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, err := filepath.Match(pattern, filepath.Base(relPath)); err == nil && ok {
			return true
		}
	}
	re, err := regexp.Compile(globToRegex(pattern))
	if err != nil {
		return false
	}
	return re.MatchString(relPath)
}

func globToRegex(pattern string) string {
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				builder.WriteString(".*")
				i++
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			builder.WriteByte('\\')
			builder.WriteByte(ch)
		default:
			builder.WriteByte(ch)
		}
	}
	builder.WriteString("$")
	return builder.String()
}

func shouldSkipWalkDir(name string, showHidden bool) bool {
	return !showHidden && isHiddenName(name)
}
