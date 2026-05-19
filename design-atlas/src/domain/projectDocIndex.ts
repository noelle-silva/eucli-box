import rawProjectDoc from "../data/project-doc.json";

export type PageId = "atomic-features" | "architecture" | "data-flow" | "roadmap" | "records";
export type FeatureStage = "mvp" | "alpha" | "beta" | "future";

export type NavigationItem = {
  id: PageId;
  label: string;
};

export type ProjectDomain = {
  id: string;
  name: string;
  summary: string;
};

export type ProjectModule = {
  id: string;
  domainId: string;
  name: string;
  summary: string;
};

export type AtomicFeature = {
  id: string;
  title: string;
  domainId: string;
  moduleId: string;
  stage: FeatureStage;
  intent: string;
  description: string;
  acceptance: string;
  relations: {
    dependsOn: string[];
    supports: string[];
  };
  signals: {
    inputs: string[];
    outputs: string[];
    events: string[];
  };
};

export type AtomicFeatureView = AtomicFeature & {
  domain: ProjectDomain;
  module: ProjectModule;
  dependencies: AtomicFeature[];
  supportedFeatures: AtomicFeature[];
  dependentFeatures: AtomicFeature[];
  relationGraph: AtomicFeatureRelationGraph;
};

export type AtomicFeatureRelationNode = {
  id: string;
  title: string;
  stage: FeatureStage;
};

export type AtomicFeatureRelationGraph = {
  center: AtomicFeatureRelationNode;
  dependencies: AtomicFeatureRelationNode[];
  dependents: AtomicFeatureRelationNode[];
};

type ProjectDoc = {
  meta: {
    title: string;
    subtitle: string;
    version: string;
  };
  navigation: NavigationItem[];
  indexes: {
    domains: string[];
    modules: string[];
    features: string[];
  };
  domains: Record<string, ProjectDomain>;
  modules: Record<string, ProjectModule>;
  features: Record<string, AtomicFeature>;
};

const projectDoc = rawProjectDoc as ProjectDoc;

validateProjectDoc(projectDoc);

export const projectMeta = projectDoc.meta;
export const navigationItems = projectDoc.navigation;
export const domainIndex = projectDoc.indexes.domains.map((id) => requireRecord(projectDoc.domains, id, "domain index item"));
export const moduleIndex = projectDoc.indexes.modules.map((id) => requireRecord(projectDoc.modules, id, "module index item"));
export const atomicFeatureIndex = projectDoc.indexes.features.map((id) => requireRecord(projectDoc.features, id, "feature index item"));

export const stageLabels: Record<FeatureStage, string> = {
  mvp: "MVP",
  alpha: "Alpha",
  beta: "Beta",
  future: "Future",
};

export function getAtomicFeatureView(featureId: string): AtomicFeatureView {
  const feature = getFeature(featureId);
  const dependencies = feature.relations.dependsOn.map(getFeature);
  const supportedFeatures = feature.relations.supports.map(getFeature);
  const dependentFeatures = atomicFeatureIndex.filter((candidate) => candidate.relations.dependsOn.includes(feature.id));

  return {
    ...feature,
    domain: getDomain(feature.domainId),
    module: getModule(feature.moduleId),
    dependencies,
    supportedFeatures,
    dependentFeatures,
    relationGraph: createRelationGraph(feature, dependencies, dependentFeatures),
  };
}

function createRelationGraph(feature: AtomicFeature, dependencies: AtomicFeature[], dependentFeatures: AtomicFeature[]): AtomicFeatureRelationGraph {
  return {
    center: toRelationNode(feature),
    dependencies: dependencies.map(toRelationNode),
    dependents: dependentFeatures.map(toRelationNode),
  };
}

function toRelationNode(feature: AtomicFeature): AtomicFeatureRelationNode {
  return {
    id: feature.id,
    title: feature.title,
    stage: feature.stage,
  };
}

export function getFeature(featureId: string): AtomicFeature {
  return requireRecord(projectDoc.features, featureId, "atomic feature");
}

function getDomain(domainId: string): ProjectDomain {
  return requireRecord(projectDoc.domains, domainId, "project domain");
}

function getModule(moduleId: string): ProjectModule {
  return requireRecord(projectDoc.modules, moduleId, "project module");
}

function validateProjectDoc(doc: ProjectDoc) {
  assertUnique(doc.indexes.domains, "domain index");
  assertUnique(doc.indexes.modules, "module index");
  assertUnique(doc.indexes.features, "feature index");

  for (const domainId of doc.indexes.domains) {
    requireRecord(doc.domains, domainId, "domain");
  }

  for (const moduleId of doc.indexes.modules) {
    const projectModule = requireRecord(doc.modules, moduleId, "module");
    requireRecord(doc.domains, projectModule.domainId, `module domain for ${moduleId}`);
  }

  for (const featureId of doc.indexes.features) {
    const feature = requireRecord(doc.features, featureId, "feature");
    requireText(feature.intent, `feature intent for ${featureId}`);
    requireText(feature.description, `feature description for ${featureId}`);
    requireMinLength(feature.description, 80, `feature description for ${featureId}`);
    requireText(feature.acceptance, `feature acceptance for ${featureId}`);
    requireRecord(doc.domains, feature.domainId, `feature domain for ${featureId}`);
    const projectModule = requireRecord(doc.modules, feature.moduleId, `feature module for ${featureId}`);

    if (projectModule.domainId !== feature.domainId) {
      throw new Error(`Feature domain/module mismatch: ${featureId}`);
    }

    for (const dependencyId of feature.relations.dependsOn) {
      requireRecord(doc.features, dependencyId, `feature dependency for ${featureId}`);
    }

    for (const supportedId of feature.relations.supports) {
      requireRecord(doc.features, supportedId, `supported feature for ${featureId}`);
    }
  }
}

function requireText(value: string, label: string) {
  if (typeof value !== "string" || value.trim().length === 0) {
    throw new Error(`${label} must not be empty`);
  }
}

function requireMinLength(value: string, minLength: number, label: string) {
  if (value.trim().length < minLength) {
    throw new Error(`${label} must be at least ${minLength} characters`);
  }
}

function assertUnique(ids: string[], label: string) {
  const seen = new Set<string>();

  for (const id of ids) {
    if (id.trim().length === 0) {
      throw new Error(`${label} contains empty id`);
    }

    if (seen.has(id)) {
      throw new Error(`${label} contains duplicate id: ${id}`);
    }

    seen.add(id);
  }
}

function requireRecord<T>(records: Record<string, T>, id: string, label: string): T {
  const record = records[id];

  if (!record) {
    throw new Error(`Missing ${label}: ${id}`);
  }

  return record;
}
