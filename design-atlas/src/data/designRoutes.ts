export type RouteBlock =
  | {
      kind: "text";
      title?: string;
      paragraphs: string[];
    }
  | {
      kind: "cards";
      title?: string;
      items: Array<{ title: string; body: string; meta?: string; accent?: string }>;
    }
  | {
      kind: "table";
      title?: string;
      columns: string[];
      rows: string[][];
    }
  | {
      kind: "code";
      title?: string;
      code: string;
    }
  | {
      kind: "mermaid";
      title?: string;
      chart: string;
    }
  | {
      kind: "timeline";
      title?: string;
      items: Array<{ label: string; title: string; body: string }>;
    }
  | {
      kind: "visual";
      title?: string;
      visual: "architecture" | "features" | "subsystems" | "mvp" | "decisions" | "roadmap";
    };

export type DesignRoute = {
  id: string;
  group: string;
  label: string;
  eyebrow: string;
  title: string;
  summary: string;
  blocks: RouteBlock[];
};

export const routeGroups = [
  { id: "foundation", label: "基础共识" },
  { id: "runtime", label: "运行时核心" },
  { id: "data", label: "数据与同步" },
  { id: "identity", label: "身份与能力" },
  { id: "delivery", label: "落地路线" },
];

export const designRoutes: DesignRoute[] = [
  {
    id: "overview",
    group: "foundation",
    label: "项目总览",
    eyebrow: "Positioning",
    title: "eucli-box 是多实例 AI agent 服务器",
    summary: "它内部封装完整 agent runtime，对外暴露统一协议，接受自定义客户端调用；它不是 CLI，也不是最终前端应用。",
    blocks: [
      {
        kind: "text",
        title: "定位共识",
        paragraphs: [
          "eucli-box 是一个可多实例部署的 AI agent 服务器。它负责会话、模型、工具、权限、占位符、供应商和持久化协作。",
          "自定义客户端只负责展示和交互，像显示器与遥控器；所有 agent 逻辑都留在 eucli-box 实例里。",
        ],
      },
      {
        kind: "visual",
        visual: "architecture",
      },
      {
        kind: "mermaid",
        title: "Mermaid 总览图",
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
        kind: "cards",
        title: "四个设计原则",
        items: [
          { title: "职责分明", body: "客户端、实例、数据中心各自负责自己的边界，避免业务逻辑散落。", accent: "#7c3aed" },
          { title: "数据驱动", body: "供应商、工具、权限、Agent 身份、占位符都进入结构化数据管理。", accent: "#0891b2" },
          { title: "协议清晰", body: "客户端协议、工具触发协议、工具执行协议分开定义，不混成一团。", accent: "#059669" },
          { title: "先闭环后扩展", body: "第一版先跑通 agent 主链路，再推进多实例同步、记忆和高级插件。", accent: "#d97706" },
        ],
      },
    ],
  },
  {
    id: "architecture",
    group: "foundation",
    label: "三层架构",
    eyebrow: "Architecture",
    title: "客户端、运行时实例、数据中心三层分离",
    summary: "客户端负责看和点，实例负责跑 agent，数据中心负责存储与同步。",
    blocks: [
      { kind: "visual", visual: "architecture" },
      {
        kind: "mermaid",
        title: "三层架构流向",
        chart: `flowchart TB
  subgraph C[自定义客户端]
    C1[发起会话]
    C2[展示聊天 / 工具 / 状态]
    C3[批准权限]
    C4[管理配置界面]
  end

  subgraph R[eucli-box 实例]
    R1[会话管理]
    R2[Agent 循环]
    R3[工具执行]
    R4[权限控制]
    R5[占位符替换]
  end

  subgraph D[数据存储中心]
    D1[会话数据]
    D2[聊天记录]
    D3[供应商配置]
    D4[工具 / 权限 / Agent]
  end

  C -->|自有协议| R
  R -->|连接 / 同步| D`,
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
      {
        kind: "cards",
        title: "部署模式",
        items: [
          { title: "共享数据库", meta: "模式 A", body: "所有实例访问同一存储，适合内网或同环境部署，数据天然一致。", accent: "#2563eb" },
          { title: "各自有库 + 自动同步", meta: "模式 B", body: "实例分布在隔离机器或网络中，各自保留本地库，并通过中心权威同步差异。", accent: "#7c3aed" },
        ],
      },
    ],
  },
  {
    id: "features",
    group: "foundation",
    label: "Feature 清单",
    eyebrow: "Feature Inventory",
    title: "按 P0/P1/P2 收口功能范围",
    summary: "P0 保证第一版主链路能跑，P1 做结构化增强，P2 留给未来演进。",
    blocks: [
      { kind: "visual", visual: "features" },
      { kind: "visual", visual: "subsystems" },
    ],
  },
  {
    id: "access-client",
    group: "foundation",
    label: "访问与客户端",
    eyebrow: "Access Mode",
    title: "HTTP 服务器模式 + 传参调用模式，共用统一核心",
    summary: "访问入口可以不同，但最终都进入同一个 agent runtime。客户端只做展示和交互。",
    blocks: [
      {
        kind: "table",
        title: "访问模式",
        columns: ["项目", "说明"],
        rows: [
          ["不是 CLI 工具", "不追求命令行交互体验"],
          ["HTTP 服务器模式", "像 VCPToolBox 一样接收外部 API 请求"],
          ["传参调用模式", "像 opencode run 一样直接传任务参数"],
          ["统一核心", "两种入口走同一个 agent runtime 核心，只是触发方式不同"],
          ["自有协议", "暴露 eucli-box 自有协议，由自定义客户端调用"],
        ],
      },
      {
        kind: "table",
        title: "客户端职责划分",
        columns: ["在 eucli-box 实例里", "在自定义客户端里"],
        rows: [
          ["供应商管理", "展示供应商状态"],
          ["agent 循环", "展示 AI 输出"],
          ["工具执行", "展示工具调用进度"],
          ["提示词占位符替换", "展示替换结果"],
          ["权限决策引擎", "展示权限请求、收集用户选择"],
          ["会话持久化", "会话列表、历史浏览"],
          ["数据存储", "数据查询、配置管理界面"],
        ],
      },
    ],
  },
  {
    id: "tool-runtime",
    group: "runtime",
    label: "工具运行",
    eyebrow: "Tool Runtime",
    title: "第一版统一子进程工具执行",
    summary: "工具身份和工具运行配置分离，工具通过 stdin/stdout JSON 与实例通信。",
    blocks: [
      {
        kind: "text",
        title: "执行模式",
        paragraphs: [
          "第一版统一采用独立子进程执行，与 VCPToolBox 一致。工具崩溃不会影响 eucli-box 实例，工具本身也可以用任意语言实现。",
          "同进程工具作为后续演进方向保留，第一版不做。这样能先把工具生态的接口收稳。",
        ],
      },
      {
        kind: "code",
        title: "stdin/stdout 协议",
        code: `eucli-box 实例\n  │ spawn(command, args)\n  │ → stdin:  {"expression": "integral(x*sin(x**2))"}\n  │ ← stdout: {"ok": true, "result": "0.5"}\n  │ ← stdout: {"ok": false, "error": "division by zero"}`,
      },
      {
        kind: "table",
        title: "工具身份 vs 工具配置",
        columns: ["位置", "内容", "性质"],
        rows: [
          ["manifest（工具自带）", "name、description、type、input_schema", "工具身份证，随工具源码存在"],
          ["数据中心配置", "command、args、timeout、working_dir、env", "工具配置单，可独立管理和调整"],
        ],
      },
    ],
  },
  {
    id: "tool-protocol",
    group: "runtime",
    label: "工具触发协议",
    eyebrow: "VCP Protocol",
    title: "采用 VCP 自定义文本协议触发工具",
    summary: "不依赖模型原生 function calling / tool_use，只要模型能输出文本，就能表达工具调用意图。",
    blocks: [
      {
        kind: "code",
        title: "协议形态",
        code: `<<<[TOOL_REQUEST]>>>\ntool_name:「始」工具名「末」\n参数1:「始」值1「末」\n参数2:「始」值2「末」\n<<<[END_TOOL_REQUEST]>>>`,
      },
      {
        kind: "cards",
        title: "选择理由",
        items: [
          { title: "不挑模型", body: "只要能输出文本的模型就能调工具，本地模型和小众模型也能接入。", accent: "#7c3aed" },
          { title: "并行工具", body: "一条回复里可以写多个 TOOL_REQUEST，实例可并行执行。", accent: "#0891b2" },
          { title: "容错强", body: "参数名大小写不敏感、分隔符宽容，AI 不用死磕格式精确。", accent: "#059669" },
          { title: "不受供应商锁死", body: "不依赖 Anthropic/OpenAI 的 function calling 实现。", accent: "#d97706" },
        ],
      },
      {
        kind: "table",
        title: "需要自行实现的部分",
        columns: ["模块", "职责"],
        rows: [
          ["系统提示词模板", "教 AI 使用 TOOL_REQUEST 格式"],
          ["文本协议解析器", "扫描 AI 输出，提取工具名和参数"],
          ["格式容错", "AI 写错格式时修正、提示或回填错误"],
          ["工具说明注入", "每次请求前把可用工具列表及用法拼入提示词"],
        ],
      },
    ],
  },
  {
    id: "own-protocol",
    group: "runtime",
    label: "自有协议",
    eyebrow: "REST + WebSocket",
    title: "同步命令走 REST，实时事件走 WebSocket",
    summary: "REST 适合配置、命令和查询；WebSocket 适合 AI 流式输出、工具进度和权限请求。",
    blocks: [
      {
        kind: "code",
        title: "REST 接口草案",
        code: `POST   /session\nPOST   /session/:id/prompt\nPOST   /session/:id/stop\nPOST   /permission/reply\nGET    /session/:id/history\nGET    /provider\nPUT    /provider/:id\nGET    /tool\nPOST   /tool\nGET    /agent\nPUT    /agent/:id`,
      },
      {
        kind: "code",
        title: "WebSocket 事件",
        code: `text_delta\ntool_start\ntool_progress\ntool_result\ntool_error\npermission_asked\nsession_status\nstep_start\nstep_finish\nerror`,
      },
      {
        kind: "cards",
        title: "选择理由",
        items: [
          { title: "各司其职", body: "REST 做问答式操作，WebSocket 做持续推送。", accent: "#2563eb" },
          { title: "调试方便", body: "列会话、查工具这类操作用 curl 就能试。", accent: "#0891b2" },
          { title: "负载分离", body: "大量事件流不占用 REST 命令通道。", accent: "#059669" },
          { title: "可扩展", body: "后续加配置接口不需要在 WebSocket 协议里开洞。", accent: "#7c3aed" },
        ],
      },
    ],
  },
  {
    id: "sync",
    group: "data",
    label: "数据同步",
    eyebrow: "Hub & Spoke",
    title: "中心权威 + 本地副本 + 分频道拉模式同步",
    summary: "业务实例本地读写，数据中心作为权威源；不同类别数据走不同同步频道。",
    blocks: [
      {
        kind: "code",
        title: "同步架构",
        code: `权威数据中心（只存数据 + 做同步，不跑业务）\n    ↕ 上传改动 / 下载改动\n实例本地库（业务实例的工作副本）`,
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
          ["chat", "聊天消息、消息 parts", "高频追加、流式", "有序事件流、按 seq 追尾"],
          ["session", "会话状态、标题、摘要", "低频状态变更", "事件 + 当前值快照"],
          ["prompt", "提示词模板、占位符定义", "低频、文本型", "版本化存储"],
          ["tool_config", "工具注册、参数定义、执行类型", "低频、结构化", "版本化存储"],
          ["provider", "供应商配置、模型列表", "极低频", "版本化存储"],
          ["permission", "权限规则", "低频", "版本化存储"],
          ["agent", "agent 身份、人设、记忆引用", "极低频", "版本化存储"],
        ],
      },
      {
        kind: "code",
        title: "乐观锁冲突处理",
        code: `配置更新携带当前版本号\n中心检查版本是否匹配\n  匹配 → 写入，版本号 +1\n  不匹配 → 拒绝，返回当前最新版本\n实例拉最新 → 在最新基础上重新修改`,
      },
      {
        kind: "table",
        title: "推荐选型",
        columns: ["组件", "推荐", "原因"],
        rows: [
          ["数据中心存储", "PostgreSQL", "成熟、支持有序查询、事务和乐观锁"],
          ["实例本地库", "SQLite", "零配置、嵌入式、单文件，适合本地部署"],
          ["同步 HTTP API", "自建 RESTful", "简单直接，不引入额外中间件"],
        ],
      },
    ],
  },
  {
    id: "permissions",
    group: "identity",
    label: "权限模型",
    eyebrow: "Permission",
    title: "allow / deny / ask 三层控制 + 会话模式",
    summary: "模式设安全底线，规则表做精确控制，Agent 级工具清单进一步收窄可用能力。",
    blocks: [
      {
        kind: "table",
        title: "三层控制",
        columns: ["指令", "含义", "效果"],
        rows: [
          ["allow", "直接放行", "工具调用不需确认，立即执行"],
          ["deny", "直接拒绝", "工具调用被拦截，不会执行"],
          ["ask", "询问用户", "弹出权限请求，等待 approve / reject"],
        ],
      },
      {
        kind: "table",
        title: "会话模式",
        columns: ["模式", "效果"],
        rows: [
          ["read-only", "全局只读，任何写文件和执行命令都 deny，只能读、搜、fetch"],
          ["workspace-write", "可读写文件，但不能执行 shell"],
          ["danger-full-access", "什么都能干，当前文档设为默认"],
        ],
      },
      {
        kind: "code",
        title: "权限交互流",
        code: `AI 请求执行工具 → 查 permission 频道规则\n  → allow → 直接执行\n  → deny  → 返回拒绝原因给 AI\n  → ask   → WebSocket 发 permission_asked\n           → 客户端回复 approve/reject\n           → approve 继续执行 / reject 返回拒绝原因`,
      },
      {
        kind: "mermaid",
        title: "权限决策流",
        chart: `flowchart TD
  A[AI 请求执行工具] --> B[读取 permission 频道规则]
  B --> C{匹配结果}
  C -->|allow| D[直接执行工具]
  C -->|deny| E[拒绝执行并返回原因]
  C -->|ask| F[暂停会话并推送 permission_asked]
  F --> G{客户端回复}
  G -->|approve| D
  G -->|reject| E
  D --> H[工具结果回填给 AI]
  E --> I[拒绝原因回填给 AI]`,
      },
    ],
  },
  {
    id: "placeholders",
    group: "identity",
    label: "占位符系统",
    eyebrow: "Prompt Slots",
    title: "注册制、递归解析、防循环、工具与插件链接",
    summary: "占位符定义和数据在数据中心，替换逻辑在 eucli-box 实例中执行。",
    blocks: [
      {
        kind: "cards",
        title: "四个子结构",
        items: [
          { title: "递归解析", body: "替换结果里如果还有 {{...}}，继续递归解析。必须检测循环引用。", accent: "#7c3aed" },
          { title: "占位符注册", body: "用户手动命名并写入内容，系统检查冲突后注册生效。", accent: "#0891b2" },
          { title: "工具链接", body: "工具可以把说明或动态信息注册成占位符，供系统提示词引用。", accent: "#059669" },
          { title: "系统插件占位符", body: "时间、天气、RAG、表情包等后台插件提供动态值。", accent: "#d97706" },
        ],
      },
      {
        kind: "timeline",
        title: "注册流程",
        items: [
          { label: "01", title: "命名", body: "用户给占位符取名，例如 {{grep_manual}}。" },
          { label: "02", title: "写入", body: "输入固定文本，或选择工具/插件提供的动态能力。" },
          { label: "03", title: "检查", body: "系统检查同名冲突和循环引用风险。" },
          { label: "04", title: "生效", body: "系统提示词使用该占位符时，实例在请求前解析替换。" },
        ],
      },
    ],
  },
  {
    id: "agents",
    group: "identity",
    label: "Agent 身份",
    eyebrow: "Agent Identity",
    title: "Agent = 占位符人设 + 元数据 + 权限清单",
    summary: "会话按 Agent 分隔，发起聊天必须选择 Agent 角色。",
    blocks: [
      {
        kind: "cards",
        title: "五个核心",
        items: [
          { title: "会话存储", body: "不同 Agent 的会话独立存放，不混在一起。", accent: "#2563eb" },
          { title: "系统提示词", body: "Agent 人设通过占位符系统注入。", accent: "#7c3aed" },
          { title: "名称", body: "客户端展示用的 Agent 显示名称。", accent: "#0891b2" },
          { title: "头像", body: "Agent 的视觉标识。", accent: "#be185d" },
          { title: "权限清单", body: "定义该 Agent 可访问哪些工具，哪些被禁用。", accent: "#dc2626" },
        ],
      },
      {
        kind: "table",
        title: "Agent 元数据",
        columns: ["属性", "说明"],
        rows: [
          ["名字", "Agent 的显示名称，客户端展示用"],
          ["头像", "Agent 的视觉标识"],
          ["描述", "Agent 的简介，客户端选择列表展示用"],
          ["可访问工具清单", "哪些工具对这个 Agent 开放，哪些被禁用"],
        ],
      },
    ],
  },
  {
    id: "providers",
    group: "identity",
    label: "供应商系统",
    eyebrow: "Provider",
    title: "密钥即数据，供应商通过配置文件扩展",
    summary: "单租户部署，默认提供 anthropic 与 openai-compat 两种经典协议配置。",
    blocks: [
      {
        kind: "text",
        title: "密钥与租户",
        paragraphs: [
          "API Key 和其他数据一样走统一数据同步机制，在 provider 频道中以加密形态存储。加密密钥由实例本地持有，数据中心即使被攻破也无法解密使用。",
          "一个 eucli-box 部署服务一个用户或组织。供应商密钥全局共享，不做多租户隔离。",
        ],
      },
      {
        kind: "table",
        title: "内置协议配置",
        columns: ["内置协议配置", "适用供应商"],
        rows: [
          ["anthropic", "Anthropic Claude 系列"],
          ["openai-compat", "OpenAI / xAI / Groq / DeepInfra / DashScope / OpenRouter 等兼容端点"],
        ],
      },
      {
        kind: "code",
        title: "模型组示例",
        code: `模型组: "fast-group" = {\n  成员: [anthropic/haiku, openai-compat/gpt-5-mini, openai-compat/grok-mini]\n  策略: weighted\n  权重: [50, 30, 20]\n  熔断: 连续失败3次踢出，60秒冷却\n}`,
      },
    ],
  },
  {
    id: "memory",
    group: "identity",
    label: "记忆系统",
    eyebrow: "Memory",
    title: "VCP 提取版与自研生产版双轨推进",
    summary: "先快速获得可用记忆能力，再设计更稳妥可靠的长期生产级记忆体系。",
    blocks: [
      {
        kind: "cards",
        title: "两条路线",
        items: [
          { title: "VCP 记忆提取版", meta: "路线一", body: "从 VCPToolBox 源码提取日记系统、TagMemo、RAGDiaryPlugin、DreamWave 等模块，作为系统插件运行。", accent: "#7c3aed" },
          { title: "自研生产级记忆", meta: "路线二", body: "另行设计一套更加稳妥可靠的记忆体系，面向长期稳定运行的生产环境。", accent: "#059669" },
        ],
      },
    ],
  },
  {
    id: "references",
    group: "delivery",
    label: "参考项目",
    eyebrow: "References",
    title: "四个参考项目的借鉴关系",
    summary: "opencode、claw-code、VCPToolBox、paseo 分别提供 agent 循环、工具协议、执行模型和远程连接形态参考。",
    blocks: [
      {
        kind: "table",
        title: "借鉴关系",
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
  },
  {
    id: "decisions",
    group: "delivery",
    label: "设计决策",
    eyebrow: "Decisions",
    title: "十三项设计决策集中收口",
    summary: "把已经确认的关键选择集中展示，避免后续讨论反复摇摆基础方向。",
    blocks: [{ kind: "visual", visual: "decisions" }],
  },
  {
    id: "roadmap",
    group: "delivery",
    label: "路线图",
    eyebrow: "Roadmap",
    title: "从 MVP 闭环到 Production 稳定化",
    summary: "先让主链路活起来，再数据化，再多实例同步，最后进入生产安全与稳定性建设。",
    blocks: [
      { kind: "visual", visual: "roadmap" },
      {
        kind: "mermaid",
        title: "MVP Agent 主链路",
        chart: `flowchart LR
  A[用户发消息] --> B[eucli-box 创建 step]
  B --> C[调用模型]
  C --> D{模型是否请求工具}
  D -->|否| E[推送 text_delta]
  D -->|是| F[解析 VCP TOOL_REQUEST]
  F --> G[权限判断]
  G --> H[执行子进程工具]
  H --> I[工具结果回填模型]
  I --> C
  E --> J[保存会话记录]`,
      },
      { kind: "visual", visual: "mvp" },
    ],
  },
  {
    id: "open-questions",
    group: "delivery",
    label: "待决问题",
    eyebrow: "Open Questions",
    title: "下一阶段需要收口的实现问题",
    summary: "当前概念层已经充分，下一步要进入 MVP 范围、协议字段、数据模型和验收标准。",
    blocks: [
      {
        kind: "cards",
        items: [
          { title: "MVP 边界", body: "第一版必须做、预留接口、后续增强需要明确切分。", accent: "#e11d48" },
          { title: "同步口径", body: "共享数据库与中心权威本地副本的关系需要在实现稿中收紧。", accent: "#d97706" },
          { title: "事件可靠性", body: "WebSocket 是否需要事件 ID、顺序保证、重连补发仍需确定。", accent: "#2563eb" },
          { title: "密钥分发", body: "多实例下加密密钥如何安全持有和轮换，需要专门设计。", accent: "#7c3aed" },
          { title: "工具 manifest", body: "工具身份证字段格式需要正式定义。", accent: "#0891b2" },
          { title: "记忆是否进主线", body: "记忆系统第一版是进入主链路，还是先作为插件预留。", accent: "#059669" },
        ],
      },
    ],
  },
];

export const defaultRouteId = designRoutes[0].id;
