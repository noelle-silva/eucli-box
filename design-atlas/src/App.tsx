import { useEffect, useMemo, useState } from "react";
import { Sidebar } from "./components/layout/Sidebar";
import { RoutePage } from "./components/layout/RoutePage";
import { defaultRouteId, designRoutes, documentTree, getDesignRoute, hasDesignRoute } from "./data/routes";

function readInitialRouteId() {
  const routeId = window.location.hash.replace("#", "");
  return hasDesignRoute(routeId) ? routeId : defaultRouteId;
}

export function App() {
  const [activeRouteId, setActiveRouteId] = useState(readInitialRouteId);

  useEffect(() => {
    function handleHashChange() {
      setActiveRouteId(readInitialRouteId());
    }

    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, []);

  const activeRoute = useMemo(
    () => getDesignRoute(activeRouteId),
    [activeRouteId],
  );

  function handleRouteChange(routeId: string) {
    window.location.hash = routeId;
    setActiveRouteId(routeId);
  }

  return (
    <div className="atlas-app-shell">
      <Sidebar routes={designRoutes} tree={documentTree} activeRouteId={activeRoute.id} onRouteChange={handleRouteChange} />
      <main className="atlas-route-shell">
        <RoutePage route={activeRoute} />
      </main>
    </div>
  );
}
