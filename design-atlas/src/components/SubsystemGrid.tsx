import type { CSSProperties } from "react";
import type { Subsystem } from "../data/designAtlas";
import { SpotlightPanel } from "./visual-system";

const statusText = {
  core: "核心闭环",
  defined: "已定义",
  planned: "计划中",
};

type SubsystemGridProps = {
  subsystems: Subsystem[];
};

export function SubsystemGrid({ subsystems }: SubsystemGridProps) {
  return (
    <div className="subsystem-grid">
      {subsystems.map((system) => (
        <SpotlightPanel
          tag="article"
          className="subsystem-card glass-lift"
          key={system.name}
          accent={`${system.accent}2e`}
          style={{ "--accent": system.accent } as CSSProperties}
        >
          <div className="subsystem-topline">
            <span className="status-pill">{statusText[system.status]}</span>
            <span className="status-dot" />
          </div>
          <h3>{system.name}</h3>
          <p>{system.role}</p>
        </SpotlightPanel>
      ))}
    </div>
  );
}
