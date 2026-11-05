// Package workspaceicon 定义工作区身份图标的载体语义与安全规则。
//
// 图标只有两种载体：图像文件（存储事实是相对工作区数据目录的路径）与内联 SVG 源码。
// 「什么是合法图标」的判定在本包收敛：存储侧写入与读取、网关侧提交都只经由本包判定，
// 不各自重复一份规则。SVG 是能执行脚本的活动内容，因此这里只做「拒绝」，
// 不做静默剔除——改写用户交上来的图标等于偷偷改变它表达的内容。
package workspaceicon

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"eucli-box/pkg/types"
)

// MaxSVGBytes 是内联 SVG 源码的字节上限。图标是身份视觉，不属于内容载体。
const MaxSVGBytes = 64 * 1024

// forbiddenElements 是静态身份图标不需要、且能承载脚本或外部取内容的元素。
// 内联样式表一并拒绝：样式规则里的 url() 与 @import 是绕过属性校验的现成通道，
// 图标用属性表达样式已经足够。
var forbiddenElements = map[string]struct{}{
	"script":           {},
	"style":            {},
	"foreignobject":    {},
	"iframe":           {},
	"object":           {},
	"embed":            {},
	"audio":            {},
	"video":            {},
	"animate":          {},
	"animatemotion":    {},
	"animatetransform": {},
	"set":              {},
	"discard":          {},
	"handler":          {},
	"listener":         {},
}

// NormalizeContent 校验一次写入提交：两种载体的字段互斥，交出哪一种就必须只交哪一种。
// 缺字段或两种都交都直接失败，不做「猜哪份才是真的」的兜底。
func NormalizeContent(content types.WorkspaceIconContent) (types.WorkspaceIconContent, error) {
	kind := strings.TrimSpace(content.Kind)
	switch kind {
	case types.WorkspaceIconKindImage:
		if strings.TrimSpace(content.SVG) != "" {
			return types.WorkspaceIconContent{}, errors.New("image workspace icon must not carry svg markup")
		}
		dataURL := strings.TrimSpace(content.DataURL)
		if dataURL == "" {
			return types.WorkspaceIconContent{}, errors.New("image workspace icon requires a data url")
		}
		return types.WorkspaceIconContent{Kind: kind, DataURL: dataURL}, nil
	case types.WorkspaceIconKindSVG:
		if strings.TrimSpace(content.DataURL) != "" {
			return types.WorkspaceIconContent{}, errors.New("svg workspace icon must not carry an image data url")
		}
		markup, err := NormalizeSVG(content.SVG)
		if err != nil {
			return types.WorkspaceIconContent{}, err
		}
		return types.WorkspaceIconContent{Kind: kind, SVG: markup}, nil
	default:
		return types.WorkspaceIconContent{}, fmt.Errorf("workspace icon kind must be %q or %q", types.WorkspaceIconKindImage, types.WorkspaceIconKindSVG)
	}
}

// NormalizeStored 校验并归一化一条存储事实：载体重合、路径逃逸、SVG 不合规都在这里失败。
func NormalizeStored(icon types.WorkspaceIcon) (types.WorkspaceIcon, error) {
	kind := strings.TrimSpace(icon.Kind)
	switch kind {
	case types.WorkspaceIconKindImage:
		if strings.TrimSpace(icon.SVG) != "" {
			return types.WorkspaceIcon{}, errors.New("image workspace icon must not carry svg markup")
		}
		relativePath, err := normalizeRelativePath(icon.Path)
		if err != nil {
			return types.WorkspaceIcon{}, err
		}
		return types.WorkspaceIcon{Kind: kind, Path: relativePath}, nil
	case types.WorkspaceIconKindSVG:
		if strings.TrimSpace(icon.Path) != "" {
			return types.WorkspaceIcon{}, errors.New("svg workspace icon must not carry an image path")
		}
		markup, err := NormalizeSVG(icon.SVG)
		if err != nil {
			return types.WorkspaceIcon{}, err
		}
		return types.WorkspaceIcon{Kind: kind, SVG: markup}, nil
	default:
		return types.WorkspaceIcon{}, fmt.Errorf("workspace icon kind must be %q or %q", types.WorkspaceIconKindImage, types.WorkspaceIconKindSVG)
	}
}

// NormalizeSVG 校验内联 SVG 并返回去掉首尾空白后的源码。
// 任何可能执行脚本、绑定事件或向文档外取内容的构造一律拒绝。
func NormalizeSVG(raw string) (string, error) {
	markup := strings.TrimSpace(raw)
	if markup == "" {
		return "", errors.New("svg workspace icon requires svg markup")
	}
	if len(markup) > MaxSVGBytes {
		return "", fmt.Errorf("svg workspace icon exceeds %d bytes", MaxSVGBytes)
	}
	decoder := xml.NewDecoder(strings.NewReader(markup))
	rootSeen := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("svg workspace icon is not well-formed xml: %w", err)
		}
		switch typed := token.(type) {
		case xml.Directive:
			return "", errors.New("svg workspace icon must not declare doctype or entities")
		case xml.ProcInst:
			if strings.EqualFold(typed.Target, "xml") {
				continue
			}
			return "", fmt.Errorf("svg workspace icon must not contain processing instruction %q", typed.Target)
		case xml.StartElement:
			if !rootSeen {
				if typed.Name.Local != "svg" {
					return "", fmt.Errorf("svg workspace icon root element must be <svg>, got <%s>", typed.Name.Local)
				}
				rootSeen = true
			}
			if err := checkElement(typed); err != nil {
				return "", err
			}
		}
	}
	if !rootSeen {
		return "", errors.New("svg workspace icon has no root element")
	}
	return markup, nil
}

func checkElement(element xml.StartElement) error {
	if _, forbidden := forbiddenElements[strings.ToLower(element.Name.Local)]; forbidden {
		return fmt.Errorf("svg workspace icon must not contain <%s>", element.Name.Local)
	}
	for _, attribute := range element.Attr {
		if err := checkAttribute(attribute); err != nil {
			return err
		}
	}
	return nil
}

func checkAttribute(attribute xml.Attr) error {
	name := strings.ToLower(attribute.Name.Local)
	value := strings.TrimSpace(attribute.Value)
	if strings.HasPrefix(name, "on") {
		return fmt.Errorf("svg workspace icon must not bind event attribute %q", attribute.Name.Local)
	}
	lowered := strings.ToLower(value)
	if strings.Contains(lowered, "javascript:") || strings.Contains(lowered, "vbscript:") {
		return fmt.Errorf("svg workspace icon attribute %q carries a script url", attribute.Name.Local)
	}
	if name == "href" || name == "src" || name == "srcset" {
		// 引用只允许指向文档内片段；外部地址会让渲染方去取域外内容。
		if !strings.HasPrefix(value, "#") {
			return fmt.Errorf("svg workspace icon reference %q must point inside the document", attribute.Name.Local)
		}
	}
	if strings.Contains(lowered, "url(") {
		return checkUrlFunctions(attribute.Name.Local, lowered)
	}
	return nil
}

// checkUrlFunctions 校验 url(...) 只引用文档内片段（渐变、裁剪、滤镜都以片段表达）：
// 外部地址会让渲染方成为取图通道。
func checkUrlFunctions(attributeName string, loweredValue string) error {
	rest := loweredValue
	for {
		index := strings.Index(rest, "url(")
		if index < 0 {
			return nil
		}
		rest = rest[index+len("url("):]
		closing := strings.Index(rest, ")")
		if closing < 0 {
			return fmt.Errorf("svg workspace icon attribute %q has an unterminated url()", attributeName)
		}
		target := strings.Trim(strings.TrimSpace(rest[:closing]), `"'`)
		if !strings.HasPrefix(target, "#") {
			return fmt.Errorf("svg workspace icon attribute %q references content outside the document", attributeName)
		}
		rest = rest[closing+1:]
	}
}

// normalizeRelativePath 归一化图标图像的存储路径：必须是干净的相对斜杠路径，
// 不带盘符、不越出图标目录。存储事实只保留一种写法，读取方不必猜。
func normalizeRelativePath(raw string) (string, error) {
	relativePath := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if relativePath == "" {
		return "", errors.New("image workspace icon path is required")
	}
	if strings.Contains(relativePath, ":") {
		return "", errors.New("image workspace icon path must not contain a volume name")
	}
	if path.IsAbs(relativePath) || path.Clean(relativePath) != relativePath {
		return "", fmt.Errorf("image workspace icon path must be a clean relative path, got %q", raw)
	}
	for _, segment := range strings.Split(relativePath, "/") {
		if segment == ".." {
			return "", fmt.Errorf("image workspace icon path must not escape the icon directory, got %q", raw)
		}
	}
	return relativePath, nil
}
