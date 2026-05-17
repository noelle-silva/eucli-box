import type { DesignRoute } from "../../data/designRoutes";

type SidebarProps = {
  routes: DesignRoute[];
  groups: Array<{ id: string; label: string }>;
  activeRouteId: string;
  onRouteChange: (routeId: string) => void;
};

export function Sidebar({ routes, groups, activeRouteId, onRouteChange }: SidebarProps) {
  return (
    <aside className="atlas-sidebar" aria-label="design atlas navigation">
      <div className="sidebar-brand">
        <span>eu</span>
        <div>
          <strong>Design Atlas</strong>
          <small>项目构想导航</small>
        </div>
      </div>

      <nav className="sidebar-nav">
        {groups.map((group) => (
          <section className="sidebar-group" key={group.id}>
            <h2>{group.label}</h2>
            {routes
              .filter((route) => route.group === group.id)
              .map((route) => (
                <button
                  type="button"
                  className={route.id === activeRouteId ? "sidebar-link active" : "sidebar-link"}
                  key={route.id}
                  onClick={() => onRouteChange(route.id)}
                >
                  <span>{route.label}</span>
                  <small>{route.eyebrow}</small>
                </button>
              ))}
          </section>
        ))}
      </nav>
    </aside>
  );
}
