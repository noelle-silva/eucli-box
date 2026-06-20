package placeholder

import (
	"regexp"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

var placeholderPattern = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

type index struct {
	items    map[string][]types.PlaceholderItem
	problems map[string]map[string]struct{}
}

func Resolve(text string, library types.PlaceholderLibrary) types.PlaceholderResolveResult {
	idx := buildIndex(library)
	resolved := resolveText(text, idx, nil)
	return types.PlaceholderResolveResult{Text: resolved, Problems: sortedProblems(idx.problems)}
}

func Problems(library types.PlaceholderLibrary) []types.PlaceholderProblem {
	idx := buildIndex(library)
	for _, item := range library.Placeholders {
		name := strings.TrimSpace(item.Name)
		if name == "" || len(idx.items[name]) != 1 {
			continue
		}
		_, _ = resolveName(name, idx, nil)
	}
	return sortedProblems(idx.problems)
}

func DependencyTree(name string, library types.PlaceholderLibrary) types.PlaceholderDependencyNode {
	idx := buildIndex(library)
	return dependencyNode(strings.TrimSpace(name), idx, nil)
}

func NamesInText(text string) []string {
	matches := placeholderPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func buildIndex(library types.PlaceholderLibrary) *index {
	idx := &index{items: map[string][]types.PlaceholderItem{}, problems: map[string]map[string]struct{}{}}
	for _, item := range library.Placeholders {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		item.Name = name
		idx.items[name] = append(idx.items[name], item)
	}
	for name, items := range idx.items {
		if len(items) > 1 {
			idx.addProblem(name, types.PlaceholderProblemDuplicateName)
		}
	}
	return idx
}

func resolveText(text string, idx *index, stack []string) string {
	return placeholderPattern.ReplaceAllStringFunc(text, func(token string) string {
		matches := placeholderPattern.FindStringSubmatch(token)
		if len(matches) < 2 {
			return token
		}
		name := strings.TrimSpace(matches[1])
		if name == "" {
			return token
		}
		value, ok := resolveName(name, idx, stack)
		if !ok {
			return token
		}
		return value
	})
}

func resolveName(name string, idx *index, stack []string) (string, bool) {
	if cycleStart := stackIndex(stack, name); cycleStart >= 0 {
		for _, cycleName := range stack[cycleStart:] {
			idx.addProblem(cycleName, types.PlaceholderProblemCycleReference)
		}
		idx.addProblem(name, types.PlaceholderProblemCycleReference)
		return tokenForName(name), true
	}
	items := idx.items[name]
	if len(items) == 0 {
		return "", false
	}
	if len(items) > 1 {
		idx.addProblem(name, types.PlaceholderProblemDuplicateName)
		return tokenForName(name), true
	}
	nextStack := append(append([]string(nil), stack...), name)
	resolved := placeholderPattern.ReplaceAllStringFunc(items[0].Value, func(token string) string {
		matches := placeholderPattern.FindStringSubmatch(token)
		if len(matches) < 2 {
			return token
		}
		childName := strings.TrimSpace(matches[1])
		if childName == "" {
			return token
		}
		child, ok := resolveName(childName, idx, nextStack)
		if !ok {
			return token
		}
		if hasProblem(idx.problems, name, types.PlaceholderProblemCycleReference) {
			return tokenForName(name)
		}
		return child
	})
	if hasProblem(idx.problems, name, types.PlaceholderProblemCycleReference) {
		return tokenForName(name), true
	}
	return resolved, true
}

func dependencyNode(name string, idx *index, stack []string) types.PlaceholderDependencyNode {
	node := types.PlaceholderDependencyNode{Name: name}
	if name == "" {
		return node
	}
	if stackIndex(stack, name) >= 0 {
		node.Cycle = true
		return node
	}
	items := idx.items[name]
	if len(items) == 0 {
		node.Missing = true
		return node
	}
	if len(items) > 1 {
		idx.addProblem(name, types.PlaceholderProblemDuplicateName)
		return node
	}
	nextStack := append(append([]string(nil), stack...), name)
	for _, childName := range NamesInText(items[0].Value) {
		node.Children = append(node.Children, dependencyNode(childName, idx, nextStack))
	}
	return node
}

func (idx *index) addProblem(name string, problemType string) {
	name = strings.TrimSpace(name)
	problemType = strings.TrimSpace(problemType)
	if name == "" || problemType == "" {
		return
	}
	if idx.problems[name] == nil {
		idx.problems[name] = map[string]struct{}{}
	}
	idx.problems[name][problemType] = struct{}{}
}

func sortedProblems(source map[string]map[string]struct{}) []types.PlaceholderProblem {
	problems := []types.PlaceholderProblem{}
	for name, typesByName := range source {
		for problemType := range typesByName {
			problems = append(problems, types.PlaceholderProblem{Name: name, Type: problemType})
		}
	}
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Name != problems[j].Name {
			return problems[i].Name < problems[j].Name
		}
		return problems[i].Type < problems[j].Type
	})
	return problems
}

func hasProblem(source map[string]map[string]struct{}, name string, problemType string) bool {
	_, ok := source[name][problemType]
	return ok
}

func stackIndex(stack []string, name string) int {
	for index, item := range stack {
		if item == name {
			return index
		}
	}
	return -1
}

func tokenForName(name string) string {
	return "{{" + name + "}}"
}
