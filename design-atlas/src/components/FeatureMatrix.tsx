import type { Feature, Priority } from "../data/designAtlas";
import { SpotlightPanel } from "./visual-system";

const priorityLabels: Record<Priority, string> = {
  P0: "第一版生命线",
  P1: "结构化增强",
  P2: "未来演进",
};

type FeatureMatrixProps = {
  features: Feature[];
};

export function FeatureMatrix({ features }: FeatureMatrixProps) {
  const priorities: Priority[] = ["P0", "P1", "P2"];

  return (
    <div className="feature-matrix" id="features">
      {priorities.map((priority) => (
        <section className={`feature-column ${priority.toLowerCase()}`} key={priority}>
          <div className="feature-column-header">
            <strong>{priority}</strong>
            <span>{priorityLabels[priority]}</span>
          </div>
          {features
            .filter((feature) => feature.priority === priority)
            .map((feature) => (
              <SpotlightPanel tag="article" className="feature-card glass-lift" key={feature.name} accent="rgba(6, 182, 212, 0.18)">
                <span>{feature.pillar}</span>
                <h3>{feature.name}</h3>
                <p>{feature.description}</p>
              </SpotlightPanel>
            ))}
        </section>
      ))}
    </div>
  );
}
