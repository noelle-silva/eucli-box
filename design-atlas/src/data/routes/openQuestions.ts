import type { DesignRoute } from "./types";

export const openQuestionsRoute: DesignRoute = {
  id: "open-questions",
  label: "待解决问题",
  eyebrow: "Open Questions",
  title: "下一阶段需要收口的实现问题",
  summary: "问题池用于记录会阻塞实现、影响数据模型或改变验收口径的事项。",
  blocks: [
    {
      kind: "cards",
      title: "当前问题池",
      items: [
        { title: "MVP 边界", body: "第一版必须做、预留接口、后续增强需要明确切分。", accent: "#e11d48" },
        { title: "同步口径", body: "共享数据库与中心权威本地副本的关系需要在实现稿中收紧。", accent: "#d97706" },
        { title: "事件可靠性", body: "WebSocket 是否需要事件 ID、顺序保证、重连补发仍需确定。", accent: "#2563eb" },
        { title: "密钥分发", body: "多实例下加密密钥如何安全持有和轮换，需要专门设计。", accent: "#7c3aed" },
        { title: "工具 manifest", body: "工具身份证字段格式需要正式定义。", accent: "#0891b2" },
        { title: "记忆是否进主线", body: "记忆系统第一版是进入主链路，还是先作为插件预留。", accent: "#059669" },
      ],
    },
    {
      kind: "table",
      title: "问题管理口径",
      columns: ["字段", "说明"],
      rows: [
        ["问题", "需要被明确回答的设计或实现疑问。"],
        ["影响范围", "会影响哪些功能树节点、数据模型或状态机。"],
        ["阻塞程度", "是否阻塞 MVP，还是只影响 Alpha/Beta/Production。"],
        ["建议动作", "需要调研、决策、实验还是直接实现。"],
      ],
    },
  ],
};
