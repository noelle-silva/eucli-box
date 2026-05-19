import type { DesignRoute } from "./types";

export const dataFlowRoute: DesignRoute = {
  id: "data-flow",
  label: "数据流向图",
  eyebrow: "Data Flow",
  title: "命令、事件、工具结果和同步数据各走各的通道",
  summary: "REST 负责命令和查询，WebSocket 负责实时事件，工具通过 stdin/stdout JSON 通信，实例与中心通过分频道增量同步。",
  blocks: [
    { kind: "visual", visual: "data-flow" },
  ],
};
