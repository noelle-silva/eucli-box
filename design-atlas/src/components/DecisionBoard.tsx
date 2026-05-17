import type { Decision, Risk } from "../data/designAtlas";
import { SpotlightPanel } from "./visual-system";

type DecisionBoardProps = {
  decisions: Decision[];
  risks: Risk[];
};

export function DecisionBoard({ decisions, risks }: DecisionBoardProps) {
  return (
    <div className="decision-layout">
      <div className="decision-grid">
        {decisions.map((decision) => (
          <SpotlightPanel tag="article" className="decision-card glass-lift" key={decision.index} accent="rgba(124, 58, 237, 0.16)">
            <span>{String(decision.index).padStart(2, "0")}</span>
            <h3>{decision.title}</h3>
            <p>{decision.conclusion}</p>
          </SpotlightPanel>
        ))}
      </div>
      <SpotlightPanel tag="aside" className="risk-panel" accent="rgba(225, 29, 72, 0.16)">
        <span className="eyebrow">Open Questions</span>
        <h3>仍需收口的问题</h3>
        {risks.map((risk) => (
          <article key={risk.title}>
            <strong>{risk.title}</strong>
            <p>{risk.detail}</p>
          </article>
        ))}
      </SpotlightPanel>
    </div>
  );
}
