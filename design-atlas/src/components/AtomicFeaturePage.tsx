import { useState } from "react";
import type { ProjectDocIndex } from "../domain/projectDocIndex";
import { stageLabels } from "../domain/projectDocIndex";
import { AtomicFeatureModal } from "./AtomicFeatureModal";
import { SpotlightCard } from "./ui/SpotlightCard";

type AtomicFeaturePageProps = {
  index: ProjectDocIndex;
};

export function AtomicFeaturePage({ index }: AtomicFeaturePageProps) {
  const [activeFeatureId, setActiveFeatureId] = useState<string | undefined>();

  function openFeature(featureId: string) {
    setActiveFeatureId(featureId);
  }

  return (
    <main className="page-shell custom-scroll-area">
      <section className="page-intro">
        <span className="eyebrow">Atomic Feature List</span>
        <h1>{index.project.name} 原子功能清单</h1>
        <p>{index.project.summary} 当前页面只展示这个项目自己的原子功能索引，点击卡片后查看验收、依赖、后续支持和数据流信号。</p>
      </section>

      <section className="feature-grid" aria-label="原子功能卡片列表">
        {index.atomicFeatureIndex.map((feature, featureIndex) => {
          const view = index.getAtomicFeatureView(feature.id);

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
                <span>{String(featureIndex + 1).padStart(2, "0")}</span>
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

      {activeFeatureId ? <AtomicFeatureModal featureId={activeFeatureId} index={index} onClose={() => setActiveFeatureId(undefined)} onFeatureOpen={openFeature} /> : null}
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
