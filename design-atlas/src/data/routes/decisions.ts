import type { DesignRoute } from "./types";

export const decisionsRoute: DesignRoute = {
  id: "decisions",
  label: "设计决策",
  eyebrow: "Decisions",
  title: "设计决策集中收口，避免基础方向反复摇摆",
  summary: "把已经确认的关键选择、参考项目和取舍关系集中展示，作为后续实现时的边界依据。",
  blocks: [
    { kind: "visual", visual: "decisions" },
    {
      kind: "table",
      title: "参考项目借鉴关系",
      columns: ["概念", "主要参考", "借鉴什么"],
      rows: [
        ["agent 循环", "opencode / claw-code", "流式 LLM 调用 → 扫描工具调用标记 → 执行 → 结果回填 → 继续循环"],
        ["工具触发协议", "VCPToolBox", "TOOL_REQUEST + key/value 自定义文本协议"],
        ["工具注册与执行", "VCPToolBox", "全子进程、stdin/stdout JSON、manifest + 配置单分离"],
        ["供应商管理", "opencode", "多 provider 适配、模型切换、认证管理"],
        ["session 管理", "opencode / paseo", "session 创建、恢复、归档、event 流"],
        ["权限系统", "opencode / claw-code", "allow/deny/ask + 模式分级"],
        ["自有协议", "paseo", "实例与客户端之间的事件、状态、命令通信协议"],
      ],
    },
  ],
};
