import { architectureRoute } from "./architecture";
import { changelogRoute } from "./changelog";
import { dataFlowRoute } from "./dataFlow";
import { dataModelRoute } from "./dataModel";
import { decisionsRoute } from "./decisions";
import { featuresRoute } from "./features";
import { implementationRoadmapRoute } from "./implementationRoadmap";
import { openQuestionsRoute } from "./openQuestions";
import { stateMachinesRoute } from "./stateMachines";
import { uiPrototypeRoute } from "./uiPrototype";
import { userJourneysRoute } from "./userJourneys";
import { documentTree } from "./documentTree";
import type { DesignRoute, DocumentTreeNode, DocumentTreeSection } from "./types";

export type { DesignRoute, DocumentTreeNode, DocumentTreeSection, RouteBlock } from "./types";
export { documentTree } from "./documentTree";

export const designRoutes: DesignRoute[] = [
  featuresRoute,
  architectureRoute,
  dataFlowRoute,
  dataModelRoute,
  uiPrototypeRoute,
  userJourneysRoute,
  stateMachinesRoute,
  decisionsRoute,
  implementationRoadmapRoute,
  openQuestionsRoute,
  changelogRoute,
];

export const defaultRouteId = "features";

const routeById = new Map<string, DesignRoute>();

for (const route of designRoutes) {
  if (routeById.has(route.id)) {
    throw new Error(`Duplicate design route id: ${route.id}`);
  }

  routeById.set(route.id, route);
}

const documentRouteIds = collectDocumentRouteIds(documentTree);

for (const routeId of documentRouteIds) {
  if (!routeById.has(routeId)) {
    throw new Error(`Document tree references missing route: ${routeId}`);
  }
}

for (const route of designRoutes) {
  if (!documentRouteIds.has(route.id)) {
    throw new Error(`Design route is not mounted in document tree: ${route.id}`);
  }
}

if (!routeById.has(defaultRouteId)) {
  throw new Error(`Default route does not exist: ${defaultRouteId}`);
}

export function hasDesignRoute(routeId: string) {
  return routeById.has(routeId);
}

export function getDesignRoute(routeId: string) {
  const route = routeById.get(routeId);

  if (!route) {
    throw new Error(`Unknown design route: ${routeId}`);
  }

  return route;
}

function collectDocumentRouteIds(sections: DocumentTreeSection[]) {
  const routeIds = new Set<string>();

  for (const section of sections) {
    collectNodeRouteIds(section.children, routeIds);
  }

  return routeIds;
}

function collectNodeRouteIds(nodes: DocumentTreeNode[], routeIds: Set<string>) {
  for (const node of nodes) {
    if (node.kind === "route") {
      if (routeIds.has(node.routeId)) {
        throw new Error(`Document tree mounts route more than once: ${node.routeId}`);
      }

      routeIds.add(node.routeId);
    }

    if (node.kind === "branch") {
      collectNodeRouteIds(node.children, routeIds);
    }
  }
}
