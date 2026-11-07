package toolcalling

// SetConfirmationBridge 接入「工具转呈外部确认」能力的实现方。
// 与消息能力同理：运行系统在工具系统之后才建成，因此以后接入的方式补齐依赖；
// 未接入前该能力明确失败，不静默伪造裁决结果。
func (s *system) SetConfirmationBridge(bridge ConfirmationBridge) error {
	if bridge == nil {
		return toolInvalid("confirmation bridge dependency is required", nil)
	}
	s.confirmationMu.Lock()
	defer s.confirmationMu.Unlock()
	s.confirmation = bridge
	return nil
}

// confirmationBridge 读取当前接入的实现方；每次执行都取最新值，不在执行期缓存。
func (s *system) confirmationBridge() ConfirmationBridge {
	s.confirmationMu.RLock()
	defer s.confirmationMu.RUnlock()
	return s.confirmation
}
