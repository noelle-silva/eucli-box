import type { AtomicFeatureRelationGraph, AtomicFeatureRelationNode } from "../domain/projectDocIndex";
import { stageLabels } from "../domain/projectDocIndex";

type FeatureRelationGraphProps = {
  graph: AtomicFeatureRelationGraph;
  onFeatureOpen: (featureId: string) => void;
};

const graphHeight = 360;
const graphWidth = 980;
const leftX = 160;
const centerX = 490;
const rightX = 820;
const nodeWidth = 230;
const nodeHeight = 62;

export function FeatureRelationGraph({ graph, onFeatureOpen }: FeatureRelationGraphProps) {
  const dependencyPoints = createNodePoints(graph.dependencies, leftX);
  const dependentPoints = createNodePoints(graph.dependents, rightX);
  const centerPoint = { x: centerX, y: graphHeight / 2 };

  return (
    <section className="relation-graph" aria-label={`${graph.center.title} 依赖关系图`}>
      <div className="relation-graph-labels" aria-hidden="true">
        <span>依赖</span>
        <span>当前功能</span>
        <span>被依赖</span>
      </div>

      <div className="relation-graph-canvas">
        <svg className="relation-graph-lines" viewBox={`0 0 ${graphWidth} ${graphHeight}`} role="img" aria-label="原子功能依赖与被依赖的有向贝塞尔曲线图">
          <defs>
            <marker id="relation-arrow" markerHeight="8" markerWidth="8" orient="auto" refX="7" refY="4">
              <path d="M0,0 L8,4 L0,8 Z" fill="#8fa78b" />
            </marker>
          </defs>

          {dependencyPoints.map((point) => (
            <path d={createCurve(point.x + nodeWidth / 2, point.y, centerPoint.x - nodeWidth / 2, centerPoint.y)} key={`dependency-${point.node.id}`} markerEnd="url(#relation-arrow)" />
          ))}

          {dependentPoints.map((point) => (
            <path d={createCurve(centerPoint.x + nodeWidth / 2, centerPoint.y, point.x - nodeWidth / 2, point.y)} key={`dependent-${point.node.id}`} markerEnd="url(#relation-arrow)" />
          ))}
        </svg>

        <div className="relation-node-column relation-node-column-left">
          {dependencyPoints.length ? dependencyPoints.map((point) => <RelationNode key={point.node.id} node={point.node} onFeatureOpen={onFeatureOpen} />) : <EmptyNode label="暂无依赖" />}
        </div>

        <div className="relation-node-center">
          <RelationNode node={graph.center} isCenter onFeatureOpen={onFeatureOpen} />
        </div>

        <div className="relation-node-column relation-node-column-right">
          {dependentPoints.length ? dependentPoints.map((point) => <RelationNode key={point.node.id} node={point.node} onFeatureOpen={onFeatureOpen} />) : <EmptyNode label="暂无被依赖" />}
        </div>
      </div>
    </section>
  );
}

function RelationNode({ isCenter = false, node, onFeatureOpen }: { isCenter?: boolean; node: AtomicFeatureRelationNode; onFeatureOpen: (featureId: string) => void }) {
  return (
    <button className={isCenter ? "relation-node relation-node-current" : "relation-node"} type="button" onClick={() => onFeatureOpen(node.id)}>
      <strong>{node.title}</strong>
      <span>{stageLabels[node.stage]}</span>
    </button>
  );
}

function EmptyNode({ label }: { label: string }) {
  return <div className="relation-node relation-node-empty">{label}</div>;
}

function createNodePoints(nodes: AtomicFeatureRelationNode[], x: number) {
  if (nodes.length === 0) {
    return [];
  }

  const gap = graphHeight / (nodes.length + 1);
  return nodes.map((node, index) => ({ node, x, y: gap * (index + 1) }));
}

function createCurve(startX: number, startY: number, endX: number, endY: number) {
  const controlOffset = Math.max(120, Math.abs(endX - startX) * 0.46);
  return `M ${startX} ${startY} C ${startX + controlOffset} ${startY}, ${endX - controlOffset} ${endY}, ${endX} ${endY}`;
}
