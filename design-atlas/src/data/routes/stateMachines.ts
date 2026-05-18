import type { DesignRoute } from "./types";

export const stateMachinesRoute: DesignRoute = {
  id: "state-machines",
  label: "状态机逻辑",
  eyebrow: "State Machine",
  title: "会话、工具、权限和同步必须有明确状态出口",
  summary: "状态机页用于约束运行时行为：每个状态由什么事件进入、由什么事件离开、失败时如何暴露真实错误。",
  blocks: [
    {
      kind: "mermaid",
      title: "会话状态机",
      chart: `stateDiagram-v2
  state "空闲" as idle
  state "运行中" as running
  state "等待权限" as waiting_permission
  state "停止中" as stopping
  state "已停止" as stopped
  state "已完成" as completed
  state "失败" as failed

  [*] --> idle
  idle --> running: 提交提示词
  running --> waiting_permission: 请求权限确认
  waiting_permission --> running: 用户批准
  waiting_permission --> running: 用户拒绝并回填原因
  running --> stopping: 请求停止
  stopping --> stopped: 运行时已停止
  running --> completed: 最终回答已保存
  running --> failed: 运行时错误
  completed --> idle: 继续下一轮提示词
  stopped --> idle: 继续下一轮提示词
  failed --> idle: 恢复或重试`,
    },
    {
      kind: "mermaid",
      title: "工具调用状态机",
      chart: `stateDiagram-v2
  state "已解析" as parsed
  state "权限检查中" as permission_checking
  state "已拦截" as blocked
  state "等待用户确认" as waiting_user
  state "启动工具进程" as spawning
  state "工具运行中" as running
  state "执行成功" as succeeded
  state "执行失败" as failed
  state "结果已回填" as result_filled
  state "错误已回填" as error_filled

  [*] --> parsed
  parsed --> permission_checking
  permission_checking --> blocked: 权限拒绝
  permission_checking --> waiting_user: 需要询问用户
  permission_checking --> spawning: 权限放行
  waiting_user --> spawning: 用户批准
  waiting_user --> blocked: 用户拒绝
  spawning --> running
  running --> succeeded: stdout 返回成功
  running --> failed: 超时或退出错误
  succeeded --> result_filled: 工具结果写回模型上下文
  failed --> error_filled: 错误原因写回模型上下文
  blocked --> error_filled: 拒绝原因写回模型上下文`,
    },
    {
      kind: "table",
      title: "状态机原则",
      columns: ["原则", "说明"],
      rows: [
        ["所有阻塞都要可见", "等待权限、等待工具、等待模型都必须能通过事件展示给客户端。"],
        ["所有失败都要有出口", "模型失败、工具失败、权限拒绝、同步冲突不能静默停住。"],
        ["状态不靠 UI 猜", "客户端只展示 runtime 推送的状态，不自行推断核心状态。"],
        ["错误不兜底掩盖", "真实错误进入 error 或 tool_error 事件，方便定位根因。"],
      ],
    },
  ],
};
