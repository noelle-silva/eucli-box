package aiimage

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// adaptersDirName 是适配文件在工具配置区内的固定子目录名。
const adaptersDirName = "adapters"

// adapterPlaceholderPattern 匹配适配模板中的占位符 {{name}}。
var adapterPlaceholderPattern = regexp.MustCompile(`\{\{([A-Za-z0-9_]+)\}\}`)

// 适配模板允许的占位符名。
const (
	placeholderAPIKey        = "apiKey"
	placeholderModel         = "model"
	placeholderPrompt        = "prompt"
	placeholderIndex         = "index"
	placeholderExt           = "ext"
	defaultFormImageFilename = "image-{{index}}.{{ext}}"
	wholeValueImages         = "{{images}}"
	wholeValueImagesBase64   = "{{imagesBase64}}"
)

// adapterFile 是一份声明式适配文件的完整结构：描述如何拼请求、如何取结果。
type adapterFile struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Request     adapterRequest  `json:"request"`
	Response    adapterResponse `json:"response"`
}

type adapterRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	JSON    json.RawMessage   `json:"json"`
	Form    *adapterForm      `json:"form"`
}

type adapterForm struct {
	Fields map[string]string `json:"fields"`
	Images adapterFormImages `json:"images"`
}

type adapterFormImages struct {
	Field    string `json:"field"`
	Filename string `json:"filename"`
}

type adapterResponse struct {
	ImagePath string `json:"imagePath"`
}

// parseAdapterFile 解析并校验一份适配文件；id 必须等于文件名。
func parseAdapterFile(expectedID string, content string) (adapterFile, error) {
	var adapter adapterFile
	if err := decodeStrictJSON(content, &adapter); err != nil {
		return adapterFile{}, fmt.Errorf("适配文件不是合法 JSON: %w", err)
	}
	adapter = normalizeAdapterFile(adapter)
	if err := validateAdapterFile(expectedID, adapter); err != nil {
		return adapterFile{}, err
	}
	return adapter, nil
}

// loadAdapter 从配置区读取并校验适配文件（adapters/<id>.json）。
func loadAdapter(root string, adapterID string) (adapterFile, error) {
	if !validConfigID(adapterID) {
		return adapterFile{}, fmt.Errorf("适配 id 无效: %q", adapterID)
	}
	content, _, err := readConfigFile(root, adaptersDirName+"/"+adapterID+".json")
	if err != nil {
		return adapterFile{}, err
	}
	return parseAdapterFile(adapterID, content)
}

func normalizeAdapterFile(adapter adapterFile) adapterFile {
	adapter.ID = strings.TrimSpace(adapter.ID)
	adapter.Name = strings.TrimSpace(adapter.Name)
	adapter.Description = strings.TrimSpace(adapter.Description)
	adapter.Request.Method = strings.ToUpper(strings.TrimSpace(adapter.Request.Method))
	if adapter.Request.Method == "" {
		adapter.Request.Method = http.MethodPost
	}
	adapter.Request.Path = strings.TrimSpace(adapter.Request.Path)
	headers := map[string]string{}
	for key, value := range adapter.Request.Headers {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		headers[trimmedKey] = strings.TrimSpace(value)
	}
	adapter.Request.Headers = headers
	if form := adapter.Request.Form; form != nil {
		form.Images.Field = strings.TrimSpace(form.Images.Field)
		form.Images.Filename = strings.TrimSpace(form.Images.Filename)
		if form.Images.Filename == "" {
			form.Images.Filename = defaultFormImageFilename
		}
		fields := map[string]string{}
		for key, value := range form.Fields {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			fields[trimmedKey] = value
		}
		form.Fields = fields
	}
	adapter.Response.ImagePath = strings.TrimSpace(adapter.Response.ImagePath)
	return adapter
}

func validateAdapterFile(expectedID string, adapter adapterFile) error {
	if !validConfigID(adapter.ID) {
		return fmt.Errorf("适配 id 无效: %q", adapter.ID)
	}
	if adapter.ID != expectedID {
		return fmt.Errorf("适配 id %q 与文件名不一致（应为 %q）", adapter.ID, expectedID)
	}
	if err := validateAdapterPath(adapter.Request.Path); err != nil {
		return err
	}
	switch adapter.Request.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("request.method 无效: %q", adapter.Request.Method)
	}
	hasJSON := len(adapter.Request.JSON) > 0
	hasForm := adapter.Request.Form != nil
	if hasJSON == hasForm {
		return errors.New("request 必须且只能提供 json 或 form 之一")
	}
	if hasJSON {
		var document any
		if err := json.Unmarshal(adapter.Request.JSON, &document); err != nil {
			return fmt.Errorf("request.json 不是合法 JSON: %w", err)
		}
	}
	if hasForm && adapter.Request.Form.Images.Field == "" {
		return errors.New("request.form.images.field 不能为空")
	}
	if adapter.Response.ImagePath != "" {
		if err := validateAdapterImagePath(adapter.Response.ImagePath); err != nil {
			return err
		}
	}
	return validateAdapterPlaceholders(adapter)
}

// validateAdapterPath 校验请求路径是相对 baseUrl 的安全路径。
func validateAdapterPath(value string) error {
	if value == "" {
		return errors.New("request.path 不能为空")
	}
	if strings.Contains(value, "://") {
		return errors.New("request.path 必须是相对 baseUrl 的路径")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return errors.New("request.path 不能包含 ..")
		}
	}
	return nil
}

// validateAdapterImagePath 校验取图路径是合法的点分路径。
func validateAdapterImagePath(value string) error {
	if strings.ContainsAny(value, "{}") {
		return errors.New("response.imagePath 是取值路径，不能包含占位符")
	}
	for _, segment := range strings.Split(value, ".") {
		if strings.TrimSpace(segment) == "" {
			return errors.New("response.imagePath 包含空分段")
		}
	}
	return nil
}

// validateAdapterPlaceholders 校验模板占位符：未知占位符在写入时就明确失败。
func validateAdapterPlaceholders(adapter adapterFile) error {
	if err := validateSimpleTemplate(adapter.Request.Path, "request.path", placeholderAPIKey, placeholderModel, placeholderPrompt); err != nil {
		return err
	}
	for key, value := range adapter.Request.Headers {
		if err := validateSimpleTemplate(value, "request.headers."+key, placeholderAPIKey, placeholderModel, placeholderPrompt); err != nil {
			return err
		}
	}
	if form := adapter.Request.Form; form != nil {
		for key, value := range form.Fields {
			if err := validateSimpleTemplate(value, "request.form.fields."+key, placeholderAPIKey, placeholderModel, placeholderPrompt); err != nil {
				return err
			}
		}
		if err := validateSimpleTemplate(form.Images.Filename, "request.form.images.filename", placeholderIndex, placeholderExt); err != nil {
			return err
		}
	}
	if len(adapter.Request.JSON) > 0 {
		var document any
		if err := json.Unmarshal(adapter.Request.JSON, &document); err != nil {
			return fmt.Errorf("request.json 不是合法 JSON: %w", err)
		}
		if err := validateJSONTemplate(document, "request.json"); err != nil {
			return err
		}
	}
	return nil
}

func validateSimpleTemplate(value string, location string, allowed ...string) error {
	for _, match := range adapterPlaceholderPattern.FindAllStringSubmatch(value, -1) {
		name := match[1]
		if !containsString(allowed, name) {
			return fmt.Errorf("%s 含未知占位符 {{%s}}（可用: %s）", location, name, strings.Join(allowed, ", "))
		}
	}
	return nil
}

func validateJSONTemplate(value any, location string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if err := validateJSONTemplate(item, location+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range typed {
			if err := validateJSONTemplate(item, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
	case string:
		if typed == wholeValueImages || typed == wholeValueImagesBase64 {
			return nil
		}
		if err := validateSimpleTemplate(typed, location, placeholderAPIKey, placeholderModel, placeholderPrompt); err != nil {
			return err
		}
	}
	return nil
}

// adapterRenderContext 是渲染一份适配请求所需的全部值。
type adapterRenderContext struct {
	APIKey string
	Model  string
	Prompt string
	Images []promptImage
}

// renderAdapterString 替换字符串中的 apiKey / model / prompt 占位符。
func renderAdapterString(template string, ctx adapterRenderContext) string {
	return adapterPlaceholderPattern.ReplaceAllStringFunc(template, func(match string) string {
		switch match[2 : len(match)-2] {
		case placeholderAPIKey:
			return ctx.APIKey
		case placeholderModel:
			return ctx.Model
		case placeholderPrompt:
			return ctx.Prompt
		default:
			return match
		}
	})
}

// renderAdapterJSON 渲染 JSON 体模板：整值占位符 {{images}} / {{imagesBase64}}
// 展开为数组，其余字符串按 apiKey / model / prompt 替换。
func renderAdapterJSON(template json.RawMessage, ctx adapterRenderContext) ([]byte, error) {
	var document any
	if err := json.Unmarshal(template, &document); err != nil {
		return nil, fmt.Errorf("request.json 不是合法 JSON: %w", err)
	}
	rendered, err := renderJSONValue(document, ctx)
	if err != nil {
		return nil, err
	}
	return marshalCanonical(rendered)
}

func renderJSONValue(value any, ctx adapterRenderContext) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			rendered, err := renderJSONValue(item, ctx)
			if err != nil {
				return nil, err
			}
			typed[key] = rendered
		}
		return typed, nil
	case []any:
		for index, item := range typed {
			rendered, err := renderJSONValue(item, ctx)
			if err != nil {
				return nil, err
			}
			typed[index] = rendered
		}
		return typed, nil
	case string:
		switch typed {
		case wholeValueImages:
			images := make([]any, 0, len(ctx.Images))
			for _, image := range ctx.Images {
				images = append(images, image.DataURL)
			}
			return images, nil
		case wholeValueImagesBase64:
			images := make([]any, 0, len(ctx.Images))
			for _, image := range ctx.Images {
				images = append(images, image.Base64)
			}
			return images, nil
		default:
			return renderAdapterString(typed, ctx), nil
		}
	default:
		return value, nil
	}
}

// renderFormFilename 渲染 multipart 图片文件名中的序号与扩展名占位符。
func renderFormFilename(template string, index int, ext string) string {
	result := strings.ReplaceAll(template, "{{"+placeholderIndex+"}}", strconv.Itoa(index))
	return strings.ReplaceAll(result, "{{"+placeholderExt+"}}", ext)
}

// containsString 判定字符串是否在允许集合中。
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
