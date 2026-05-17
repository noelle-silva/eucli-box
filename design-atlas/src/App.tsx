import { useEffect, useMemo, useState } from "react";
import { Sidebar } from "./components/layout/Sidebar";
import { RoutePage } from "./components/layout/RoutePage";
import { defaultRouteId, designRoutes, routeGroups } from "./data/designRoutes";

function readInitialRouteId() {
  const routeId = window.location.hash.replace("#", "");
  return designRoutes.some((route) => route.id === routeId) ? routeId : defaultRouteId;
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
    () => designRoutes.find((route) => route.id === activeRouteId) ?? designRoutes[0],
    [activeRouteId],
  );

  function handleRouteChange(routeId: string) {
    window.location.hash = routeId;
    setActiveRouteId(routeId);
  }

  return (
    <div className="atlas-app-shell">
      <Sidebar routes={designRoutes} groups={routeGroups} activeRouteId={activeRoute.id} onRouteChange={handleRouteChange} />
      <main className="atlas-route-shell">
        <RoutePage route={activeRoute} />
      </main>
    </div>
  );
}
