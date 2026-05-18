import type { DesignRoute } from "./types";

export const featuresRoute: DesignRoute = {
  id: "features",
  label: "功能列表",
  eyebrow: "Feature Tree",
  title: "按系统树组织功能范围",
  summary: "功能先按领域和模块分层组织，再给具体条目标 P0/P1/P2，后续实现时按树逐项核对。",
  blocks: [
    { kind: "visual", visual: "feature-tree" },
    {
      kind: "cards",
      title: "核对方式",
      items: [
        { title: "先看归属", body: "先确认功能属于哪个系统领域和模块，而不是先按优先级打散。", accent: "#7c3aed" },
        { title: "再看优先级", body: "P0/P1/P2 只作为 feature 标签，用来判断实现顺序和验收强度。", accent: "#0891b2" },
        { title: "最后看验收口径", body: "每个叶子节点都必须能说清楚做到什么样才算完成。", accent: "#059669" },
        { title: "持续对照", body: "实现路线、待解决问题和更改日志都应该能反向关联到功能树。", accent: "#d97706" },
      ],
    },
  ],
};
