import type { CSSProperties } from "react";
import { architectureModules, architectureRelations } from "../data/projectModel";

type ArchitectureMapProps = {
  selectedFeatureId?: string;
  onFeatureSelect: (featureId: string) => void;
};

export function ArchitectureMap({ selectedFeatureId, onFeatureSelect }: ArchitectureMapProps) {
  return (
    <section className="architecture-diagram" aria-label="atomic feature architecture view">
      <div className="architecture-intro">
        <span className="diagram-kicker">Atomic Architecture</span>
        <h2>原子功能如何组成系统架构</h2>
        <p>这张图不再维护独立架构事实，而是从统一项目模型派生：模块承载原子功能，功能关系生成模块协作边界。</p>
      </div>

      <div className="architecture-module-grid">
        {architectureModules.map((moduleView) => (
          <article className="architecture-card architecture-module-card" key={moduleView.id} style={{ "--module-accent": moduleView.domain.accent } as CSSProperties}>
            <div className="architecture-card-head">
              <span>{moduleView.domain.title}</span>
              <strong>{moduleView.title}</strong>
            </div>
            <p>{moduleView.summary}</p>
            <div className="architecture-feature-list" aria-label={`${moduleView.title} atomic features`}>
              {moduleView.features.map((feature) => (
                <button className={selectedFeatureId === feature.id ? "is-selected" : ""} key={feature.id} type="button" onClick={() => onFeatureSelect(feature.id)}>{feature.title}</button>
              ))}
            </div>
          </article>
        ))}
      </div>

      <div className="architecture-relation-list" aria-label="atomic feature derived module relations">
        {architectureRelations.map((relation) => (
          <article className="architecture-relation" key={`${relation.from.id}-${relation.to.id}`}>
            <span>{relation.features.length} 个原子功能</span>
            <strong>{relation.from.title} → {relation.to.title}</strong>
            <div className="architecture-relation-feature-row">
              {relation.features.map((feature) => (
                <button className={selectedFeatureId === feature.id ? "is-selected" : ""} key={feature.id} type="button" onClick={() => onFeatureSelect(feature.id)}>{feature.title}</button>
              ))}
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
