package filereader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

type grepMatch struct {
	Path string
	Line int
	Text string
}

type grepCollector struct {
	Matches   []grepMatch
	Truncated bool
	Skipped   int
	Max       int
}

func (c *grepCollector) Add(match grepMatch) bool {
	if len(c.Matches) >= c.Max {
		c.Truncated = true
		return false
	}
	c.Matches = append(c.Matches, match)
	return true
}

func runGrep(ctx context.Context, input types.ToolExecutionInput, config Config, policy PathPolicy) types.ToolExecutionOutput {
	pathArg, err := stringArgument(input, "path", false)
	if err != nil {
		return failure("parse grep request", err, nil)
	}
	query, err := stringArgument(input, "query", true)
	if err != nil {
		return failure("parse grep request", err, nil)
	}
	include, err := stringArgument(input, "include", false)
	if err != nil {
		return failure("parse grep request", err, nil)
	}
	ignoreCase, err := boolArgument(input, "ignoreCase", false)
	if err != nil {
		return failure("parse grep request", err, nil)
	}
	showHidden, err := boolArgument(input, "showHidden", false)
	if err != nil {
		return failure("parse grep request", err, nil)
	}
	resolved, err := policy.ResolveExisting(pathArg)
	if err != nil {
		return failure("resolve grep path", err, nil)
	}
	metadata := baseMetadata("grep", resolved)
	metadata["query"] = query
	metadata["include"] = include
	if ignoreCase {
		query = "(?i)" + query
	}
	compiled, err := regexp.Compile(query)
	if err != nil {
		return failure("compile grep query", err, metadata)
	}
	var includeMatcher *globMatcher
	if strings.TrimSpace(include) != "" {
		matcher, err := compileGlob(include)
		if err != nil {
			return failure("compile grep include", err, metadata)
		}
		includeMatcher = &matcher
	}
	maxOutput, err := effectiveMaxOutput(input, config)
	if err != nil {
		return failure("parse output limit", err, metadata)
	}
	collector := &grepCollector{Max: config.MaxSearchResults}
	root := resolved.Absolute
	info, err := os.Stat(root)
	if err != nil {
		return failure("stat grep path", err, metadata)
	}
	if !info.IsDir() {
		if err := grepFile(root, filepath.Dir(root), compiled, includeMatcher, config, policy, collector); err != nil {
			return failure("grep file", err, metadata)
		}
	} else {
		walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				collector.Skipped++
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if path == root {
				return nil
			}
			if entry.IsDir() {
				if shouldSkipWalkDir(entry.Name(), showHidden) {
					return filepath.SkipDir
				}
				return nil
			}
			if !showHidden && isHiddenName(entry.Name()) {
				return nil
			}
			if collector.Truncated {
				return filepath.SkipAll
			}
			return grepFile(path, root, compiled, includeMatcher, config, policy, collector)
		})
		if walkErr != nil {
			return failure("grep files", walkErr, metadata)
		}
	}
	matches := collector.Matches
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return strings.ToLower(matches[i].Path) < strings.ToLower(matches[j].Path)
		}
		return matches[i].Line < matches[j].Line
	})
	lineTruncated := false
	var builder strings.Builder
	for index, match := range matches {
		line, truncated := truncateLine(match.Text, config.MaxLineChars)
		if truncated {
			lineTruncated = true
		}
		builder.WriteString(fmt.Sprintf("%d: %s:%d: %s\n", index+1, filepath.ToSlash(match.Path), match.Line, line))
	}
	facts := []resultFact{
		intFact("resultsCount", len(matches)),
	}
	if collector.Skipped > 0 {
		facts = append(facts, intFact("skippedFiles", collector.Skipped))
	}
	facts = append(facts, boolFact("truncated", collector.Truncated))
	content, outputTruncated := composeContent(builder.String(), "grep", facts, maxOutput)
	metadata["resultsCount"] = len(matches)
	metadata["skippedFiles"] = collector.Skipped
	metadata["truncated"] = collector.Truncated || outputTruncated
	metadata["maxSearchResults"] = config.MaxSearchResults
	if lineTruncated {
		metadata["lineTruncated"] = true
	}
	return success(content, metadata)
}

func grepFile(path string, root string, compiled *regexp.Regexp, includeMatcher *globMatcher, config Config, policy PathPolicy, collector *grepCollector) error {
	if collector.Truncated {
		return nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	if includeMatcher != nil && !includeMatcher.MatchFilter(rel) {
		return nil
	}
	resolved, err := policy.ResolveExisting(path)
	if err != nil {
		collector.Skipped++
		return nil
	}
	data, _, err := readTextFile(resolved.Absolute, config.MaxFileBytes)
	if err != nil {
		collector.Skipped++
		return nil
	}
	decoded, err := decodeText(data)
	if err != nil {
		collector.Skipped++
		return nil
	}
	lines := splitLines(decoded.Text)
	for index, line := range lines {
		if compiled.MatchString(line) {
			if !collector.Add(grepMatch{Path: displayPath(policy.baseDir, resolved.Absolute), Line: index + 1, Text: line}) {
				return nil
			}
		}
	}
	return nil
}
