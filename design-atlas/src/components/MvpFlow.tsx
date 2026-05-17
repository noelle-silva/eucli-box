const steps = ["发消息", "调用模型", "解析 VCP", "权限判断", "子进程工具", "结果回填", "保存会话"];

export function MvpFlow() {
  return (
    <div className="mvp-flow">
      {steps.map((step, index) => (
        <div className="flow-step" key={step}>
          <span>{String(index + 1).padStart(2, "0")}</span>
          <strong>{step}</strong>
        </div>
      ))}
    </div>
  );
}
