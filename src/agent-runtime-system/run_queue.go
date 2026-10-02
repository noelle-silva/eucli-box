package agentruntime

// 运行处理队列：一条运行的外部变化统一排进这里，由该运行自己的处理协程
// 按到达顺序串行取走并应用。外部来源是两条真正会改变会话事实的意图：
// 用户停止、异步结果就绪。
//
// 模型吐字、工具挂载、工具状态落定与恢复补写，本就由处理协程在运行主干上
// 就地生成并应用，与队列同属一个写入者，因此不再另立事件。
//
// 这条队列保证：发布方只写入、不等待、不失败；处理协程是唯一消费者，
// 也是该运行会话状态的唯一写入者。

type runEventKind int

const (
	// runEventAsyncReady 表示该运行名下的异步任务已有结果可回灌。
	runEventAsyncReady runEventKind = iota + 1
	// runEventStop 表示用户请求停止该运行。
	runEventStop
)

// runEvent 是排进运行处理队列的一条事件。
type runEvent struct {
	kind   runEventKind
	taskID string
}

const runInboxCapacity = 64

func newRunInbox() chan runEvent {
	return make(chan runEvent, runInboxCapacity)
}

// enqueueRunEvent 把一条外部事件排进运行的处理队列。
// 缓冲满时丢弃：停止另有上下文取消作为硬信号，异步结果只需一次唤醒
// （运行会自行读取全部就绪任务），因此丢弃不会造成会话事实丢失。
func enqueueRunEvent(record *runRecord, event runEvent) {
	if record == nil || record.inbox == nil {
		return
	}
	select {
	case record.inbox <- event:
	default:
	}
}

// drainRunInbox 处理队列里已到达的事件，返回期间是否收到过停止意图。
// 处理协程在安全点调用它，把停止意图转成运行自己的收尾动作。
func (s *system) drainRunInbox(record *runRecord) bool {
	if record == nil || record.inbox == nil {
		return false
	}
	stop := false
	for {
		select {
		case event := <-record.inbox:
			if event.kind == runEventStop {
				stop = true
			}
		default:
			return stop
		}
	}
}
