import type { DesignRoute } from "./types";

export const dataModelRoute: DesignRoute = {
  id: "data-model",
  label: "数据模型",
  eyebrow: "Data Model",
  title: "核心实体、频道归属和关系边界",
  summary: "把会话、消息、Agent、供应商、工具、权限、占位符和记忆拆成可持久化、可同步、可验收的数据实体。",
  blocks: [
    {
      kind: "mermaid",
      title: "核心实体关系",
      chart: `flowchart LR
  Agent[Agent 身份]
  Session[会话]
  Message[消息]
  ToolCall[工具调用记录]
  Tool[工具]
  AgentToolRule[Agent 工具规则]
  Provider[供应商]
  Model[模型]
  ModelGroup[模型组]
  ModelGroupMember[模型组成员]
  Placeholder[占位符]
  PromptTemplate[提示词模板]
  PermissionRule[权限规则]

  Agent -->|拥有| Session
  Session -->|包含| Message
  Message -->|记录| ToolCall
  Tool -->|执行| ToolCall
  Agent -->|限制| AgentToolRule
  Tool -->|被规则引用| AgentToolRule
  Provider -->|提供| Model
  ModelGroup -->|包含| ModelGroupMember
  Model -->|加入| ModelGroupMember
  Placeholder -->|注入| PromptTemplate
  PermissionRule -->|裁决| ToolCall`,
    },
    {
      kind: "table",
      title: "实体归属",
      columns: ["实体", "同步频道", "核心字段", "说明"],
      rows: [
        ["Agent", "agent", "id、name、avatar、description、systemPromptRef", "会话、人设、工具权限和展示元数据的组合实体"],
        ["Session", "session", "id、agentId、title、status、summary", "会话生命周期和状态入口"],
        ["Message", "chat", "id、sessionId、role、parts、createdAt", "用户、AI、工具结果统一进入消息流"],
        ["Tool", "tool_config", "id、name、manifest、command、timeout、env", "工具身份和运行配置分离"],
        ["Provider", "provider", "id、type、baseUrl、encryptedApiKey", "密钥即数据，但以密文保存"],
        ["PermissionRule", "permission", "scope、action、effect、condition", "allow / deny / ask 的规则来源"],
        ["Placeholder", "prompt", "name、source、value、resolver", "支持静态文本、工具链接和系统插件动态值"],
      ],
    },
    {
      kind: "cards",
      title: "建模原则",
      items: [
        { title: "会话按 Agent 分隔", body: "发起聊天必须选择 Agent，不同 Agent 的会话和权限互不混淆。", accent: "#7c3aed" },
        { title: "工具身份和运行配置分离", body: "manifest 是工具身份证，数据中心配置是运行配置单。", accent: "#0891b2" },
        { title: "密钥即数据", body: "供应商密钥进入 provider 频道，但必须加密存储，由实例本地解密。", accent: "#059669" },
        { title: "配置版本化", body: "低频结构化配置使用版本号与乐观锁，避免冲突被静默覆盖。", accent: "#d97706" },
      ],
    },
  ],
};
