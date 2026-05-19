import { useId, useLayoutEffect, useRef, useState } from "react";
import type { AtomicFeatureRelationGraph, AtomicFeatureRelationNode } from "../domain/projectDocIndex";
import { stageLabels } from "../domain/projectDocIndex";

type FeatureRelationGraphProps = {
  graph: AtomicFeatureRelationGraph;
  onFeatureOpen: (featureId: string) => void;
};

type RelationSide = "dependency" | "dependent";

type AnchorRect = {
  left: number;
  right: number;
  verticalCenter: number;
};

type RelationPath = {
  id: string;
  d: string;
};

type CanvasSize = {
  width: number;
  height: number;
};

const centerNodeKey = "center";

export function FeatureRelationGraph({ graph, onFeatureOpen }: FeatureRelationGraphProps) {
  const arrowMarkerId = `relation-arrow-${useId().replace(/:/g, "")}`;
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const nodeElements = useRef(new Map<string, HTMLElement>());
  const [relationPaths, setRelationPaths] = useState<RelationPath[]>([]);
  const [canvasSize, setCanvasSize] = useState<CanvasSize>({ width: 0, height: 0 });

  useLayoutEffect(() => {
    const canvas = canvasRef.current;

    if (!canvas) {
      return;
    }

    let animationFrame = 0;

    const measureRelations = () => {
      const canvasRect = canvas.getBoundingClientRect();
      const paths = createRelationPaths(graph, canvas, nodeElements.current);

      setCanvasSize((currentSize) => (areCanvasSizesEqual(currentSize, canvasRect) ? currentSize : { width: canvasRect.width, height: canvasRect.height }));
      setRelationPaths((currentPaths) => (areRelationPathsEqual(currentPaths, paths) ? currentPaths : paths));
    };

    const scheduleMeasure = () => {
      cancelAnimationFrame(animationFrame);
      animationFrame = requestAnimationFrame(measureRelations);
    };

    const resizeObserver = new ResizeObserver(scheduleMeasure);
    resizeObserver.observe(canvas);
    nodeElements.current.forEach((element) => resizeObserver.observe(element));
    scheduleMeasure();

    return () => {
      cancelAnimationFrame(animationFrame);
      resizeObserver.disconnect();
    };
  }, [graph]);

  const setNodeElement = (key: string, element: HTMLElement | null) => {
    if (element) {
      nodeElements.current.set(key, element);
      return;
    }

    nodeElements.current.delete(key);
  };

  return (
    <section className="relation-graph" aria-label={`${graph.center.title} 依赖关系图`}>
      <div className="relation-graph-labels" aria-hidden="true">
        <span>依赖</span>
        <span>当前功能</span>
        <span>被依赖</span>
      </div>

      <div className="relation-graph-canvas" ref={canvasRef}>
        <svg className="relation-graph-lines" viewBox={`0 0 ${canvasSize.width} ${canvasSize.height}`} aria-hidden="true" focusable="false">
          <defs>
            <marker id={arrowMarkerId} markerHeight="10" markerWidth="10" orient="auto" refX="10" refY="5">
              <path className="relation-arrow-head" d="M0,0 L10,5 L0,10 Z" />
            </marker>
          </defs>

          {relationPaths.map((path) => (
            <path className="relation-curve" d={path.d} key={path.id} markerEnd={`url(#${arrowMarkerId})`} />
          ))}
        </svg>

        <div className="relation-node-column relation-node-column-left">
          {graph.dependencies.length ? graph.dependencies.map((node) => <RelationNode key={node.id} node={node} nodeRef={(element) => setNodeElement(createNodeKey("dependency", node.id), element)} onFeatureOpen={onFeatureOpen} />) : <EmptyNode label="暂无依赖" />}
        </div>

        <div className="relation-node-center">
          <RelationNode node={graph.center} isCenter nodeRef={(element) => setNodeElement(centerNodeKey, element)} onFeatureOpen={onFeatureOpen} />
        </div>

        <div className="relation-node-column relation-node-column-right">
          {graph.dependents.length ? graph.dependents.map((node) => <RelationNode key={node.id} node={node} nodeRef={(element) => setNodeElement(createNodeKey("dependent", node.id), element)} onFeatureOpen={onFeatureOpen} />) : <EmptyNode label="暂无被依赖" />}
        </div>
      </div>
    </section>
  );
}

function RelationNode({ isCenter = false, node, nodeRef, onFeatureOpen }: { isCenter?: boolean; node: AtomicFeatureRelationNode; nodeRef?: (element: HTMLButtonElement | null) => void; onFeatureOpen: (featureId: string) => void }) {
  return (
    <button className={isCenter ? "relation-node relation-node-current" : "relation-node"} ref={nodeRef} type="button" onClick={() => onFeatureOpen(node.id)}>
      <strong>{node.title}</strong>
      <span>{stageLabels[node.stage]}</span>
    </button>
  );
}

function EmptyNode({ label }: { label: string }) {
  return <div className="relation-node relation-node-empty">{label}</div>;
}

function createRelationPaths(graph: AtomicFeatureRelationGraph, canvas: HTMLElement, elements: Map<string, HTMLElement>) {
  const center = getAnchorRect(elements.get(centerNodeKey), canvas);

  if (!center) {
    return [];
  }

  return [
    ...graph.dependencies.flatMap((node) => createRelationPath(`dependency-${node.id}`, elements.get(createNodeKey("dependency", node.id)), center, canvas, "dependency")),
    ...graph.dependents.flatMap((node) => createRelationPath(`dependent-${node.id}`, elements.get(createNodeKey("dependent", node.id)), center, canvas, "dependent")),
  ];
}

function createRelationPath(id: string, nodeElement: HTMLElement | undefined, center: AnchorRect, canvas: HTMLElement, side: RelationSide): RelationPath[] {
  const node = getAnchorRect(nodeElement, canvas);

  if (!node) {
    return [];
  }

  const start = side === "dependency" ? { x: node.right, y: node.verticalCenter } : { x: center.right, y: center.verticalCenter };
  const end = side === "dependency" ? { x: center.left, y: center.verticalCenter } : { x: node.left, y: node.verticalCenter };

  return [{ id, d: createCurve(start, end) }];
}

function getAnchorRect(element: HTMLElement | undefined, canvas: HTMLElement): AnchorRect | null {
  if (!element) {
    return null;
  }

  const canvasRect = canvas.getBoundingClientRect();
  const nodeRect = element.getBoundingClientRect();

  return {
    left: nodeRect.left - canvasRect.left,
    right: nodeRect.right - canvasRect.left,
    verticalCenter: nodeRect.top - canvasRect.top + nodeRect.height / 2,
  };
}

function createNodeKey(side: RelationSide, nodeId: string) {
  return `${side}:${nodeId}`;
}

function createCurve(start: { x: number; y: number }, end: { x: number; y: number }) {
  const direction = end.x >= start.x ? 1 : -1;
  const controlOffset = clamp(Math.abs(end.x - start.x) * 0.42, 72, 220);

  return `M ${start.x} ${start.y} C ${start.x + controlOffset * direction} ${start.y}, ${end.x - controlOffset * direction} ${end.y}, ${end.x} ${end.y}`;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function areRelationPathsEqual(currentPaths: RelationPath[], nextPaths: RelationPath[]) {
  return currentPaths.length === nextPaths.length && currentPaths.every((path, index) => path.id === nextPaths[index].id && path.d === nextPaths[index].d);
}

function areCanvasSizesEqual(currentSize: CanvasSize, nextRect: DOMRect) {
  return currentSize.width === nextRect.width && currentSize.height === nextRect.height;
}
