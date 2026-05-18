export type Subsystem = {
  name: string;
  role: string;
  status: "core" | "defined" | "planned";
  accent: string;
};

export type Decision = {
  index: number;
  title: string;
  conclusion: string;
};

export type Risk = {
  title: string;
  detail: string;
};

export type RoadmapStage = {
  stage: string;
  title: string;
  outcome: string;
  items: string[];
};

export const subsystems: Subsystem[] = [
  { name: "会话系统", role: "生命周期、消息收发、事件状态", status: "core", accent: "#7c3aed" },
  { name: "自有协议系统", role: "REST + WebSocket 的客户端协作层", status: "core", accent: "#2563eb" },
  { name: "工具调用系统", role: "VCP 解析、权限检查、子进程执行", status: "core", accent: "#0891b2" },
  { name: "供应商系统", role: "模型协议、密钥、模型坐标、模型组", status: "core", accent: "#059669" },
  { name: "权限系统", role: "allow/deny/ask 与会话模式", status: "core", accent: "#dc2626" },
  { name: "占位符系统", role: "递归替换、注册制、插件动态值", status: "defined", accent: "#d97706" },
  { name: "数据同步系统", role: "中心权威、本地副本、seq 增量", status: "defined", accent: "#4f46e5" },
  { name: "Agent 身份系统", role: "人设、元数据、会话归属、工具清单", status: "defined", accent: "#be185d" },
  { name: "系统插件系统", role: "后台动态能力，不直接作为 AI 工具", status: "planned", accent: "#0f766e" },
  { name: "记忆系统", role: "VCP 提取版与自研生产版双轨", status: "planned", accent: "#9333ea" },
];

export const decisions: Decision[] = [
  { index: 1, title: "访问方式", conclusion: "HTTP 服务器 + 传参调用，共用同一 runtime 核心。" },
  { index: 2, title: "数据存储", conclusion: "支持共享库与中心权威同步模式，部署时选择。" },
  { index: 3, title: "工具执行", conclusion: "第一版统一子进程，manifest 与运行配置分离。" },
  { index: 4, title: "客户端协议", conclusion: "暴露 eucli-box 自有协议，本项目不实现最终客户端。" },
  { index: 5, title: "占位符系统", conclusion: "递归解析、注册制、工具链接、系统插件链接。" },
  { index: 6, title: "工具触发", conclusion: "采用 VCP 文本协议，不绑定模型原生工具调用。" },
  { index: 7, title: "协议形态", conclusion: "REST 负责命令配置，WebSocket 负责实时事件。" },
  { index: 8, title: "数据同步", conclusion: "中心权威 + 本地副本，分频道同步，配置用乐观锁。" },
  { index: 9, title: "同步模式", conclusion: "第一版拉模式，实例主动按 seq 拉取增量。" },
  { index: 10, title: "权限模型", conclusion: "allow/deny/ask 叠加 read-only/workspace-write/full-access。" },
  { index: 11, title: "供应商", conclusion: "密钥即数据，单租户，配置式扩展协议与模型组。" },
  { index: 12, title: "Agent 身份", conclusion: "会话存储、系统提示词、名称、头像、权限清单。" },
  { index: 13, title: "记忆系统", conclusion: "VCP 提取版与自研生产版双轨推进。" },
];

export const risks: Risk[] = [
  { title: "MVP 边界", detail: "需要明确第一版必须做、预留接口和后续增强，防止范围失控。" },
  { title: "同步口径", detail: "共享数据库与中心权威本地副本的关系需要在实现稿中收紧。" },
  { title: "事件可靠性", detail: "WebSocket 事件是否需要 ID、顺序保证、重连补发仍需确定。" },
  { title: "密钥分发", detail: "多实例下加密密钥如何安全持有和轮换，需要专门设计。" },
];

export const roadmap: RoadmapStage[] = [
  {
    stage: "01",
    title: "MVP 闭环",
    outcome: "一个实例能完成对话、工具调用和会话保存。",
    items: ["REST 发消息", "WebSocket 推流", "VCP 解析", "子进程工具", "基础权限"],
  },
  {
    stage: "02",
    title: "Alpha 结构化",
    outcome: "配置、Agent、工具和供应商进入数据化管理。",
    items: ["工具 manifest", "供应商配置", "Agent 身份", "占位符注册", "历史浏览"],
  },
  {
    stage: "03",
    title: "Beta 多实例",
    outcome: "中心权威与本地副本同步机制可用。",
    items: ["分频道 seq", "乐观锁", "启动拉取", "配置冲突处理", "同步监控"],
  },
  {
    stage: "04",
    title: "Production 稳定化",
    outcome: "权限、密钥、错误恢复和记忆系统进入生产设计。",
    items: ["密钥安全", "事件恢复", "模型组熔断", "记忆插件", "运维指标"],
  },
];
