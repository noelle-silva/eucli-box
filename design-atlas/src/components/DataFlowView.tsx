import { dataFlowSteps } from "../data/projectModel";

type DataFlowViewProps = {
  selectedFeatureId?: string;
  onFeatureSelect: (featureId: string) => void;
};

export function DataFlowView({ selectedFeatureId, onFeatureSelect }: DataFlowViewProps) {
  return (
    <section className="data-flow-view" aria-label="atomic feature data flow view">
      <div className="data-flow-intro">
        <span className="diagram-kicker">Atomic Flow</span>
        <h2>原子功能驱动的数据流</h2>
        <p>数据流节点来自原子功能的 flow 字段；输入、输出、事件和状态变化不再在页面里单独维护。</p>
      </div>

      <div className="data-flow-rail">
        {dataFlowSteps.map((step, index) => (
          <button className={`data-flow-card ${selectedFeatureId === step.feature.id ? "is-selected" : ""}`} key={step.feature.id} type="button" onClick={() => onFeatureSelect(step.feature.id)}>
            <span className="data-flow-index">{String(index + 1).padStart(2, "0")}</span>
            <div>
              <small>{step.module.title}</small>
              <h3>{step.feature.title}</h3>
              <p>{step.feature.intent}</p>
              <FlowPills label="输入" values={step.inputs} />
              <FlowPills label="输出" values={step.outputs} />
              <FlowPills label="事件" values={step.emits} />
              <FlowPills label="状态" values={step.stateChanges} />
            </div>
          </button>
        ))}
      </div>
    </section>
  );
}

function FlowPills({ label, values }: { label: string; values: string[] }) {
  if (values.length === 0) {
    return null;
  }

  return (
    <div className="data-flow-pill-row">
      <strong>{label}</strong>
      {values.map((value) => (
        <span key={value}>{value}</span>
      ))}
    </div>
  );
}
