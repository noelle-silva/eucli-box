package agentruntime

// 运行处理队列：一条运行的外部变化统一排进这里，由该运行自己的处理协程
// 按到达顺序串行取走并应用。外部来源是三条真正会改变会话事实的意图：
// 用户停止、异步结果就绪、工具确认提交。
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
	// runEventToolConfirmation 表示外部提交了一次工具确认决定。
	runEventToolConfirmation
)

// runEvent 是排进运行处理队列的一条事件。
type runEvent struct {
	kind         runEventKind
	taskID       string
	confirmation *toolConfirmationRequest
}

const runInboxCapacity = 64

func newRunInbox() chan runEvent {
	return make(chan runEvent, runInboxCapacity)
}

// enqueueRunEvent 把一条外部事件排进运行的处理队列，返回是否成功入队。
// 缓冲满时丢弃：停止另有上下文取消作为硬信号，异步结果只需一次唤醒
// （运行会自行读取全部就绪任务），因此丢弃不会造成会话事实丢失；
// 工具确认需要回执，投递方据此判定是否送达。
func enqueueRunEvent(record *runRecord, event runEvent) bool {
	if record == nil || record.inbox == nil {
		return false
	}
	select {
	case record.inbox <- event:
		return true
	default:
		return false
	}
}

// drainRunInbox 处理队列里已到达的事件，返回期间是否收到过停止意图。
// 处理协程在安全点调用它：停止意图转成运行自己的收尾动作；
// 非等待确认时到达的确认意图当场回绝，保证投递方拿到回执而非空等。
func (s *system) drainRunInbox(record *runRecord) bool {
	if record == nil || record.inbox == nil {
		return false
	}
	stop := false
	for {
		select {
		case event := <-record.inbox:
			switch event.kind {
			case runEventStop:
				stop = true
			case runEventToolConfirmation:
				if event.confirmation != nil {
					finishToolConfirmationRequest(*event.confirmation, runtimeStateInvalid("run is not waiting for confirmation", nil))
				}
			}
		default:
			return stop
		}
	}
}
