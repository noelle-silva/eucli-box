import type { DesignRoute } from "./types";

export const uiPrototypeRoute: DesignRoute = {
  id: "ui-prototype",
  label: "UI 交互原型",
  eyebrow: "Interaction",
  title: "客户端只做展示、操作和权限确认",
  summary: "UI 原型关注用户看见什么、触发什么命令、收到什么事件；agent 逻辑、权限判断和工具执行都留在实例内。",
  blocks: [
    {
      kind: "mermaid",
      title: "客户端信息布局",
      chart: `flowchart LR
  Shell[客户端 Shell]
  Sessions[会话列表]
  Chat[聊天窗口]
  Inspector[运行检查器]
  Config[配置面板]

  Shell --> Sessions
  Shell --> Chat
  Shell --> Inspector
  Shell --> Config
  Chat --> Composer[输入框 + 发送/停止]
  Inspector --> ToolEvents[工具进度]
  Inspector --> Permission[权限请求]
  Config --> Providers[供应商]
  Config --> Agents[Agent]
  Config --> Tools[工具]
  Config --> Permissions[权限规则]`,
    },
    {
      kind: "table",
      title: "交互契约",
      columns: ["用户动作", "客户端命令", "运行时事件", "UI 反馈"],
      rows: [
        ["创建会话", "POST /session", "session_status", "会话列表出现新会话"],
        ["发送任务", "POST /session/:id/prompt", "text_delta / step_start", "聊天窗口流式显示回答"],
        ["停止任务", "POST /session/:id/stop", "session_status", "输入区恢复可操作"],
        ["批准工具", "POST /permission/reply", "tool_start / tool_result", "权限卡片变为已批准，工具进度继续"],
        ["拒绝工具", "POST /permission/reply", "tool_error", "权限卡片变为已拒绝，AI 收到拒绝原因"],
      ],
    },
    {
      kind: "cards",
      title: "客户端不越界",
      items: [
        { title: "不跑 agent 循环", body: "客户端不决定下一步推理，只展示 runtime 推送的事件。", accent: "#7c3aed" },
        { title: "不执行工具", body: "工具执行必须在 eucli-box 实例内经过权限判断后发起。", accent: "#0891b2" },
        { title: "不吞错误", body: "协议、权限、工具、模型错误都要以真实事件展示。", accent: "#e11d48" },
        { title: "不持久化权威数据", body: "客户端可缓存 UI 状态，但权威数据属于实例和数据中心。", accent: "#059669" },
      ],
    },
  ],
};
