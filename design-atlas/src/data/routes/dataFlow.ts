import type { DesignRoute } from "./types";

export const dataFlowRoute: DesignRoute = {
  id: "data-flow",
  label: "数据流向图",
  eyebrow: "Data Flow",
  title: "命令、事件、工具结果和同步数据各走各的通道",
  summary: "REST 负责命令和查询，WebSocket 负责实时事件，工具通过 stdin/stdout JSON 通信，实例与中心通过分频道增量同步。",
  blocks: [
    {
      kind: "mermaid",
      title: "一次 Agent 任务的数据流",
      chart: `sequenceDiagram
  participant User as 用户
  participant Client as 自定义客户端
  participant Runtime as eucli-box 实例
  participant LLM as 模型供应商
  participant Tool as 工具子进程
  participant Store as 数据中心

  User->>Client: 输入任务
  Client->>Runtime: POST /session/:id/prompt
  Runtime->>Store: 保存 user message
  Runtime->>LLM: 调用模型
  LLM-->>Runtime: text_delta / TOOL_REQUEST
  Runtime-->>Client: WebSocket text_delta
  Runtime->>Runtime: 解析工具请求 + 权限判断
  Runtime->>Tool: stdin JSON
  Tool-->>Runtime: stdout JSON
  Runtime->>LLM: 工具结果回填
  Runtime->>Store: 保存 assistant/tool 消息
  Runtime-->>Client: session_status`,
    },
    {
      kind: "mermaid",
      title: "中心权威 + 本地副本同步流",
      chart: `sequenceDiagram
  participant Local as 实例本地库
  participant Runtime as eucli-box 实例
  participant Hub as 权威数据中心

  Runtime->>Local: 运行时读写本地数据
  Runtime->>Hub: 上传本地变更事件
  Hub-->>Runtime: 接收并分配 seq
  Runtime->>Hub: GET /sync/{channel}?after={seq}
  Hub-->>Runtime: 返回增量事件列表
  Runtime->>Local: 应用增量到本地副本`,
    },
    {
      kind: "table",
      title: "分频道同步",
      columns: ["频道", "内容", "特征", "同步策略"],
      rows: [
        ["chat", "聊天消息、消息 parts、工具结果", "高频追加、流式", "有序事件流、按 seq 追尾"],
        ["session", "会话状态、标题、摘要", "低频状态变更", "事件 + 当前值快照"],
        ["prompt", "提示词模板、占位符定义", "低频、文本型", "版本化存储"],
        ["tool_config", "工具注册、参数定义、执行类型", "低频、结构化", "版本化存储"],
        ["provider", "供应商配置、模型列表", "极低频", "版本化存储 + 密文存储"],
        ["permission", "权限规则", "低频", "版本化存储"],
        ["agent", "agent 身份、人设、记忆引用", "极低频", "版本化存储"],
      ],
    },
    {
      kind: "code",
      title: "乐观锁冲突处理",
      code: `配置更新携带当前版本号
中心检查版本是否匹配
  匹配 → 写入，版本号 +1
  不匹配 → 拒绝，返回当前最新版本
实例拉最新 → 在最新基础上重新修改`,
    },
  ],
};
