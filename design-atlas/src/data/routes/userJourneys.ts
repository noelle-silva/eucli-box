import type { DesignRoute } from "./types";

export const userJourneysRoute: DesignRoute = {
  id: "user-journeys",
  label: "用户故事路径",
  eyebrow: "User Journey",
  title: "从用户目标到 Agent 完成任务的可验收路径",
  summary: "用户故事路径把功能树中的叶子节点串成真实使用场景，用来检查体验、协议、权限和数据是否闭环。",
  blocks: [
    {
      kind: "timeline",
      title: "MVP 主故事：带工具调用的对话",
      items: [
        { label: "01", title: "选择 Agent", body: "用户选择一个 Agent，客户端创建或恢复该 Agent 下的会话。" },
        { label: "02", title: "发送任务", body: "用户输入任务，客户端通过 REST 把 prompt 发送给 eucli-box 实例。" },
        { label: "03", title: "模型推理", body: "实例调用模型，并通过 WebSocket 把 text_delta 和 step 事件推给客户端。" },
        { label: "04", title: "请求工具", body: "模型输出 VCP TOOL_REQUEST，实例解析工具名和参数。" },
        { label: "05", title: "权限判断", body: "实例按会话模式、权限规则和 Agent 工具白名单判断 allow / deny / ask。" },
        { label: "06", title: "执行并回填", body: "工具子进程返回结果，实例回填模型继续推理，最终保存完整会话记录。" },
      ],
    },
    {
      kind: "mermaid",
      title: "MVP 用户故事路径",
      chart: `flowchart LR
  A[选择 Agent] --> B[创建/恢复会话]
  B --> C[发送任务]
  C --> D[模型推理]
  D --> E{是否请求工具}
  E -->|否| F[流式返回回答]
  E -->|是| G[解析 TOOL_REQUEST]
  G --> H{权限结果}
  H -->|allow| I[执行工具]
  H -->|ask| J[客户端批准/拒绝]
  H -->|deny| K[拒绝原因回填]
  J -->|approve| I
  J -->|reject| K
  I --> L[工具结果回填]
  L --> D
  F --> M[保存会话记录]`,
    },
    { kind: "visual", title: "实现链路视觉版", visual: "mvp" },
  ],
};
