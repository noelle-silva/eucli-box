import type { CSSProperties } from "react";
import type { FeatureBranchNode, FeatureLeafNode, FeatureNode } from "../data/featureChecklist";
import { featureChecklistTotals, featureDeliveryStageLabels, getFeatureCount } from "../data/featureChecklist";
import { priorityDisplayLabels } from "../data/priorities";
import { SpotlightPanel } from "./visual-system";

type FeatureChecklistProps = {
  tree: FeatureBranchNode[];
};

export function FeatureChecklist({ tree }: FeatureChecklistProps) {
  return (
    <div className="feature-checklist">
      <header className="feature-checklist-summary">
        <div>
          <span className="diagram-kicker">Feature Tree</span>
          <h2>按系统结构组织的功能核对树</h2>
          <p>主结构按领域、模块、功能项逐层展开，P0/P1/P2 只是功能项标签。后续实现时沿着树核对，不再被优先级分类打散上下文。</p>
        </div>
        <div className="feature-checklist-metrics" aria-label="feature tree totals">
          <Metric label="功能项" value={featureChecklistTotals.total} />
          <Metric label="P0" value={featureChecklistTotals.byPriority.P0} />
          <Metric label="P1" value={featureChecklistTotals.byPriority.P1} />
          <Metric label="P2" value={featureChecklistTotals.byPriority.P2} />
        </div>
      </header>

      <div className="feature-checklist-status-row" aria-label="delivery stage totals">
        {Object.entries(featureChecklistTotals.byStage).map(([stage, total]) => (
          <span key={stage}>{featureDeliveryStageLabels[stage as keyof typeof featureDeliveryStageLabels]} · {total}</span>
        ))}
      </div>

      <div className="feature-tree-root">
        {tree.map((node) => (
          <FeatureBranch branch={node} depth={0} key={node.id} />
        ))}
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="feature-checklist-metric">
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

function FeatureBranch({ branch, depth }: { branch: FeatureBranchNode; depth: number }) {
  const accent = branch.accent ?? "#7c3aed";

  return (
    <section className={`feature-tree-branch depth-${Math.min(depth, 2)}`} style={{ "--branch-accent": accent } as CSSProperties}>
      <div className="feature-tree-branch-head">
        <div>
          <span>{depth === 0 ? "Domain" : "Module"}</span>
          <h3>{branch.title}</h3>
          <p>{branch.summary}</p>
        </div>
        <strong>{getFeatureCount(branch)} 项</strong>
      </div>

      <div className="feature-tree-children">
        {branch.children.map((node) => (
          <FeatureNodeView node={node} depth={depth + 1} key={node.id} />
        ))}
      </div>
    </section>
  );
}

function FeatureNodeView({ node, depth }: { node: FeatureNode; depth: number }) {
  if (node.kind === "branch") {
    return <FeatureBranch branch={node} depth={depth} />;
  }

  return <FeatureLeaf feature={node} />;
}

function FeatureLeaf({ feature }: { feature: FeatureLeafNode }) {
  return (
    <SpotlightPanel tag="article" className={`feature-tree-leaf ${feature.priority.toLowerCase()} glass-lift`} accent="rgba(124, 58, 237, 0.12)">
      <div className="feature-tree-leaf-topline">
        <span>{feature.priority}</span>
        <small>{featureDeliveryStageLabels[feature.stage]}</small>
      </div>
      <h4>{feature.title}</h4>
      <p>{feature.description}</p>
      <div className="feature-tree-meta-row">
        <span>{priorityDisplayLabels[feature.priority]}</span>
      </div>
      <div className="feature-checklist-acceptance">
        <strong>验收口径</strong>
        <span>{feature.acceptance}</span>
      </div>
    </SpotlightPanel>
  );
}
