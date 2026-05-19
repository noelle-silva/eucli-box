import { useEffect, useMemo, useState } from "react";
import { Sidebar } from "./components/layout/Sidebar";
import { RoutePage } from "./components/layout/RoutePage";
import { hasAtomicFeature } from "./data/projectModel";
import { defaultRouteId, designRoutes, documentTree, getDesignRoute, hasDesignRoute } from "./data/routes";

function readInitialRouteId() {
  const routeId = readHashState().routeId;
  return hasDesignRoute(routeId) ? routeId : defaultRouteId;
}

function readInitialFeatureId() {
  const featureId = readHashState().featureId;

  if (!featureId) {
    return undefined;
  }

  if (!hasAtomicFeature(featureId)) {
    throw new Error(`URL references missing atomic feature: ${featureId}`);
  }

  return featureId;
}

function readHashState() {
  const [routeId = "", query = ""] = window.location.hash.replace("#", "").split("?");
  const params = new URLSearchParams(query);

  return {
    routeId,
    featureId: params.get("feature") ?? undefined,
  };
}

function writeHashState(routeId: string, featureId?: string) {
  const params = new URLSearchParams();

  if (featureId) {
    params.set("feature", featureId);
  }

  const query = params.toString();
  window.location.hash = query ? `${routeId}?${query}` : routeId;
}

export function App() {
  const [activeRouteId, setActiveRouteId] = useState(readInitialRouteId);
  const [selectedFeatureId, setSelectedFeatureId] = useState<string | undefined>(readInitialFeatureId);

  useEffect(() => {
    function handleHashChange() {
      setActiveRouteId(readInitialRouteId());
      setSelectedFeatureId(readInitialFeatureId());
    }

    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, []);

  const activeRoute = useMemo(
    () => getDesignRoute(activeRouteId),
    [activeRouteId],
  );
  function handleRouteChange(routeId: string) {
    writeHashState(routeId, selectedFeatureId);
    setActiveRouteId(routeId);
  }

  function handleFeatureSelect(featureId: string) {
    if (!hasAtomicFeature(featureId)) {
      throw new Error(`Cannot select missing atomic feature: ${featureId}`);
    }

    writeHashState(activeRoute.id, featureId);
    setSelectedFeatureId(featureId);
  }

  return (
    <div className="atlas-app-shell">
      <Sidebar routes={designRoutes} tree={documentTree} activeRouteId={activeRoute.id} onRouteChange={handleRouteChange} />
      <main className="atlas-route-shell">
        <RoutePage
          activeRouteId={activeRoute.id}
          onFeatureSelect={handleFeatureSelect}
          onRouteChange={handleRouteChange}
          route={activeRoute}
          selectedFeatureId={selectedFeatureId}
          tree={documentTree}
        />
      </main>
    </div>
  );
}
