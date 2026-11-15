package toolcalling

import (
	"sync"

	"eucli-box/pkg/types"
)

// toolLiveOutput 汇总一次工具执行期间控制通道推送的实时输出。
//
// 回调来自控制通道自己的接收协程，与执行主干并发，因此读写都在这里加锁。
// 它存在的唯一理由：工具被中断时，宿主手里能证明「已经跑出过什么」的
// 只剩最后一份输出尾窗——这份内容必须能交回结果翻译，而不是随协程结束蒸发。
type toolLiveOutput struct {
	mu      sync.Mutex
	preview string
}

func (o *toolLiveOutput) observe(update types.ToolOutputUpdate) {
	if o == nil {
		return
	}
	preview := update.Preview
	if preview == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.preview = preview
}

// previewSnapshot 返回最后一份实时输出尾窗；没有收到过更新时为空串。
func (o *toolLiveOutput) previewSnapshot() string {
	if o == nil {
		return ""
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.preview
}
