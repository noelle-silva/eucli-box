import type { DesignRoute } from "./types";

export const architectureRoute: DesignRoute = {
  id: "architecture",
  label: "架构图",
  eyebrow: "Architecture",
  title: "客户端、运行时实例、数据中心三层分离",
  summary: "客户端负责看和点，实例负责跑 agent，数据中心负责存储与同步；工具、供应商和插件作为运行时外部能力接入。",
  blocks: [
    { kind: "visual", visual: "architecture" },
  ],
};
