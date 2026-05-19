import type { DesignRoute } from "./types";

export const implementationRoadmapRoute: DesignRoute = {
  id: "implementation-roadmap",
  label: "实现路线",
  eyebrow: "Implementation",
  title: "从 MVP 闭环到 Production 稳定化",
  summary: "实现路线是任务管理入口：先让主链路活起来，再数据化，再多实例同步，最后进入生产安全与稳定性建设。",
  blocks: [
    { kind: "visual", visual: "roadmap" },
  ],
};
