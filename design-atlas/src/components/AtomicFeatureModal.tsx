import { useEffect, useRef } from "react";
import { featureDeliveryStageLabels, getAtomicFeatureDetail } from "../data/projectModel";

type AtomicFeatureModalProps = {
  featureId: string;
  onClose: () => void;
  onFeatureSelect: (featureId: string) => void;
  onRouteChange: (routeId: string) => void;
};

export function AtomicFeatureModal({ featureId, onClose, onFeatureSelect, onRouteChange }: AtomicFeatureModalProps) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const detail = getAtomicFeatureDetail(featureId);

  useEffect(() => {
    closeButtonRef.current?.focus();

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="atomic-feature-modal-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        aria-labelledby="atomic-feature-modal-title"
        aria-modal="true"
        className="atomic-feature-modal"
        role="dialog"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="atomic-feature-modal-head">
          <div>
            <span>{detail.id}</span>
            <h2 id="atomic-feature-modal-title">{detail.title}</h2>
            <p>{detail.intent}</p>
          </div>
          <button ref={closeButtonRef} type="button" onClick={onClose}>关闭</button>
        </header>

        <div className="atomic-feature-modal-facts" aria-label={`${detail.title} 基础事实`}>
          <Fact label="领域" value={detail.domain.title} />
          <Fact label="模块" value={detail.module.title} />
          <Fact label="阶段" value={featureDeliveryStageLabels[detail.stage]} />
        </div>

        <section className="atomic-feature-modal-section">
          <h3>验收口径</h3>
          <p>{detail.acceptance}</p>
        </section>

        <div className="atomic-feature-modal-grid">
          <FeatureLinkGroup title="依赖功能" empty="暂无依赖" features={detail.dependencies} onFeatureSelect={onFeatureSelect} />
          <FeatureLinkGroup title="被哪些功能依赖" empty="暂无下游依赖" features={detail.dependents} onFeatureSelect={onFeatureSelect} />
          <TextGroup title="关联模块" empty="暂无跨模块关联" values={detail.relatedModules.map((projectModule) => projectModule.title)} />
          <TextGroup title="架构提供" empty="暂无显式提供项" values={detail.architecture?.provides ?? []} />
          <TextGroup title="架构消费" empty="暂无显式消费项" values={detail.architecture?.consumes ?? []} />
          <TextGroup title="输入" empty="暂无输入信号" values={detail.flow?.inputs ?? []} />
          <TextGroup title="输出" empty="暂无输出信号" values={detail.flow?.outputs ?? []} />
          <TextGroup title="事件" empty="暂无事件信号" values={detail.flow?.emits ?? []} />
          <TextGroup title="状态变化" empty="暂无状态变化" values={detail.flow?.stateChanges ?? []} />
        </div>

        <section className="atomic-feature-modal-section">
          <h3>跨页面视角</h3>
          <div className="atomic-feature-modal-actions">
            {detail.references.map((reference) => (
              <button type="button" key={reference.routeId} onClick={() => onRouteChange(reference.routeId)}>{reference.title}</button>
            ))}
          </div>
        </section>
      </section>
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function FeatureLinkGroup({ title, empty, features, onFeatureSelect }: { title: string; empty: string; features: Array<{ id: string; title: string }>; onFeatureSelect: (featureId: string) => void }) {
  return (
    <section className="atomic-feature-modal-section">
      <h3>{title}</h3>
      {features.length ? (
        <div className="atomic-feature-modal-actions">
          {features.map((feature) => (
            <button type="button" key={feature.id} onClick={() => onFeatureSelect(feature.id)}>{feature.title}</button>
          ))}
        </div>
      ) : (
        <p>{empty}</p>
      )}
    </section>
  );
}

function TextGroup({ title, empty, values }: { title: string; empty: string; values: string[] }) {
  return (
    <section className="atomic-feature-modal-section">
      <h3>{title}</h3>
      {values.length ? (
        <div className="atomic-feature-modal-chip-row">
          {values.map((value) => (
            <span key={value}>{value}</span>
          ))}
        </div>
      ) : (
        <p>{empty}</p>
      )}
    </section>
  );
}
