package toolcalling

// SetMessageSystem 接入「发送会话消息」能力的实现方。
// 装配顺序决定运行系统在工具系统之后才存在，因此这里以后接入的方式补齐依赖；
// 未接入前该能力明确失败，不静默伪造结果。
func (s *system) SetMessageSystem(messenger MessageSystem) error {
	if messenger == nil {
		return toolInvalid("message system dependency is required", nil)
	}
	s.messageMu.Lock()
	defer s.messageMu.Unlock()
	s.message = messenger
	return nil
}

// messageSystem 读取当前接入的实现方；每次执行都取最新值，不在执行期缓存。
func (s *system) messageSystem() MessageSystem {
	s.messageMu.RLock()
	defer s.messageMu.RUnlock()
	return s.message
}
