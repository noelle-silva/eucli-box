package filereader

import (
	"fmt"
	"path/filepath"
	"strings"
)

// globMatcher is a compiled path pattern. Matching is case-sensitive on every
// platform so one pattern behaves the same everywhere.
//
// Semantics:
//   - `*` and `?` never cross a `/` boundary.
//   - `**` as a whole segment spans zero or more path segments, so `**/*.go`
//     also matches a root-level file.
//   - A pattern without `/` matches the root level only when used as a path
//     pattern; the include filter compares the base name instead.
type globMatcher struct {
	segments []string
	hasSlash bool
}

func compileGlob(pattern string) (globMatcher, error) {
	normalized := filepath.ToSlash(strings.TrimSpace(pattern))
	if normalized == "" {
		return globMatcher{}, fmt.Errorf("pattern is required")
	}
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		if segment == "" {
			return globMatcher{}, fmt.Errorf("pattern %q contains an empty path segment", pattern)
		}
	}
	return globMatcher{segments: segments, hasSlash: strings.Contains(normalized, "/")}, nil
}

// Match reports whether the pattern matches the whole path.
func (m globMatcher) Match(relPath string) bool {
	normalized := filepath.ToSlash(relPath)
	if normalized == "" || normalized == "." {
		return false
	}
	return matchGlobSegments(m.segments, strings.Split(normalized, "/"))
}

// MatchFilter reports whether the pattern accepts a file path as a search
// filter. A pattern without `/` is a file-name filter applied at any depth;
// a pattern with `/` is matched against the path like Match.
func (m globMatcher) MatchFilter(relPath string) bool {
	if m.hasSlash {
		return m.Match(relPath)
	}
	return m.Match(baseName(relPath))
}

func matchGlobSegments(pattern []string, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		if matchGlobSegments(pattern[1:], path) {
			return true
		}
		if len(path) == 0 {
			return false
		}
		return matchGlobSegments(pattern, path[1:])
	}
	if len(path) == 0 {
		return false
	}
	if !matchGlobSegment(pattern[0], path[0]) {
		return false
	}
	return matchGlobSegments(pattern[1:], path[1:])
}

// matchGlobSegment matches one path segment: `*` spans any run inside the
// segment, `?` matches exactly one character.
func matchGlobSegment(pattern string, name string) bool {
	patternRunes := []rune(pattern)
	nameRunes := []rune(name)
	patternIndex := 0
	nameIndex := 0
	starIndex := -1
	retryIndex := 0
	for nameIndex < len(nameRunes) {
		switch {
		case patternIndex < len(patternRunes) && (patternRunes[patternIndex] == '?' || patternRunes[patternIndex] == nameRunes[nameIndex]):
			patternIndex++
			nameIndex++
		case patternIndex < len(patternRunes) && patternRunes[patternIndex] == '*':
			starIndex = patternIndex
			retryIndex = nameIndex
			patternIndex++
		case starIndex >= 0:
			patternIndex = starIndex + 1
			retryIndex++
			nameIndex = retryIndex
		default:
			return false
		}
	}
	for patternIndex < len(patternRunes) && patternRunes[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(patternRunes)
}

func baseName(relPath string) string {
	normalized := filepath.ToSlash(relPath)
	if index := strings.LastIndex(normalized, "/"); index >= 0 {
		return normalized[index+1:]
	}
	return normalized
}

func shouldSkipWalkDir(name string, showHidden bool) bool {
	return !showHidden && isHiddenName(name)
}
