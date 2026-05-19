import type { DesignRoute } from "./types";

export const featuresRoute: DesignRoute = {
  id: "features",
  label: "原子功能列表",
  eyebrow: "Atomic Feature Catalog",
  title: "全部原子功能都是一级对象",
  summary: "原子功能直接平铺展示；领域、模块、阶段、依赖、架构关系和数据流只是功能自身的信息，不再作为上层分类压住功能。",
  blocks: [
    { kind: "visual", visual: "atomic-feature-list" },
  ],
};
