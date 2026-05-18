import type { CSSProperties } from "react";
import type { DocumentTreeNode, DocumentTreeSection } from "../data/routes";
import { useMeasuredMindMapCurves } from "./mind-map/useMeasuredMindMapCurves";
import { usePanZoomViewport } from "./mind-map/usePanZoomViewport";

type DocumentMindMapProps = {
  activeRouteId: string;
  tree: DocumentTreeSection[];
  onRouteChange: (routeId: string) => void;
};

export function DocumentMindMap({ activeRouteId, tree, onRouteChange }: DocumentMindMapProps) {
  const panZoom = usePanZoomViewport();
  const measuredCurves = useMeasuredMindMapCurves();

  return (
    <div className={panZoom.isPanning ? "document-map is-panning" : "document-map"} aria-label="文档树思维导图">
      <div
        ref={panZoom.viewportRef}
        className="document-map-viewport"
        onPointerCancel={panZoom.handlePointerCancel}
        onPointerDown={panZoom.handlePointerDown}
        onPointerMove={panZoom.handlePointerMove}
        onPointerUp={panZoom.handlePointerUp}
      >
        <div
          ref={measuredCurves.canvasRef}
          className="document-map-canvas"
          style={{ transform: `translate3d(${panZoom.transform.x}px, calc(-50% + ${panZoom.transform.y}px), 0) scale(${panZoom.transform.scale})` }}
        >
          <svg className="document-map-measured-curves" aria-hidden="true" focusable="false">
            {measuredCurves.curves.map((curve) => (
              <path d={curve.path} key={curve.id} style={{ "--curve-accent": curve.accent } as CSSProperties} />
            ))}
          </svg>

          <div className="document-map-root" ref={measuredCurves.rootRef}>
            <span>主页</span>
            <strong>文档树总览</strong>
            <small>从左到右生长</small>
          </div>

          <div className="document-map-branches">
            {tree.map((section, index) => (
              <section className="document-map-section" key={section.id} style={{ "--branch-index": index } as CSSProperties}>
                <div className="document-map-section-node" ref={measuredCurves.registerSection(section.id)}>
                  <span>{String(index + 1).padStart(2, "0")}</span>
                  <strong>{section.label}</strong>
                  <small>{section.summary}</small>
                </div>

                <div className="document-map-children">
                  {section.children.map((node) => (
                    <DocumentMapNode
                      activeRouteId={activeRouteId}
                      key={node.id}
                      node={node}
                      onRouteChange={onRouteChange}
                      registerLeaf={measuredCurves.registerLeaf}
                      sectionId={section.id}
                    />
                  ))}
                </div>
              </section>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function DocumentMapNode({
  activeRouteId,
  node,
  onRouteChange,
  registerLeaf,
  sectionId,
}: {
  activeRouteId: string;
  node: DocumentTreeNode;
  onRouteChange: (routeId: string) => void;
  registerLeaf: (id: string) => (element: HTMLElement | null) => void;
  sectionId: string;
}) {
  if (node.kind === "route") {
    const isActive = node.routeId === activeRouteId;

    return (
      <button
        ref={registerLeaf(`${sectionId}:${node.id}`)}
        type="button"
        className={isActive ? "document-map-leaf active" : "document-map-leaf"}
        aria-current={isActive ? "page" : undefined}
        onClick={() => onRouteChange(node.routeId)}
      >
        <strong>{node.label}</strong>
        {node.summary ? <small>{node.summary}</small> : null}
      </button>
    );
  }

  return (
    <div className="document-map-nested-branch">
      <strong>{node.label}</strong>
      {node.summary ? <small>{node.summary}</small> : null}
      <div>
        {node.children.map((child) => (
          <DocumentMapNode activeRouteId={activeRouteId} key={child.id} node={child} onRouteChange={onRouteChange} registerLeaf={registerLeaf} sectionId={sectionId} />
        ))}
      </div>
    </div>
  );
}
