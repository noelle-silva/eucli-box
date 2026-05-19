import type { DocumentTreeSection } from "./types";

export const documentTree: DocumentTreeSection[] = [
  {
    id: "constraints",
    label: "具体细节约束",
    summary: "定义系统应该是什么、怎么运行、怎么验收。",
    children: [
      { kind: "route", id: "atomic-feature-list", label: "原子功能列表", routeId: "features", summary: "直接平铺展示所有原子功能，每个功能可展开查看关系与验收。" },
      { kind: "route", id: "architecture-map", label: "架构图", routeId: "architecture", summary: "客户端、运行时实例、数据中心和外部能力关系。" },
      { kind: "route", id: "data-flow-map", label: "数据流向图", routeId: "data-flow", summary: "请求、事件、同步和工具结果的流转路径。" },
      { kind: "route", id: "data-model", label: "数据模型", routeId: "data-model", summary: "核心实体、归属频道和关键关系。" },
      { kind: "route", id: "ui-prototype", label: "UI 交互原型", routeId: "ui-prototype", summary: "客户端看见什么、点什么、收到什么反馈。" },
      { kind: "route", id: "user-journey", label: "用户故事路径", routeId: "user-journeys", summary: "从用户目标反推完整运行链路。" },
      { kind: "route", id: "state-machine", label: "状态机逻辑", routeId: "state-machines", summary: "会话、工具、权限和同步的状态转换。" },
      { kind: "route", id: "design-decisions", label: "设计决策", routeId: "decisions", summary: "已经收口的架构选择和取舍。" },
    ],
  },
  {
    id: "task-management",
    label: "任务管理",
    summary: "把设计约束转成可以推进的开发任务和问题池。",
    children: [
      { kind: "route", id: "implementation-route", label: "实现路线", routeId: "implementation-roadmap", summary: "按阶段推进 MVP、Alpha、Beta、Production。" },
      { kind: "route", id: "open-questions", label: "待解决问题", routeId: "open-questions", summary: "记录会阻塞实现或需要决策的问题。" },
    ],
  },
  {
    id: "records",
    label: "记录",
    summary: "记录文档结构、设计范围和关键结论的变化。",
    children: [{ kind: "route", id: "change-log", label: "更改日志", routeId: "changelog", summary: "追踪 web 文档与设计结论的演进。" }],
  },
];
