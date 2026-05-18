import type { DesignRoute } from "./types";

export const stateMachinesRoute: DesignRoute = {
  id: "state-machines",
  label: "状态机逻辑",
  eyebrow: "State Machine",
  title: "会话、工具、权限和同步必须有明确状态出口",
  summary: "状态机页用于约束运行时行为：每个状态由什么事件进入、由什么事件离开、失败时如何暴露真实错误。",
  blocks: [
    {
      kind: "mermaid",
      title: "会话状态机",
      chart: `stateDiagram-v2
  [*] --> idle
  idle --> running: prompt_submitted
  running --> waiting_permission: permission_asked
  waiting_permission --> running: approved
  waiting_permission --> running: rejected_with_reason
  running --> stopping: stop_requested
  stopping --> stopped: runtime_stopped
  running --> completed: final_answer_saved
  running --> failed: runtime_error
  completed --> idle: next_prompt
  stopped --> idle: next_prompt
  failed --> idle: recover_or_retry`,
    },
    {
      kind: "mermaid",
      title: "工具调用状态机",
      chart: `stateDiagram-v2
  [*] --> parsed
  parsed --> permission_checking
  permission_checking --> blocked: deny
  permission_checking --> waiting_user: ask
  permission_checking --> spawning: allow
  waiting_user --> spawning: approve
  waiting_user --> blocked: reject
  spawning --> running
  running --> succeeded: stdout_ok
  running --> failed: timeout_or_exit_error
  succeeded --> result_filled
  failed --> error_filled
  blocked --> error_filled`,
    },
    {
      kind: "table",
      title: "状态机原则",
      columns: ["原则", "说明"],
      rows: [
        ["所有阻塞都要可见", "等待权限、等待工具、等待模型都必须能通过事件展示给客户端。"],
        ["所有失败都要有出口", "模型失败、工具失败、权限拒绝、同步冲突不能静默停住。"],
        ["状态不靠 UI 猜", "客户端只展示 runtime 推送的状态，不自行推断核心状态。"],
        ["错误不兜底掩盖", "真实错误进入 error 或 tool_error 事件，方便定位根因。"],
      ],
    },
  ],
};
