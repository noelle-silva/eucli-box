import { useMemo, useState } from "react";
import type { FeatureBranchNode, FeatureNode } from "../../data/featureChecklist";

export type FeatureTreeExpansion = {
  allBranchIds: string[];
  expandedCount: number;
  isExpanded: (branchId: string) => boolean;
  toggleBranch: (branchId: string) => void;
  expandAll: () => void;
  collapseAll: () => void;
};

export function useFeatureTreeExpansion(tree: FeatureBranchNode[]): FeatureTreeExpansion {
  const allBranchIds = useMemo(() => collectBranchIds(tree), [tree]);
  const initialExpandedIds = useMemo(() => tree.map((branch) => branch.id), [tree]);
  const [expandedIds, setExpandedIds] = useState(() => new Set(initialExpandedIds));

  return {
    allBranchIds,
    expandedCount: expandedIds.size,
    isExpanded: (branchId) => expandedIds.has(branchId),
    toggleBranch: (branchId) => {
      setExpandedIds((current) => {
        const next = new Set(current);

        if (next.has(branchId)) {
          next.delete(branchId);
        } else {
          next.add(branchId);
        }

        return next;
      });
    },
    expandAll: () => setExpandedIds(new Set(allBranchIds)),
    collapseAll: () => setExpandedIds(new Set()),
  };
}

function collectBranchIds(nodes: FeatureNode[]): string[] {
  return nodes.flatMap((node) => {
    if (node.kind === "feature") {
      return [];
    }

    return [node.id, ...collectBranchIds(node.children)];
  });
}
