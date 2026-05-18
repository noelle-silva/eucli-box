import type { DesignRoute } from "./types";

export const homeRoute: DesignRoute = {
  id: "home",
  label: "主页",
  layout: "immersive",
  eyebrow: "Atlas Home",
  title: "eucli-box 文档树总览",
  summary: "从这里先看整体文档结构，再按具体节点跳到对应页面。主页只负责建立方向感，不混入细节结论。",
  blocks: [{ kind: "visual", visual: "document-map" }],
};
