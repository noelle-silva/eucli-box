package aiimage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// sessionImage 是会话图片附件的逻辑信息：工具只持逻辑标识，不接触物理路径。
type sessionImage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
}

// sessionServiceOrFail 取会话能力服务；独立运行（无宿主控制通道）时明确失败。
func sessionServiceOrFail(session SessionService) (SessionService, error) {
	if session == nil {
		return nil, errors.New("当前执行不在宿主会话中，无法使用会话图片能力")
	}
	return session, nil
}

// listSessionImages 经会话附件读取能力列出当前会话图片附件。
func listSessionImages(ctx context.Context, session SessionService) ([]sessionImage, error) {
	service, err := sessionServiceOrFail(session)
	if err != nil {
		return nil, err
	}
	result, err := service.Request(ctx, types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessRead, types.SessionAttachmentsReadRequest{Operation: types.SessionAttachmentOperationList})
	if err != nil {
		return nil, fmt.Errorf("读取会话图片清单失败: %w", err)
	}
	if err := capabilityFailure(result); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return nil, fmt.Errorf("会话图片清单响应无效: %w", err)
	}
	var list types.SessionAttachmentsListResult
	if err := json.Unmarshal(payload, &list); err != nil {
		return nil, fmt.Errorf("会话图片清单响应无效: %w", err)
	}
	images := make([]sessionImage, 0, len(list.Attachments))
	for _, attachment := range list.Attachments {
		images = append(images, sessionImage{ID: attachment.ID, Name: attachment.Name, Mime: attachment.Mime})
	}
	return images, nil
}

// loadSessionImage 经会话附件读取能力读取单张图片的 data URL。
func loadSessionImage(ctx context.Context, session SessionService, attachmentID string) (promptImage, error) {
	service, err := sessionServiceOrFail(session)
	if err != nil {
		return promptImage{}, err
	}
	result, err := service.Request(ctx, types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessRead, types.SessionAttachmentsReadRequest{Operation: types.SessionAttachmentOperationRead, AttachmentID: attachmentID})
	if err != nil {
		return promptImage{}, fmt.Errorf("读取会话图片失败: %w", err)
	}
	if err := capabilityFailure(result); err != nil {
		return promptImage{}, err
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return promptImage{}, fmt.Errorf("会话图片响应无效: %w", err)
	}
	var data types.SessionAttachmentData
	if err := json.Unmarshal(payload, &data); err != nil {
		return promptImage{}, fmt.Errorf("会话图片响应无效: %w", err)
	}
	image, err := normalizeImageInput(data.DataURL)
	if err != nil {
		return promptImage{}, fmt.Errorf("会话图片数据无效: %w", err)
	}
	return image, nil
}

// writeSessionImage 经会话附件写入能力把生成结果挂回会话；写入由宿主代办。
func writeSessionImage(ctx context.Context, session SessionService, name string, imageDataURL string) (types.SessionAttachmentInfo, error) {
	service, err := sessionServiceOrFail(session)
	if err != nil {
		return types.SessionAttachmentInfo{}, err
	}
	result, err := service.Request(ctx, types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessWrite, types.SessionAttachmentWriteRequest{Name: name, DataURL: imageDataURL})
	if err != nil {
		return types.SessionAttachmentInfo{}, fmt.Errorf("写入会话图片失败: %w", err)
	}
	if err := capabilityFailure(result); err != nil {
		return types.SessionAttachmentInfo{}, err
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return types.SessionAttachmentInfo{}, fmt.Errorf("会话图片写入响应无效: %w", err)
	}
	var info types.SessionAttachmentInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return types.SessionAttachmentInfo{}, fmt.Errorf("会话图片写入响应无效: %w", err)
	}
	return info, nil
}

// capabilityFailure 把能力响应中的拒绝/失败状态翻译为错误；
// 未授权时透传宿主的标准拒绝文案。
func capabilityFailure(result toolcontrol.CapabilityResult) error {
	switch result.Status {
	case toolcontrol.CapabilityStatusSuccess:
		return nil
	case toolcontrol.CapabilityStatusDenied:
		if strings.TrimSpace(result.Error) != "" {
			return errors.New(result.Error)
		}
		return errors.New(toolcontrol.CapabilityDeniedMessage)
	default:
		if strings.TrimSpace(result.Error) != "" {
			return errors.New(result.Error)
		}
		return errors.New("宿主能力请求失败")
	}
}

// resolveReferenceImages 把 referenceImages 参数解析为图片：支持附件 id、
// 1 起始序号与 data URL / 裸 base64 直接输入。
func resolveReferenceImages(ctx context.Context, session SessionService, references []string) ([]promptImage, error) {
	if len(references) == 0 {
		return nil, nil
	}
	images := make([]promptImage, 0, len(references))
	var listed []sessionImage
	for _, reference := range references {
		value := strings.TrimSpace(reference)
		if value == "" {
			continue
		}
		if isDirectImageInput(value) {
			image, err := normalizeImageInput(value)
			if err != nil {
				return nil, fmt.Errorf("参考图 %q 无效: %w", value, err)
			}
			images = append(images, image)
			continue
		}
		if listed == nil {
			var err error
			listed, err = listSessionImages(ctx, session)
			if err != nil {
				return nil, err
			}
		}
		attachment, err := findSessionImage(listed, value)
		if err != nil {
			return nil, err
		}
		image, err := loadSessionImage(ctx, session, attachment.ID)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, nil
}

// isDirectImageInput 判定参考图是否为直接图片数据（data URL 或裸 base64）。
func isDirectImageInput(value string) bool {
	if strings.HasPrefix(strings.ToLower(value), "data:image/") {
		return true
	}
	return bareBase64Pattern.MatchString(value) && len(value) > 200
}

// findSessionImage 按附件 id 或 1 起始序号定位会话图片。
func findSessionImage(images []sessionImage, reference string) (sessionImage, error) {
	for _, image := range images {
		if image.ID == reference {
			return image, nil
		}
	}
	if index, err := strconv.Atoi(reference); err == nil {
		if index < 1 || index > len(images) {
			return sessionImage{}, fmt.Errorf("会话图片序号越界: %d（共 %d 张）", index, len(images))
		}
		return images[index-1], nil
	}
	return sessionImage{}, fmt.Errorf("会话图片不存在: %s", reference)
}

// runSessionImages 列出当前会话图片清单（序号、id、名称）。
func runSessionImages(ctx context.Context, session SessionService) types.ToolExecutionOutput {
	images, err := listSessionImages(ctx, session)
	if err != nil {
		return failure("list session images", err, nil)
	}
	lines := make([]string, 0, len(images))
	for index, image := range images {
		lines = append(lines, fmt.Sprintf("%d. %s（id: %s）", index+1, image.Name, image.ID))
	}
	if len(lines) == 0 {
		return success("当前会话没有图片附件。", map[string]any{"action": actionSessionImages, "count": 0})
	}
	return success(strings.Join(lines, "\n"), map[string]any{"action": actionSessionImages, "count": len(images), "images": images})
}
