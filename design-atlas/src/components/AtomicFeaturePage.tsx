import { useState } from "react";
import { atomicFeatureIndex, getAtomicFeatureView, stageLabels } from "../domain/projectDocIndex";
import { AtomicFeatureModal } from "./AtomicFeatureModal";
import { SpotlightCard } from "./ui/SpotlightCard";

export function AtomicFeaturePage() {
  const [activeFeatureId, setActiveFeatureId] = useState<string | undefined>();

  function openFeature(featureId: string) {
    setActiveFeatureId(featureId);
  }

  return (
    <main className="page-shell custom-scroll-area">
      <section className="page-intro">
        <span className="eyebrow">Atomic Feature List</span>
        <h1>原子功能清单</h1>
        <p>所有项目能力直接以原子功能卡片呈现。每张卡片都是一级对象，点击后查看验收、依赖、后续支持和数据流信号。</p>
      </section>

      <section className="feature-grid" aria-label="原子功能卡片列表">
        {atomicFeatureIndex.map((feature, index) => {
          const view = getAtomicFeatureView(feature.id);

          return (
            <SpotlightCard
              tag="button"
              type="button"
              className="feature-card"
              key={feature.id}
              spotlightColor="rgba(255,255,255,0.42)"
              onClick={() => openFeature(feature.id)}
            >
              <div className="feature-card-head">
                <span>{String(index + 1).padStart(2, "0")}</span>
                <strong>{feature.id}</strong>
              </div>
              <h2>{feature.title}</h2>
              <p>{feature.intent}</p>
              <div className="feature-tags">
                <span>{view.domain.name}</span>
                <span>{view.module.name}</span>
                <span>{stageLabels[feature.stage]}</span>
              </div>
              <div className="relation-grid">
                <Metric label="依赖" value={view.dependencies.length} />
                <Metric label="支持" value={view.supportedFeatures.length} />
                <Metric label="事件" value={view.signals.events.length} />
              </div>
            </SpotlightCard>
          );
        })}
      </section>

      {activeFeatureId ? <AtomicFeatureModal featureId={activeFeatureId} onClose={() => setActiveFeatureId(undefined)} onFeatureOpen={openFeature} /> : null}
    </main>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <span>
      <strong>{value}</strong>
      {label}
    </span>
  );
}
