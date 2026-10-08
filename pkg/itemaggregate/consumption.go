package itemaggregate

// Available 返回全部可用项的值，即调用方消费的「尽力而为增强结果」。
// 调用方据此获得增强，不把聚合结果当作硬前置或致命条件。
func (r Result[T]) Available() []T {
	values := make([]T, 0, len(r.Outcomes))
	for _, outcome := range r.Outcomes {
		if outcome.IsAvailable() {
			values = append(values, outcome.Value)
		}
	}
	return values
}

// Unavailable 返回全部不可用项，供调用方观测单点失败。
func (r Result[T]) Unavailable() []Outcome[T] {
	outcomes := make([]Outcome[T], 0)
	for _, outcome := range r.Outcomes {
		if !outcome.IsAvailable() {
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes
}
