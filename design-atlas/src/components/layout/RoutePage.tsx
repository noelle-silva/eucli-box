import type { CSSProperties } from "react";
import { ArchitectureMap } from "../ArchitectureMap";
import { DecisionBoard } from "../DecisionBoard";
import { DocumentMindMap } from "../DocumentMindMap";
import { FeatureChecklist } from "../FeatureChecklist";
import { MvpFlow } from "../MvpFlow";
import { MermaidDiagram } from "../MermaidDiagram";
import { Roadmap } from "../Roadmap";
import { SubsystemGrid } from "../SubsystemGrid";
import { GradientText, SpotlightPanel } from "../visual-system";
import { decisions, risks, roadmap, subsystems } from "../../data/designAtlas";
import { featureTree } from "../../data/featureChecklist";
import type { DesignRoute, DocumentTreeSection, RouteBlock } from "../../data/routes";

type RoutePageProps = {
  activeRouteId: string;
  route: DesignRoute;
  tree: DocumentTreeSection[];
  onRouteChange: (routeId: string) => void;
};

export function RoutePage({ activeRouteId, route, tree, onRouteChange }: RoutePageProps) {
  const isImmersive = route.layout === "immersive";

  return (
    <article className={isImmersive ? "route-page route-page-immersive" : "route-page"}>
      {isImmersive ? null : (
        <header className="route-hero">
          <span className="eyebrow">{route.eyebrow}</span>
          <h1>
            <GradientText>{route.title}</GradientText>
          </h1>
          <p>{route.summary}</p>
        </header>
      )}

      <div className="route-blocks">
        {route.blocks.map((block, index) => (
          <RouteBlockView activeRouteId={activeRouteId} block={block} key={`${route.id}-${block.kind}-${index}`} onRouteChange={onRouteChange} tree={tree} />
        ))}
      </div>
    </article>
  );
}

function RouteBlockView({
  activeRouteId,
  block,
  tree,
  onRouteChange,
}: {
  activeRouteId: string;
  block: RouteBlock;
  tree: DocumentTreeSection[];
  onRouteChange: (routeId: string) => void;
}) {
  if (block.kind === "visual") {
    return <VisualBlock activeRouteId={activeRouteId} block={block} onRouteChange={onRouteChange} tree={tree} />;
  }

  return (
    <SpotlightPanel tag="section" className={`doc-block doc-block-${block.kind} glass-lift`} accent="rgba(124, 58, 237, 0.14)">
      {"title" in block && block.title ? <h2>{block.title}</h2> : null}
      {block.kind === "text" ? <TextBlock block={block} /> : null}
      {block.kind === "cards" ? <CardsBlock block={block} /> : null}
      {block.kind === "table" ? <TableBlock block={block} /> : null}
      {block.kind === "code" ? <CodeBlock block={block} /> : null}
      {block.kind === "mermaid" ? <MermaidBlock block={block} /> : null}
      {block.kind === "timeline" ? <TimelineBlock block={block} /> : null}
    </SpotlightPanel>
  );
}

function TextBlock({ block }: { block: Extract<RouteBlock, { kind: "text" }> }) {
  return (
    <div className="doc-copy">
      {block.paragraphs.map((paragraph) => (
        <p key={paragraph}>{paragraph}</p>
      ))}
    </div>
  );
}

function CardsBlock({ block }: { block: Extract<RouteBlock, { kind: "cards" }> }) {
  return (
    <div className="doc-card-grid">
      {block.items.map((item) => (
        <article className="doc-card" key={item.title} style={{ "--card-accent": item.accent ?? "#7c3aed" } as CSSProperties}>
          {item.meta ? <span>{item.meta}</span> : null}
          <h3>{item.title}</h3>
          <p>{item.body}</p>
        </article>
      ))}
    </div>
  );
}

function TableBlock({ block }: { block: Extract<RouteBlock, { kind: "table" }> }) {
  return (
    <div className="doc-table-wrap">
      <table className="doc-table">
        <thead>
          <tr>
            {block.columns.map((column) => (
              <th key={column}>{column}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {block.rows.map((row) => (
            <tr key={row.join("|")}>
              {row.map((cell, index) => (
                <td key={`${cell}-${index}`}>{cell}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function CodeBlock({ block }: { block: Extract<RouteBlock, { kind: "code" }> }) {
  return <pre className="doc-code"><code>{block.code}</code></pre>;
}

function MermaidBlock({ block }: { block: Extract<RouteBlock, { kind: "mermaid" }> }) {
  return <MermaidDiagram chart={block.chart} />;
}

function TimelineBlock({ block }: { block: Extract<RouteBlock, { kind: "timeline" }> }) {
  return (
    <div className="doc-timeline">
      {block.items.map((item) => (
        <article key={item.label}>
          <span>{item.label}</span>
          <div>
            <h3>{item.title}</h3>
            <p>{item.body}</p>
          </div>
        </article>
      ))}
    </div>
  );
}

function VisualBlock({
  activeRouteId,
  block,
  tree,
  onRouteChange,
}: {
  activeRouteId: string;
  block: Extract<RouteBlock, { kind: "visual" }>;
  tree: DocumentTreeSection[];
  onRouteChange: (routeId: string) => void;
}) {
  return (
    <section className={block.visual === "document-map" ? "visual-block visual-block-document-map" : "visual-block"}>
      {block.title ? <h2>{block.title}</h2> : null}
      {block.visual === "architecture" ? <ArchitectureMap /> : null}
      {block.visual === "document-map" ? <DocumentMindMap activeRouteId={activeRouteId} onRouteChange={onRouteChange} tree={tree} /> : null}
      {block.visual === "feature-tree" ? <FeatureChecklist tree={featureTree} /> : null}
      {block.visual === "subsystems" ? <SubsystemGrid subsystems={subsystems} /> : null}
      {block.visual === "mvp" ? <MvpFlow /> : null}
      {block.visual === "decisions" ? <DecisionBoard decisions={decisions} risks={risks} /> : null}
      {block.visual === "roadmap" ? <Roadmap roadmap={roadmap} /> : null}
    </section>
  );
}
