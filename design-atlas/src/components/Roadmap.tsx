import type { RoadmapStage } from "../data/designAtlas";
import { SpotlightPanel } from "./visual-system";

type RoadmapProps = {
  roadmap: RoadmapStage[];
};

export function Roadmap({ roadmap }: RoadmapProps) {
  return (
    <div className="roadmap" id="roadmap">
      {roadmap.map((stage) => (
        <SpotlightPanel tag="article" className="roadmap-stage glass-lift" key={stage.stage} accent="rgba(6, 182, 212, 0.16)">
          <span className="stage-number">{stage.stage}</span>
          <h3>{stage.title}</h3>
          <p>{stage.outcome}</p>
          <ul>
            {stage.items.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </SpotlightPanel>
      ))}
    </div>
  );
}
