import type { DesignRoute } from "./types";

export const changelogRoute: DesignRoute = {
  id: "changelog",
  label: "更改日志",
  eyebrow: "Changelog",
  title: "记录 web 文档结构和设计结论的演进",
  summary: "更改日志不记录零散编辑，而记录会影响理解、实现路线或验收口径的结构性变化。",
  blocks: [
    {
      kind: "timeline",
      title: "当前已发生的结构性变化",
      items: [
        { label: "01", title: "创建 Design Atlas", body: "建立 Vite + React + TypeScript 的独立 web 文档，用可视化页面承载 eucli-box 构想。" },
        { label: "02", title: "引入左侧导航", body: "从长页面改为导航式文档，支持按主题切换内容。" },
        { label: "03", title: "加入 Mermaid", body: "支持架构、同步、权限和路线图等图形化说明，并使用动态加载。" },
        { label: "04", title: "原子功能一等公民", body: "原子功能列表成为主入口，全部原子功能直接平铺可见，结构关系由统一项目模型派生。" },
        { label: "05", title: "迁移为 web 文档结构树", body: "文档改为具体细节约束、任务管理、记录三大分支，并把已有内容迁移归位。" },
      ],
    },
    {
      kind: "cards",
      title: "记录规则",
      items: [
        { title: "只记结构性变化", body: "页面重组、设计结论变化、功能范围变化要记录。", accent: "#7c3aed" },
        { title: "不记琐碎样式", body: "普通视觉微调不进入更改日志，避免噪音。", accent: "#0891b2" },
        { title: "关联影响范围", body: "每条重要变更要能说清影响了哪些页面或功能树节点。", accent: "#059669" },
        { title: "服务后续核对", body: "更改日志要能帮助回看为什么文档结构和设计方向变成现在这样。", accent: "#d97706" },
      ],
    },
  ],
};
