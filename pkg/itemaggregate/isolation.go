package itemaggregate

import "context"

// Processor 独立处理单项：成功返回值，失败返回原因。
type Processor[T any] func(ctx context.Context, item Item) (T, error)

// Aggregate 对每一项独立调用 process，逐项隔离单点失败：
// 失败被吸收为该项的「不可用」标记，不向聚合调用方传播；
// 聚合永不因单点失败而整体失败，尽力而为地产出结果。
func Aggregate[T any](ctx context.Context, items []Item, process Processor[T]) Result[T] {
	outcomes := make([]Outcome[T], 0, len(items))
	for _, item := range items {
		outcomes = append(outcomes, aggregateItem(ctx, item, process))
	}
	return Result[T]{Outcomes: outcomes}
}

func aggregateItem[T any](ctx context.Context, item Item, process Processor[T]) Outcome[T] {
	value, err := process(ctx, item)
	if err != nil {
		return Outcome[T]{Item: item, Availability: AvailabilityUnavailable, Reason: err.Error()}
	}
	return Outcome[T]{Item: item, Availability: AvailabilityAvailable, Value: value}
}
