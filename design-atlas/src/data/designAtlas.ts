export type Decision = {
  index: number;
  title: string;
  conclusion: string;
};

export type Risk = {
  title: string;
  detail: string;
};

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
