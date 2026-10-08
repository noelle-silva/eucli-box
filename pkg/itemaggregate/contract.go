// Package itemaggregate 提供「逐项容错聚合」统一机制。
//
// 顶层契约：对一组同类项逐项独立处理，单点失败被隔离为该项的「不可用」标记，
// 聚合永不因单点失败而整体失败，尽力而为地产出「结果 + 逐项可用性」。
// 该结构面向同类聚合问题（工具清单、成员角色、批量认领等）可复用，
// 不绑定任何具体业务语义。
package itemaggregate

// Availability 是单项在聚合结果中的可用性标记。
type Availability string

const (
	// AvailabilityAvailable 表示该项处理成功、结果可用。
	AvailabilityAvailable Availability = "available"
	// AvailabilityUnavailable 表示该项处理失败、已被隔离；聚合其余项不受影响。
	AvailabilityUnavailable Availability = "unavailable"
)

// Item 是聚合中的一个待处理项，只承载身份，不绑定具体业务语义。
type Item struct {
	ID   string
	Name string
}

// Outcome 是单项的处理结果：可用时携带值，不可用时携带失败原因。
type Outcome[T any] struct {
	Item         Item
	Availability Availability
	Value        T
	Reason       string
}

// IsAvailable 报告该项是否可用。
func (o Outcome[T]) IsAvailable() bool {
	return o.Availability == AvailabilityAvailable
}

// Result 是聚合产出：尽力而为的结果 + 逐项可用性，取代「全有或全无」。
type Result[T any] struct {
	Outcomes []Outcome[T]
}
