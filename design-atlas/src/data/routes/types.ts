export type RouteBlock =
  | {
      kind: "text";
      title?: string;
      paragraphs: string[];
    }
  | {
      kind: "cards";
      title?: string;
      items: Array<{ title: string; body: string; meta?: string; accent?: string }>;
    }
  | {
      kind: "table";
      title?: string;
      columns: string[];
      rows: string[][];
    }
  | {
      kind: "code";
      title?: string;
      code: string;
    }
  | {
      kind: "mermaid";
      title?: string;
      chart: string;
    }
  | {
      kind: "timeline";
      title?: string;
      items: Array<{ label: string; title: string; body: string }>;
    }
  | {
      kind: "visual";
      title?: string;
      visual: "architecture" | "document-map" | "feature-tree" | "subsystems" | "mvp" | "decisions" | "roadmap";
    };

export type DesignRoute = {
  id: string;
  label: string;
  eyebrow: string;
  title: string;
  summary: string;
  blocks: RouteBlock[];
};

export type DocumentTreeNode = DocumentRouteNode | DocumentBranchNode;

export type DocumentRouteNode = {
  kind: "route";
  id: string;
  label: string;
  summary?: string;
  routeId: string;
};

export type DocumentBranchNode = {
  kind: "branch";
  id: string;
  label: string;
  summary?: string;
  children: DocumentTreeNode[];
};

export type DocumentTreeSection = {
  id: string;
  label: string;
  summary: string;
  children: DocumentTreeNode[];
};
