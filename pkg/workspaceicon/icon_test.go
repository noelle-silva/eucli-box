package workspaceicon

import (
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

const validSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><defs><linearGradient id="g"><stop offset="0" stop-color="#c2410c"/></linearGradient></defs><path fill="url(#g)" d="M2 2h20v20H2z"/></svg>`

func TestNormalizeSVGAcceptsStaticIcon(t *testing.T) {
	markup, err := NormalizeSVG("  " + validSVG + "\n")
	if err != nil {
		t.Fatalf("NormalizeSVG() error = %v", err)
	}
	if markup != validSVG {
		t.Fatalf("NormalizeSVG() = %q, want trimmed markup", markup)
	}
}

func TestNormalizeSVGRejectsActiveContent(t *testing.T) {
	cases := map[string]string{
		"empty":              "   ",
		"event attribute":    `<svg onload="alert(1)"/>`,
		"script element":     `<svg><script>alert(1)</script></svg>`,
		"style element":      `<svg><style>path{fill:red}</style></svg>`,
		"foreign object":     `<svg><foreignObject><div/></foreignObject></svg>`,
		"external image":     `<svg><image href="https://example.com/a.png"/></svg>`,
		"script url":         `<svg><a href="javascript:alert(1)">x</a></svg>`,
		"external url()":     `<svg><path fill="url(https://example.com/x.svg#a)"/></svg>`,
		"unterminated url()": `<svg><path fill="url(#a"/></svg>`,
		"doctype":            `<!DOCTYPE svg SYSTEM "about:legacy-compat"><svg/>`,
		"entity":             `<!ENTITY xxe SYSTEM "file:///etc/passwd"><svg/>`,
		"wrong root":         `<div/>`,
		"malformed xml":      `<svg>`,
		"oversized":          `<svg>` + strings.Repeat("a", MaxSVGBytes) + `</svg>`,
	}
	for name, markup := range cases {
		if _, err := NormalizeSVG(markup); err == nil {
			t.Fatalf("%s: NormalizeSVG() accepted forbidden markup", name)
		}
	}
}

func TestNormalizeContentKeepsCarriersExclusive(t *testing.T) {
	content, err := NormalizeContent(types.WorkspaceIconContent{Kind: types.WorkspaceIconKindSVG, SVG: validSVG})
	if err != nil {
		t.Fatalf("NormalizeContent(svg) error = %v", err)
	}
	if content.SVG != validSVG || content.DataURL != "" {
		t.Fatalf("NormalizeContent(svg) = %#v", content)
	}

	content, err = NormalizeContent(types.WorkspaceIconContent{Kind: types.WorkspaceIconKindImage, DataURL: "data:image/png;base64,AAAA"})
	if err != nil {
		t.Fatalf("NormalizeContent(image) error = %v", err)
	}
	if content.DataURL != "data:image/png;base64,AAAA" || content.SVG != "" {
		t.Fatalf("NormalizeContent(image) = %#v", content)
	}

	rejected := map[string]types.WorkspaceIconContent{
		"missing kind":      {SVG: validSVG},
		"unknown kind":      {Kind: "font", SVG: validSVG},
		"image without url": {Kind: types.WorkspaceIconKindImage},
		"image with svg":    {Kind: types.WorkspaceIconKindImage, DataURL: "data:image/png;base64,AAAA", SVG: validSVG},
		"svg with url":      {Kind: types.WorkspaceIconKindSVG, SVG: validSVG, DataURL: "data:image/png;base64,AAAA"},
		"svg without svg":   {Kind: types.WorkspaceIconKindSVG},
	}
	for name, input := range rejected {
		if _, err := NormalizeContent(input); err == nil {
			t.Fatalf("%s: NormalizeContent() accepted invalid content", name)
		}
	}
}

func TestNormalizeStoredImagePathMustStayInIconDirectory(t *testing.T) {
	stored, err := NormalizeStored(types.WorkspaceIcon{Kind: types.WorkspaceIconKindImage, Path: "attachments/icon/icon.png"})
	if err != nil {
		t.Fatalf("NormalizeStored() error = %v", err)
	}
	if stored.Path != "attachments/icon/icon.png" {
		t.Fatalf("NormalizeStored() path = %q", stored.Path)
	}

	rejected := map[string]types.WorkspaceIcon{
		"empty path":     {Kind: types.WorkspaceIconKindImage},
		"parent escape":  {Kind: types.WorkspaceIconKindImage, Path: "../icon.png"},
		"absolute path":  {Kind: types.WorkspaceIconKindImage, Path: "/etc/icon.png"},
		"volume name":    {Kind: types.WorkspaceIconKindImage, Path: "C:/icon.png"},
		"unclean path":   {Kind: types.WorkspaceIconKindImage, Path: "attachments//icon.png"},
		"image with svg": {Kind: types.WorkspaceIconKindImage, Path: "attachments/icon/icon.png", SVG: validSVG},
		"svg with path":  {Kind: types.WorkspaceIconKindSVG, SVG: validSVG, Path: "attachments/icon/icon.png"},
		"unknown kind":   {Kind: "font"},
	}
	for name, input := range rejected {
		if _, err := NormalizeStored(input); err == nil {
			t.Fatalf("%s: NormalizeStored() accepted invalid fact", name)
		}
	}
}
