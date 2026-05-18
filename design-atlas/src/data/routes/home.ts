import type { DesignRoute } from "./types";

export const homeRoute: DesignRoute = {
  id: "home",
  label: "主页",
  eyebrow: "Atlas Home",
  title: "eucli-box 文档树总览",
  summary: "从这里先看整体文档结构，再按具体节点跳到对应页面。主页只负责建立方向感，不混入细节结论。",
  blocks: [
    { kind: "visual", visual: "document-map" },
    {
      kind: "cards",
      title: "主页使用方式",
      items: [
        { title: "先看主枝", body: "从左到右扫一遍三条主枝：具体细节约束、任务管理、记录。", accent: "#7c3aed" },
        { title: "再点叶子", body: "点击右侧页面节点，直接跳到对应文档页，不需要先理解全部内容。", accent: "#0891b2" },
        { title: "只认真实目录", body: "这张图由当前 documentTree 自动生成，避免主页和侧边栏出现两套结构。", accent: "#059669" },
        { title: "后续可调整", body: "后面你总结出规范后，可以继续把这张图升级成更贴合你理解模式的入口。", accent: "#d97706" },
      ],
    },
  ],
};
