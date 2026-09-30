package sessionimage

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// fakeSession 是会话能力服务的测试替身：固定成功应答。
type fakeSession struct{}

func (f *fakeSession) Request(_ context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error) {
	request, _ := payload.(types.SessionAttachmentReferenceRequest)
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.SessionAttachmentInfo{ID: request.AttachmentID, Name: "历史图", Mime: "image/png"}}, nil
}

func runWith(imageID string) types.ToolExecutionOutput {
	input := types.ToolExecutionInput{ActionID: "t", ToolName: "session_image", Arguments: map[string]any{"imageId": imageID}}
	return Execute(context.Background(), input, &fakeSession{})
}

// TestAttachmentIDValidationDistinguishesFormats 钉住 ID 分级：
// 格式非法、大小写不符与合法 ID 必须返回不同结果。
func TestAttachmentIDValidationDistinguishesFormats(t *testing.T) {
	output := runWith("abc")
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "格式不合法") {
		t.Fatalf("plain text id must fail as malformed: %#v", output)
	}

	upper := runWith("ATT-1790722702891563400-6352")
	if upper.Status != types.ToolStatusFailed || !strings.Contains(upper.Error, "区分大小写") {
		t.Fatalf("uppercase id must explain case sensitivity: %#v", upper)
	}

	truncated := runWith("att-1790722702891563400")
	if truncated.Status != types.ToolStatusFailed || !strings.Contains(truncated.Error, "完整") {
		t.Fatalf("truncated id must ask for the complete id: %#v", truncated)
	}

	valid := runWith("att-1790722702891563400-6352")
	if valid.Status != types.ToolStatusSuccess {
		t.Fatalf("valid id must pass local validation: %#v", valid)
	}
}
