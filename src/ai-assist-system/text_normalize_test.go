package aiassist

import (
	"strings"
	"testing"
)

func TestReplaceMermaidFenceOnceMatchesTrimmedSource(t *testing.T) {
	source := "graph TD\n    A --> B"
	markdown := "```mermaid\n" + source + " \n```"
	updated, ok := replaceMermaidFenceOnce(markdown, source, "graph TD\n    A --> C")
	if !ok {
		t.Fatal("replaceMermaidFenceOnce() = false")
	}
	want := "```mermaid\ngraph TD\n    A --> C\n```"
	if updated != want {
		t.Fatalf("updated = %q, want %q", updated, want)
	}
}

func TestReplaceMermaidFenceOnceMatchesPaddedFenceBody(t *testing.T) {
	markdown := "前言\n\n```mermaid\n\n\ngraph TD\n    A --> B\n\n```\n\n后记"
	updated, ok := replaceMermaidFenceOnce(markdown, "graph TD\n    A --> B", "graph LR\n    A --> B")
	if !ok {
		t.Fatal("replaceMermaidFenceOnce() = false")
	}
	want := "前言\n\n```mermaid\ngraph LR\n    A --> B\n```\n\n后记"
	if updated != want {
		t.Fatalf("updated = %q, want %q", updated, want)
	}
}

func TestReplaceMermaidFenceOncePreservesCRLF(t *testing.T) {
	markdown := "```mermaid\r\ngraph TD\r\n    A --> B\r\n```\r\n"
	updated, ok := replaceMermaidFenceOnce(markdown, "graph TD\r\n    A --> B", "graph TD\r\n    A --> C")
	if !ok {
		t.Fatal("replaceMermaidFenceOnce() = false")
	}
	want := "```mermaid\r\ngraph TD\r\n    A --> C\r\n```\r\n"
	if updated != want {
		t.Fatalf("updated = %q, want %q", updated, want)
	}
}

func TestReplaceMermaidFenceOnceAcceptsFlowchartAndGraphLanguages(t *testing.T) {
	for _, language := range []string{"flowchart", "graph"} {
		t.Run(language, func(t *testing.T) {
			markdown := "```" + language + "\ngraph TD\n    A --> B\n```"
			updated, ok := replaceMermaidFenceOnce(markdown, "graph TD\n    A --> B", "graph TD\n    A --> C")
			if !ok {
				t.Fatal("replaceMermaidFenceOnce() = false")
			}
			if !strings.Contains(updated, "A --> C") || !strings.Contains(updated, "```"+language) {
				t.Fatalf("updated = %q", updated)
			}
		})
	}
}

func TestReplaceMermaidFenceOnceIgnoresOtherLanguages(t *testing.T) {
	markdown := "```js\ngraph TD\n    A --> B\n```"
	if _, ok := replaceMermaidFenceOnce(markdown, "graph TD\n    A --> B", "graph TD\n    A --> C"); ok {
		t.Fatal("replaceMermaidFenceOnce() matched a non-mermaid fence")
	}
}

func TestReplaceMermaidFenceOnceIgnoresUnclosedFence(t *testing.T) {
	markdown := "```mermaid\ngraph TD\n    A --> B"
	if _, ok := replaceMermaidFenceOnce(markdown, "graph TD\n    A --> B", "graph TD\n    A --> C"); ok {
		t.Fatal("replaceMermaidFenceOnce() matched an unclosed fence")
	}
}

func TestReplaceMermaidFenceOnceReplacesFirstMatchOnly(t *testing.T) {
	source := "graph TD\n    A --> B"
	markdown := "```mermaid\n" + source + "\n```\n\n```mermaid\n" + source + "\n```"
	updated, ok := replaceMermaidFenceOnce(markdown, source, "graph TD\n    A --> C")
	if !ok {
		t.Fatal("replaceMermaidFenceOnce() = false")
	}
	if strings.Count(updated, "A --> C") != 1 || strings.Count(updated, "A --> B") != 1 {
		t.Fatalf("updated = %q", updated)
	}
}
