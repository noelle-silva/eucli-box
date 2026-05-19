import type { CSSProperties } from "react";
import { ArchitectureMap } from "../ArchitectureMap";
import { AtomicFeatureList } from "../AtomicFeatureList";
import { DataFlowView } from "../DataFlowView";
import { DecisionBoard } from "../DecisionBoard";
import { DocumentMindMap } from "../DocumentMindMap";
import { MvpFlow } from "../MvpFlow";
import { MermaidDiagram } from "../MermaidDiagram";
import { Roadmap } from "../Roadmap";
import { SpotlightPanel } from "../visual-system";
import { decisions, risks } from "../../data/designAtlas";
import type { DesignRoute, DocumentTreeSection, RouteBlock } from "../../data/routes";

type RoutePageProps = {
  activeRouteId: string;
  route: DesignRoute;
  selectedFeatureId?: string;
  tree: DocumentTreeSection[];
  onFeatureSelect: (featureId: string) => void;
  onRouteChange: (routeId: string) => void;
};

export function RoutePage({ activeRouteId, route, selectedFeatureId, tree, onFeatureSelect, onRouteChange }: RoutePageProps) {
  const isImmersive = route.layout === "immersive";

  return (
    <article className={isImmersive ? "route-page route-page-immersive" : "route-page"}>
      <div className="route-blocks">
        {route.blocks.map((block, index) => (
          <RouteBlockView
            activeRouteId={activeRouteId}
            block={block}
            key={`${route.id}-${block.kind}-${index}`}
            onFeatureSelect={onFeatureSelect}
            onRouteChange={onRouteChange}
            selectedFeatureId={selectedFeatureId}
            tree={tree}
          />
        ))}
      </div>
    </article>
  );
}

function RouteBlockView({
  activeRouteId,
  block,
  selectedFeatureId,
  tree,
  onFeatureSelect,
  onRouteChange,
}: {
  activeRouteId: string;
  block: RouteBlock;
  selectedFeatureId?: string;
  tree: DocumentTreeSection[];
  onFeatureSelect: (featureId: string) => void;
  onRouteChange: (routeId: string) => void;
}) {
  if (block.kind === "visual") {
    return <VisualBlock activeRouteId={activeRouteId} block={block} onFeatureSelect={onFeatureSelect} onRouteChange={onRouteChange} selectedFeatureId={selectedFeatureId} tree={tree} />;
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
  selectedFeatureId,
  tree,
  onFeatureSelect,
  onRouteChange,
}: {
  activeRouteId: string;
  block: Extract<RouteBlock, { kind: "visual" }>;
  selectedFeatureId?: string;
  tree: DocumentTreeSection[];
  onFeatureSelect: (featureId: string) => void;
  onRouteChange: (routeId: string) => void;
}) {
  return (
    <section className={block.visual === "document-map" ? "visual-block visual-block-document-map" : "visual-block"}>
      {block.title ? <h2>{block.title}</h2> : null}
      {block.visual === "architecture" ? <ArchitectureMap onFeatureSelect={onFeatureSelect} selectedFeatureId={selectedFeatureId} /> : null}
      {block.visual === "atomic-feature-list" ? <AtomicFeatureList onFeatureSelect={onFeatureSelect} onRouteChange={onRouteChange} selectedFeatureId={selectedFeatureId} /> : null}
      {block.visual === "data-flow" ? <DataFlowView onFeatureSelect={onFeatureSelect} selectedFeatureId={selectedFeatureId} /> : null}
      {block.visual === "document-map" ? <DocumentMindMap activeRouteId={activeRouteId} onRouteChange={onRouteChange} tree={tree} /> : null}
      {block.visual === "mvp" ? <MvpFlow /> : null}
      {block.visual === "decisions" ? <DecisionBoard decisions={decisions} risks={risks} /> : null}
      {block.visual === "roadmap" ? <Roadmap onFeatureSelect={onFeatureSelect} selectedFeatureId={selectedFeatureId} /> : null}
    </section>
  );
}
