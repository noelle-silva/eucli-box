import type { AtomicFeature, ProjectDomain, ProjectModule } from "./types";
import { validateProjectModel } from "./validate";

export const projectDomains: ProjectDomain[] = [
  {
    id: "entry-and-client-protocol",
    title: "访问入口与客户端协议",
    summary: "让外部客户端能稳定连接 eucli-box，并把同步命令与实时事件拆开。",
    accent: "#2563eb",
  },
  {
    id: "runtime-session-loop",
    title: "运行时与会话主链路",
    summary: "让单实例能够完成对话、推理、工具调用、结果回填和持久化。",
    accent: "#7c3aed",
  },
  {
    id: "tools-and-permissions",
    title: "工具系统与权限控制",
    summary: "用文本协议触发工具，用子进程隔离执行，并在执行前经过明确权限判断。",
    accent: "#0891b2",
  },
  {
    id: "data-and-sync",
    title: "数据中心与多实例同步",
    summary: "把配置和业务数据沉淀为权威数据，并为多实例协作预留可靠同步机制。",
    accent: "#059669",
  },
  {
    id: "identity-provider-prompt",
    title: "身份、供应商与提示词能力",
    summary: "把模型来源、Agent 身份和提示词动态内容变成可管理的数据能力。",
    accent: "#d97706",
  },
  {
    id: "memory-production-readiness",
    title: "记忆与生产化能力",
    summary: "先保留可落地的记忆路线，再把稳定性、安全性和可运维性补齐。",
    accent: "#be185d",
  },
];

export const projectModules: ProjectModule[] = [
  { id: "entry-modes", domainId: "entry-and-client-protocol", title: "访问入口", summary: "不同入口复用同一个 agent runtime，不分裂业务核心。" },
  { id: "client-protocol", domainId: "entry-and-client-protocol", title: "客户端协议", summary: "REST 处理同步命令，WebSocket 处理运行时事件。" },
  { id: "session-state", domainId: "runtime-session-loop", title: "会话与消息状态", summary: "负责会话生命周期、消息保存和历史恢复。" },
  { id: "agent-runtime-loop", domainId: "runtime-session-loop", title: "Agent 执行循环", summary: "负责模型调用、工具回合、step 事件和错误显式暴露。" },
  { id: "tool-trigger-protocol", domainId: "tools-and-permissions", title: "工具触发协议", summary: "模型通过 VCP 文本协议表达工具调用意图。" },
  { id: "tool-registry-runtime", domainId: "tools-and-permissions", title: "工具注册与执行", summary: "工具身份与运行配置分离，第一版统一子进程执行。" },
  { id: "permission-model", domainId: "tools-and-permissions", title: "权限决策模型", summary: "会话模式、权限规则和 Agent 工具清单共同收窄能力边界。" },
  { id: "storage-topology", domainId: "data-and-sync", title: "存储拓扑", summary: "中心权威源、本地副本和共享库模式的边界清晰分离。" },
  { id: "sync-consistency", domainId: "data-and-sync", title: "同步与一致性", summary: "不同数据频道独立同步，配置更新用版本约束防止覆盖。" },
  { id: "provider-system", domainId: "identity-provider-prompt", title: "供应商系统", summary: "模型协议、密钥、模型坐标和模型组的数据化管理。" },
  { id: "prompt-slots", domainId: "identity-provider-prompt", title: "提示词占位符", summary: "注册制、递归解析、防循环，以及工具和插件动态值。" },
  { id: "agent-identity-system", domainId: "identity-provider-prompt", title: "Agent 身份系统", summary: "Agent 作为会话、人设、工具权限和展示元数据的组合实体。" },
  { id: "memory-system", domainId: "memory-production-readiness", title: "记忆系统", summary: "VCP 提取版与自研生产版双轨推进。" },
  { id: "production-hardening", domainId: "memory-production-readiness", title: "生产稳定性", summary: "围绕事件恢复和密钥生命周期补齐生产级能力。" },
];

export const atomicFeatures: AtomicFeature[] = [
  { id: "http-server-mode", title: "HTTP 服务器模式", intent: "实例以 HTTP 服务承载 agent runtime，对外接收客户端请求。", domainId: "entry-and-client-protocol", moduleId: "entry-modes", stage: "mvp", acceptance: "能创建会话、发送消息、停止任务、查询历史，并返回明确错误。" },
  { id: "argument-run-mode", title: "传参调用模式", intent: "支持像 run 命令一样直接传入任务参数，但仍复用同一 runtime 核心。", domainId: "entry-and-client-protocol", moduleId: "entry-modes", stage: "alpha", acceptance: "传参入口和 HTTP 入口进入同一套会话、权限、工具、供应商逻辑。" },
  { id: "rest-command-api", title: "REST 命令接口", intent: "用 REST 处理会话、配置、工具、供应商、Agent 等同步操作。", domainId: "entry-and-client-protocol", moduleId: "client-protocol", stage: "mvp", acceptance: "接口语义清楚，命令请求不混入实时流式事件。", architecture: { relatedModuleIds: ["agent-runtime-loop", "session-state"] }, flow: { inputs: ["command_request"], outputs: ["command_result"], emits: ["structured_error"] } },
  { id: "websocket-event-stream", title: "WebSocket 实时事件流", intent: "用 WebSocket 推送 AI 文本、工具进度、权限请求、状态变化和错误。", domainId: "entry-and-client-protocol", moduleId: "client-protocol", stage: "mvp", acceptance: "客户端能按事件类型展示 text_delta、tool_*、permission_asked、session_status。", dependsOn: ["rest-command-api"], flow: { inputs: ["runtime_event"], outputs: ["client_event"], emits: ["text_delta", "tool_result", "permission_asked", "session_status"] } },
  { id: "custom-client-boundary", title: "自定义客户端边界", intent: "客户端只负责展示、交互和配置管理，不运行 agent 循环，不执行工具。", domainId: "entry-and-client-protocol", moduleId: "client-protocol", stage: "mvp", acceptance: "agent 决策、工具执行、权限判断、占位符替换全部留在 eucli-box 实例内。" },
  { id: "session-lifecycle", title: "会话生命周期", intent: "支持会话创建、恢复、停止、状态维护、历史读取和标题摘要。", domainId: "runtime-session-loop", moduleId: "session-state", stage: "mvp", acceptance: "一次会话从创建到结束的数据和状态都能被查询并复现。", dependsOn: ["rest-command-api"] },
  { id: "message-persistence", title: "聊天记录持久化", intent: "保存用户消息、AI 输出、工具调用、工具结果和错误事件。", domainId: "runtime-session-loop", moduleId: "session-state", stage: "mvp", acceptance: "刷新客户端或重启实例后，历史记录仍能按顺序恢复。", dependsOn: ["session-lifecycle"] },
  { id: "agent-loop", title: "Agent 循环", intent: "模型输出后扫描工具请求，执行工具，回填结果，再继续推理。", domainId: "runtime-session-loop", moduleId: "agent-runtime-loop", stage: "mvp", acceptance: "一次任务内能完成多轮模型调用与工具调用，直到产生最终回答。", dependsOn: ["session-lifecycle"] },
  { id: "step-events", title: "Step 级运行状态", intent: "把模型调用、工具解析、权限判断、工具执行等阶段拆成可观察 step。", domainId: "runtime-session-loop", moduleId: "agent-runtime-loop", stage: "alpha", acceptance: "客户端能展示 step_start、step_finish 和错误所在阶段。", dependsOn: ["websocket-event-stream", "agent-loop"] },
  { id: "runtime-error-surface", title: "真实错误显式暴露", intent: "运行时不静默吞错，不用无意义兜底掩盖真实问题。", domainId: "runtime-session-loop", moduleId: "agent-runtime-loop", stage: "mvp", acceptance: "模型、工具、权限、协议解析失败时，客户端能看到结构化错误原因。", dependsOn: ["websocket-event-stream"] },
  { id: "vcp-tool-request", title: "VCP TOOL_REQUEST 协议", intent: "模型通过自定义文本块声明工具名和参数，不依赖原生 function calling。", domainId: "tools-and-permissions", moduleId: "tool-trigger-protocol", stage: "mvp", acceptance: "能从模型文本中提取多个工具请求，并把解析失败作为真实错误返回。", dependsOn: ["agent-loop"], architecture: { relatedModuleIds: ["agent-runtime-loop", "permission-model"] }, flow: { inputs: ["model_text"], outputs: ["tool_request"], emits: ["tool_parse_error"] } },
  { id: "parallel-tool-requests", title: "并行工具请求", intent: "一条模型回复中可以包含多个 TOOL_REQUEST，并由实例调度执行。", domainId: "tools-and-permissions", moduleId: "tool-trigger-protocol", stage: "alpha", acceptance: "并行执行的工具结果能按请求身份回填，不互相串线。", dependsOn: ["vcp-tool-request"] },
  { id: "tool-manifest", title: "工具 Manifest 身份", intent: "工具自带 name、description、type、input_schema 等身份信息。", domainId: "tools-and-permissions", moduleId: "tool-registry-runtime", stage: "alpha", acceptance: "工具身份与运行配置分离，系统可读取并展示工具说明。" },
  { id: "tool-runtime-config", title: "工具运行配置", intent: "数据中心保存 command、args、timeout、working_dir、env 等运行配置。", domainId: "tools-and-permissions", moduleId: "tool-registry-runtime", stage: "mvp", acceptance: "同一个工具身份可以通过配置调整执行方式，无需改工具源码。", dependsOn: ["tool-manifest"], architecture: { relatedModuleIds: ["permission-model"] }, flow: { inputs: ["tool_request"], outputs: ["tool_process_config"] } },
  { id: "subprocess-tool-runtime", title: "子进程工具执行", intent: "第一版统一 spawn 独立子进程，通过 stdin/stdout JSON 通信。", domainId: "tools-and-permissions", moduleId: "tool-registry-runtime", stage: "mvp", acceptance: "工具崩溃不会拖垮实例，超时、非零退出、JSON 错误都有明确结果。", dependsOn: ["tool-runtime-config", "permission-rules"] },
  { id: "in-process-tools", title: "同进程工具执行", intent: "未来为高信任工具提供同进程执行方式，第一版不做。", domainId: "tools-and-permissions", moduleId: "tool-registry-runtime", stage: "future", acceptance: "同进程工具必须有明确隔离边界和信任模型，不能绕过权限系统。", dependsOn: ["subprocess-tool-runtime"] },
  { id: "permission-rules", title: "allow / deny / ask 权限规则", intent: "工具执行前匹配权限规则，直接放行、拒绝或询问用户。", domainId: "tools-and-permissions", moduleId: "permission-model", stage: "mvp", acceptance: "权限结果能决定工具是否执行，并把拒绝原因或用户选择回填给 agent。", dependsOn: ["vcp-tool-request"], flow: { inputs: ["tool_request"], outputs: ["permission_decision"], emits: ["permission_asked"] } },
  { id: "session-sandbox-mode", title: "会话安全模式", intent: "支持 read-only、workspace-write、danger-full-access 等会话级安全底线。", domainId: "tools-and-permissions", moduleId: "permission-model", stage: "mvp", acceptance: "会话模式先于细粒度规则生效，越权工具请求必须被拒绝。", dependsOn: ["session-lifecycle"] },
  { id: "agent-tool-allowlist", title: "Agent 工具白名单", intent: "每个 Agent 有自己的可访问工具清单，进一步收窄能力边界。", domainId: "tools-and-permissions", moduleId: "permission-model", stage: "alpha", acceptance: "同一工具可以对某些 Agent 可用，对另一些 Agent 不可用。", dependsOn: ["agent-identity", "tool-manifest"] },
  { id: "central-data-store", title: "数据存储中心", intent: "集中保存会话、聊天、供应商、工具、权限、Agent、占位符和记忆数据。", domainId: "data-and-sync", moduleId: "storage-topology", stage: "alpha", acceptance: "核心配置和业务记录不散落在客户端或临时内存里。" },
  { id: "local-runtime-store", title: "实例本地副本", intent: "业务实例读写本地库，并与中心权威源同步差异。", domainId: "data-and-sync", moduleId: "storage-topology", stage: "beta", acceptance: "实例离线或网络波动时，本地仍有可恢复的工作副本。", dependsOn: ["central-data-store"] },
  { id: "shared-db-mode", title: "共享数据库部署模式", intent: "多实例直接访问同一数据库，适合内网或同环境部署。", domainId: "data-and-sync", moduleId: "storage-topology", stage: "future", acceptance: "部署文档清楚标明共享库模式与同步模式的边界。", dependsOn: ["central-data-store"] },
  { id: "channel-sync", title: "分频道增量同步", intent: "chat、session、prompt、tool_config、provider、permission、agent 分频道同步。", domainId: "data-and-sync", moduleId: "sync-consistency", stage: "beta", acceptance: "每个频道有独立 seq，实例能按 after seq 拉取增量。", dependsOn: ["local-runtime-store"] },
  { id: "optimistic-locking", title: "配置乐观锁", intent: "低频配置更新携带版本号，版本不匹配时拒绝写入。", domainId: "data-and-sync", moduleId: "sync-consistency", stage: "beta", acceptance: "冲突不会被静默覆盖，客户端能看到最新版本并重新提交。", dependsOn: ["central-data-store"] },
  { id: "provider-config", title: "供应商配置系统", intent: "支持 anthropic 与 openai-compat，保存 baseUrl、model、API Key 等配置。", domainId: "identity-provider-prompt", moduleId: "provider-system", stage: "mvp", acceptance: "用户能配置模型供应商并被 runtime 正确调用。" },
  { id: "encrypted-provider-secrets", title: "供应商密钥加密存储", intent: "API Key 作为数据进入 provider 频道，并以加密形态存储。", domainId: "identity-provider-prompt", moduleId: "provider-system", stage: "alpha", acceptance: "数据中心拿到密文也不能直接使用密钥，实例本地负责解密。", dependsOn: ["provider-config"] },
  { id: "model-groups", title: "模型组", intent: "多个模型坐标组成逻辑组，支持权重、轮询、主备和熔断策略。", domainId: "identity-provider-prompt", moduleId: "provider-system", stage: "alpha", acceptance: "模型连续失败后能按策略降级或冷却，不阻塞整个会话。", dependsOn: ["provider-config"] },
  { id: "prompt-placeholders", title: "占位符注册与解析", intent: "用户注册 {{slot}}，请求模型前由实例递归解析并替换。", domainId: "identity-provider-prompt", moduleId: "prompt-slots", stage: "alpha", acceptance: "占位符支持嵌套解析，循环引用会被检测并报错。" },
  { id: "tool-placeholder-link", title: "工具链接占位符", intent: "工具可以把说明或动态信息注册为占位符，供系统提示词引用。", domainId: "identity-provider-prompt", moduleId: "prompt-slots", stage: "alpha", acceptance: "工具提供的占位符能进入提示词，来源和更新时间可追踪。", dependsOn: ["prompt-placeholders", "tool-manifest"] },
  { id: "system-plugin-placeholders", title: "系统插件动态占位符", intent: "时间、天气、RAG、表情包等后台插件提供动态占位符值。", domainId: "identity-provider-prompt", moduleId: "prompt-slots", stage: "future", acceptance: "系统插件不直接暴露为 AI 工具，但能提供稳定的动态提示词材料。", dependsOn: ["prompt-placeholders"] },
  { id: "agent-identity", title: "Agent 身份系统", intent: "Agent 拥有名称、头像、描述、系统提示词、会话归属和工具权限。", domainId: "identity-provider-prompt", moduleId: "agent-identity-system", stage: "alpha", acceptance: "发起会话必须选择 Agent，不同 Agent 的会话和权限互不混淆。" },
  { id: "vcp-memory-extraction", title: "VCP 记忆提取版", intent: "提取 TagMemo、RAGDiaryPlugin、DreamWave 等模块作为系统插件。", domainId: "memory-production-readiness", moduleId: "memory-system", stage: "future", acceptance: "记忆能力以插件身份接入，不污染 MVP agent 主链路。" },
  { id: "production-memory", title: "自研生产级记忆", intent: "另行设计长期稳定运行的生产级记忆体系。", domainId: "memory-production-readiness", moduleId: "memory-system", stage: "future", acceptance: "记忆写入、检索、遗忘、权限和可解释性都有明确设计。" },
  { id: "event-recovery", title: "事件可靠性与恢复", intent: "WebSocket 事件增加顺序、重连补发或恢复策略。", domainId: "memory-production-readiness", moduleId: "production-hardening", stage: "future", acceptance: "客户端断线后能恢复关键事件，不误判任务状态。", dependsOn: ["websocket-event-stream"] },
  { id: "key-rotation", title: "密钥持有与轮换", intent: "明确多实例下加密密钥如何安全持有、分发和轮换。", domainId: "memory-production-readiness", moduleId: "production-hardening", stage: "future", acceptance: "密钥轮换不会导致已有 provider 配置不可用或泄露。", dependsOn: ["encrypted-provider-secrets"] },
];

validateProjectModel({ domains: projectDomains, modules: projectModules, features: atomicFeatures });
