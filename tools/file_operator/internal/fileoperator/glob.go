package fileoperator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

type pathMatch struct {
	Path     string
	Display  string
	IsDir    bool
	Size     int64
	Modified int64
}

func runGlob(ctx context.Context, input types.ToolExecutionInput, config Config, policy PathPolicy) types.ToolExecutionOutput {
	pathArg, err := stringArgument(input, "path", false)
	if err != nil {
		return failure("parse glob request", err, nil)
	}
	pattern, err := stringArgument(input, "pattern", true)
	if err != nil {
		return failure("parse glob request", err, nil)
	}
	resolved, err := policy.ResolveExisting(pathArg)
	if err != nil {
		return failure("resolve glob path", err, nil)
	}
	showHidden, err := boolArgument(input, "showHidden", false)
	if err != nil {
		return failure("parse glob request", err, baseMetadata("glob", resolved))
	}
	maxOutput, err := effectiveMaxOutput(input, config)
	if err != nil {
		return failure("parse output limit", err, baseMetadata("glob", resolved))
	}
	metadata := baseMetadata("glob", resolved)
	metadata["pattern"] = pattern
	results := make([]pathMatch, 0)
	root := resolved.Absolute
	info, err := os.Stat(root)
	if err != nil {
		return failure("stat glob path", err, metadata)
	}
	if !info.IsDir() {
		root = filepath.Dir(root)
	}
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == root {
			return nil
		}
		if entry.IsDir() && shouldSkipWalkDir(entry.Name(), showHidden) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && !showHidden && isHiddenName(entry.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if !globMatches(pattern, rel) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		results = append(results, pathMatch{Path: path, Display: displayPath(policy.hostRoot, path), IsDir: entry.IsDir(), Size: info.Size(), Modified: info.ModTime().UnixNano()})
		return nil
	})
	if walkErr != nil {
		return failure("glob files", walkErr, metadata)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Modified != results[j].Modified {
			return results[i].Modified > results[j].Modified
		}
		return strings.ToLower(results[i].Display) < strings.ToLower(results[j].Display)
	})
	truncated := false
	if len(results) > config.MaxSearchResults {
		results = results[:config.MaxSearchResults]
		truncated = true
	}
	var builder strings.Builder
	for index, item := range results {
		kind := "file"
		name := item.Display
		if item.IsDir {
			kind = "directory"
			name += "/"
		}
		builder.WriteString(fmt.Sprintf("%d: %s\t%s\t%d\n", index+1, filepath.ToSlash(name), kind, item.Size))
	}
	content, outputTruncated := truncateText(builder.String(), maxOutput)
	metadata["resultsCount"] = len(results)
	metadata["truncated"] = truncated || outputTruncated
	metadata["maxSearchResults"] = config.MaxSearchResults
	return success(content, metadata)
}
