import type { DesignRoute } from "./types";

export const architectureRoute: DesignRoute = {
  id: "architecture",
  label: "架构图",
  eyebrow: "Architecture",
  title: "客户端、运行时实例、数据中心三层分离",
  summary: "客户端负责看和点，实例负责跑 agent，数据中心负责存储与同步；工具、供应商和插件作为运行时外部能力接入。",
  blocks: [
    {
      kind: "text",
      title: "定位共识",
      paragraphs: [
        "eucli-box 是一个可多实例部署的 AI agent 服务器。它负责会话、模型、工具、权限、占位符、供应商和持久化协作。",
        "自定义客户端只负责展示和交互，像显示器与遥控器；所有 agent 逻辑都留在 eucli-box 实例里。",
      ],
    },
    { kind: "visual", visual: "architecture" },
    {
      kind: "mermaid",
      title: "系统总览",
      chart: `flowchart TD
  Client[自定义客户端<br/>显示器 + 遥控器]
  Runtime[eucli-box 实例<br/>Agent Runtime 核心]
  Store[数据存储中心<br/>权威数据源]
  Tools[工具子进程<br/>隔离执行单元]
  Providers[模型供应商<br/>LLM 能力来源]
  Plugins[系统插件<br/>后台动态能力]

  Client -->|REST + WebSocket| Runtime
  Runtime -->|Pull Sync| Store
  Runtime -->|spawn + stdin/stdout JSON| Tools
  Runtime -->|LLM API| Providers
  Runtime -->|dynamic slots| Plugins`,
    },
    {
      kind: "table",
      title: "三层职责",
      columns: ["层级", "职责", "不做什么"],
      rows: [
        ["自定义客户端", "发起会话、展示聊天/工具/状态、批准权限、管理配置界面", "不做 agent 循环，不执行工具"],
        ["eucli-box 实例", "会话管理、供应商管理、agent 循环、工具执行、权限控制、占位符替换", "不承担最终 UI 产品职责"],
        ["数据存储中心", "会话、聊天记录、供应商配置、工具配置、Agent 身份、记忆等持久化数据", "不跑模型，不执行工具，不做业务循环"],
      ],
    },
    { kind: "visual", title: "子系统协作边界", visual: "subsystems" },
  ],
};
