import { useEffect, useState } from "react";
import { atomicFeatureConnections, featureDeliveryStageLabels, getAtomicFeatureDetail } from "../data/projectModel";
import { AtomicFeatureModal } from "./AtomicFeatureModal";
import { SpotlightPanel } from "./visual-system";

type AtomicFeatureListProps = {
  selectedFeatureId?: string;
  onFeatureSelect: (featureId: string) => void;
  onRouteChange: (routeId: string) => void;
};

export function AtomicFeatureList({ selectedFeatureId, onFeatureSelect, onRouteChange }: AtomicFeatureListProps) {
  const [activeModalFeatureId, setActiveModalFeatureId] = useState<string | undefined>(selectedFeatureId);

  function openFeature(featureId: string) {
    onFeatureSelect(featureId);
    setActiveModalFeatureId(featureId);
  }

  useEffect(() => {
    setActiveModalFeatureId(selectedFeatureId);
  }, [selectedFeatureId]);

  return (
    <div className="atomic-feature-list">
      <header className="atomic-feature-list-head">
        <div>
          <span className="diagram-kicker">Atomic Features</span>
          <h2>全部原子功能</h2>
          <p>每个原子功能都是一级对象；模块、阶段、依赖、架构关系和数据流都作为它自身的信息展开。</p>
        </div>
        <div className="atomic-feature-list-total" aria-label="原子功能总数">
          <strong>{atomicFeatureConnections.length}</strong>
          <span>个原子功能</span>
        </div>
      </header>

      <div className="atomic-feature-grid">
        {atomicFeatureConnections.map((connection, index) => {
          const detail = getAtomicFeatureDetail(connection.feature.id);
          const isSelected = selectedFeatureId === connection.feature.id;

          return (
            <SpotlightPanel
              tag="button"
              type="button"
              className={`atomic-feature-card glass-lift ${isSelected ? "is-selected" : ""}`}
              key={connection.feature.id}
              accent="rgba(124, 58, 237, 0.14)"
              onClick={() => openFeature(connection.feature.id)}
            >
              <div className="atomic-feature-card-head">
                <span>{String(index + 1).padStart(2, "0")}</span>
                <strong>{connection.feature.id}</strong>
              </div>

              <h3>{connection.feature.title}</h3>
              <p>{connection.feature.intent}</p>

              <div className="atomic-feature-fact-row" aria-label={`${connection.feature.title} 基础信息`}>
                <span>{detail.domain.title}</span>
                <span>{detail.module.title}</span>
                <span>{featureDeliveryStageLabels[connection.feature.stage]}</span>
              </div>

              <div className="atomic-feature-relation-strip" aria-label={`${connection.feature.title} 关系概览`}>
                <RelationStat label="依赖" value={connection.dependencies.length} />
                <RelationStat label="被依赖" value={connection.dependents.length} />
                <RelationStat label="关联模块" value={connection.relatedModules.length} />
                <RelationStat label="数据流" value={connection.hasFlowSignals ? 1 : 0} />
              </div>
            </SpotlightPanel>
          );
        })}
      </div>

      {activeModalFeatureId ? (
        <AtomicFeatureModal
          featureId={activeModalFeatureId}
          onClose={() => setActiveModalFeatureId(undefined)}
          onFeatureSelect={openFeature}
          onRouteChange={onRouteChange}
        />
      ) : null}
    </div>
  );
}

function RelationStat({ label, value }: { label: string; value: number }) {
  return (
    <span>
      <strong>{value}</strong>
      {label}
    </span>
  );
}
