import type { DesignRoute } from "./types";

export const featuresRoute: DesignRoute = {
  id: "features",
  label: "功能列表",
  eyebrow: "Feature Tree",
  title: "按系统树组织功能范围",
  summary: "功能先按领域和模块分层组织，再给具体条目标 P0/P1/P2，后续实现时按树逐项核对。",
  blocks: [
    { kind: "visual", visual: "feature-tree", density: "compact" },
  ],
};
