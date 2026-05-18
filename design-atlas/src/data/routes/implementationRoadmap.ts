import type { DesignRoute } from "./types";

export const implementationRoadmapRoute: DesignRoute = {
  id: "implementation-roadmap",
  label: "实现路线",
  eyebrow: "Implementation",
  title: "从 MVP 闭环到 Production 稳定化",
  summary: "实现路线是任务管理入口：先让主链路活起来，再数据化，再多实例同步，最后进入生产安全与稳定性建设。",
  blocks: [
    { kind: "visual", visual: "roadmap" },
    {
      kind: "table",
      title: "阶段与功能树关系",
      columns: ["阶段", "目标", "主要绑定功能"],
      rows: [
        ["MVP", "一个实例能完成对话、工具调用和会话保存", "HTTP 服务、REST/WebSocket、会话生命周期、Agent 循环、VCP 工具、子进程工具、基础权限、供应商配置"],
        ["Alpha", "配置、Agent、工具和供应商进入数据化管理", "工具 manifest、Agent 身份、占位符注册、模型组、密钥加密、step 事件"],
        ["Beta", "中心权威与本地副本同步机制可用", "分频道同步、本地副本、乐观锁、配置冲突处理"],
        ["Production", "权限、密钥、错误恢复和记忆系统进入生产设计", "事件恢复、密钥轮换、记忆系统、同进程工具信任模型"],
      ],
    },
  ],
};
