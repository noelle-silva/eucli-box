import type { DesignRoute, DocumentTreeNode, DocumentTreeSection } from "../../data/routes";

type SidebarProps = {
  routes: DesignRoute[];
  tree: DocumentTreeSection[];
  activeRouteId: string;
  onRouteChange: (routeId: string) => void;
};

export function Sidebar({ routes, tree, activeRouteId, onRouteChange }: SidebarProps) {
  const routeById = new Map(routes.map((route) => [route.id, route]));

  return (
    <aside className="atlas-sidebar" aria-label="design atlas navigation">
      <div className="sidebar-brand">
        <span>eu</span>
        <div>
          <strong>Web Docs</strong>
          <small>结构化文档树</small>
        </div>
      </div>

      <nav className="sidebar-nav">
        {tree.map((section) => (
          <section className="sidebar-group" key={section.id}>
            <h2>{section.label}</h2>
            <p>{section.summary}</p>
            {section.children.map((node) => (
              <SidebarNode activeRouteId={activeRouteId} depth={0} key={node.id} node={node} onRouteChange={onRouteChange} routeById={routeById} />
            ))}
          </section>
        ))}
      </nav>
    </aside>
  );
}

function SidebarNode({
  activeRouteId,
  depth,
  node,
  onRouteChange,
  routeById,
}: {
  activeRouteId: string;
  depth: number;
  node: DocumentTreeNode;
  onRouteChange: (routeId: string) => void;
  routeById: Map<string, DesignRoute>;
}) {
  if (node.kind === "route") {
    const route = routeById.get(node.routeId)!;

    return (
      <button
        type="button"
        className={route.id === activeRouteId ? "sidebar-link active" : "sidebar-link"}
        data-depth={depth}
        onClick={() => onRouteChange(node.routeId)}
      >
        <span>{node.label}</span>
        <small>{route.eyebrow}</small>
      </button>
    );
  }

  return (
    <div className="sidebar-branch" data-depth={depth}>
      <strong>{node.label}</strong>
      {node.summary ? <small>{node.summary}</small> : null}
      {node.children.map((child) => (
        <SidebarNode activeRouteId={activeRouteId} depth={depth + 1} key={child.id} node={child} onRouteChange={onRouteChange} routeById={routeById} />
      ))}
    </div>
  );
}
