import { featureDeliveryStageLabels, roadmapStages } from "../data/projectModel";
import { SpotlightPanel } from "./visual-system";

type RoadmapProps = {
  selectedFeatureId?: string;
  onFeatureSelect: (featureId: string) => void;
};

export function Roadmap({ selectedFeatureId, onFeatureSelect }: RoadmapProps) {
  return (
    <div className="roadmap" id="roadmap">
      {roadmapStages.map((stage, index) => (
        <SpotlightPanel tag="article" className="roadmap-stage glass-lift" key={stage.stage} accent="rgba(6, 182, 212, 0.16)">
          <span className="stage-number">{String(index + 1).padStart(2, "0")}</span>
          <h3>{stage.title}</h3>
          <p>{stage.outcome}</p>
          <div className="roadmap-stage-meta">
            <span>{featureDeliveryStageLabels[stage.stage]}</span>
            <span>{stage.features.length} 个原子功能</span>
          </div>
          <ul>
            {stage.features.map((feature) => (
              <li className={selectedFeatureId === feature.id ? "is-selected" : ""} key={feature.id}>
                <strong>{feature.title}</strong>
                <span>{featureDeliveryStageLabels[feature.stage]}</span>
                <button type="button" onClick={() => onFeatureSelect(feature.id)}>{selectedFeatureId === feature.id ? "已选中" : "查看"}</button>
              </li>
            ))}
          </ul>
        </SpotlightPanel>
      ))}
    </div>
  );
}
