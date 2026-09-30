package aiimage

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

// runGenerate 执行一次生图：加载配置、解析参考图、按协议请求、回写会话。
func runGenerate(ctx context.Context, input types.ToolExecutionInput, args arguments, session SessionService) types.ToolExecutionOutput {
	if args.Prompt == "" {
		return failure("generate image", permanentError(fmt.Errorf("缺少 prompt 参数"), actionFixParams), nil)
	}
	root, err := configRoot(input)
	if err != nil {
		return failure("generate image", err, nil)
	}
	config, err := loadProviderConfig(root)
	if err != nil {
		return failure("load provider config", err, nil)
	}
	provider, err := resolveProvider(config, args.Provider)
	if err != nil {
		return failure("select provider", err, map[string]any{"provider": args.Provider})
	}
	if args.Model != "" {
		provider.DefaultModel = strings.TrimSpace(args.Model)
	}
	references, err := resolveReferenceImages(ctx, session, args.ReferenceImages)
	if err != nil {
		return failure("resolve reference images", err, map[string]any{"provider": provider.ID})
	}
	network, err := networkrequest.NewSystem(networkrequest.Config{UserAgent: "eucli-box-ai-image/1.0"})
	if err != nil {
		return failure("initialize network", err, map[string]any{"provider": provider.ID})
	}
	result, err := callProvider(ctx, network, root, provider, generationRequest{Prompt: args.Prompt, ReferenceImages: references}, args.TimeoutMs)
	if err != nil {
		return failure("execute generation request", err, map[string]any{"provider": provider.ID, "protocol": provider.Protocol})
	}
	attachment, err := writeSessionImage(ctx, session, generatedImageName(provider.ID), result.ImageDataURL)
	if err != nil {
		return failure("write generated image to session", err, map[string]any{"provider": provider.ID, "protocol": result.ProtocolKind})
	}
	content := fmt.Sprintf("图片已生成并挂回会话（运营商 %s，协议 %s，附件 %s）。", provider.ID, result.ProtocolKind, attachment.ID)
	return success(content, map[string]any{
		"action":       actionGenerate,
		"provider":     provider.ID,
		"protocol":     result.ProtocolKind,
		"statusCode":   result.StatusCode,
		"durationMs":   result.DurationMs,
		"attachmentId": attachment.ID,
		"attachment":   attachment,
	})
}

// generatedImageName 生成结果附件的展示名。
func generatedImageName(providerID string) string {
	return "ai-image " + providerID
}
