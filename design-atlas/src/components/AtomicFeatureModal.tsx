import { getAtomicFeatureView, stageLabels } from "../domain/projectDocIndex";

type AtomicFeatureModalProps = {
  featureId: string;
  onClose: () => void;
  onFeatureOpen: (featureId: string) => void;
};

export function AtomicFeatureModal({ featureId, onClose, onFeatureOpen }: AtomicFeatureModalProps) {
  const feature = getAtomicFeatureView(featureId);

  return (
    <div className="modal-layer" role="presentation" onMouseDown={onClose}>
      <section aria-labelledby="feature-modal-title" aria-modal="true" className="feature-modal" role="dialog" onMouseDown={(event) => event.stopPropagation()}>
        <header className="modal-head">
          <div>
            <span className="eyebrow">{feature.id}</span>
            <h2 id="feature-modal-title">{feature.title}</h2>
            <p>{feature.intent}</p>
          </div>
          <button className="close-button" type="button" onClick={onClose}>关闭</button>
        </header>

        <div className="fact-grid">
          <Fact label="领域" value={feature.domain.name} />
          <Fact label="模块" value={feature.module.name} />
          <Fact label="阶段" value={stageLabels[feature.stage]} />
        </div>

        <section className="modal-section wide">
          <h3>验收口径</h3>
          <p>{feature.acceptance}</p>
        </section>

        <div className="modal-grid">
          <FeatureLinks title="依赖" empty="暂无依赖" features={feature.dependencies} onFeatureOpen={onFeatureOpen} />
          <FeatureLinks title="支持后续" empty="暂无后续功能" features={feature.supportedFeatures} onFeatureOpen={onFeatureOpen} />
          <FeatureLinks title="被依赖" empty="暂无被依赖" features={feature.dependentFeatures} onFeatureOpen={onFeatureOpen} />
          <TextChips title="输入" empty="暂无输入" values={feature.signals.inputs} />
          <TextChips title="输出" empty="暂无输出" values={feature.signals.outputs} />
          <TextChips title="事件" empty="暂无事件" values={feature.signals.events} />
        </div>
      </section>
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="fact-card">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function FeatureLinks({ title, empty, features, onFeatureOpen }: { title: string; empty: string; features: Array<{ id: string; title: string }>; onFeatureOpen: (featureId: string) => void }) {
  return (
    <section className="modal-section">
      <h3>{title}</h3>
      {features.length ? (
        <div className="chip-row">
          {features.map((feature) => (
            <button type="button" key={feature.id} onClick={() => onFeatureOpen(feature.id)}>{feature.title}</button>
          ))}
        </div>
      ) : (
        <p>{empty}</p>
      )}
    </section>
  );
}

function TextChips({ title, empty, values }: { title: string; empty: string; values: string[] }) {
  return (
    <section className="modal-section">
      <h3>{title}</h3>
      {values.length ? (
        <div className="chip-row text-only">
          {values.map((value) => <span key={value}>{value}</span>)}
        </div>
      ) : (
        <p>{empty}</p>
      )}
    </section>
  );
}
