import rawProjectsCatalog from "../data/projects.json";

export type PageId = "atomic-features" | "architecture" | "data-flow" | "roadmap" | "records";
export type FeatureStage = "mvp" | "alpha" | "beta" | "future";

export type NavigationItem = {
  id: PageId;
  label: string;
};

export type ProjectCatalogEntry = {
  id: string;
  name: string;
  summary: string;
  repositoryLabel: string;
  documentFile: string;
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

export type ProjectDocIndex = {
  project: ProjectCatalogEntry;
  meta: ProjectDoc["meta"];
  navigationItems: NavigationItem[];
  domainIndex: ProjectDomain[];
  moduleIndex: ProjectModule[];
  atomicFeatureIndex: AtomicFeature[];
  getAtomicFeatureView: (featureId: string) => AtomicFeatureView;
  getFeature: (featureId: string) => AtomicFeature;
};

type ProjectsCatalog = {
  defaultProjectId: string;
  indexes: {
    projects: string[];
  };
  projects: Record<string, ProjectCatalogEntry>;
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

const projectDocumentLoaders = import.meta.glob("../data/projects/*.json", { import: "default" });

const projectsCatalog = rawProjectsCatalog as ProjectsCatalog;

validateProjectsCatalog(projectsCatalog, projectDocumentLoaders);

export const projectCatalogIndex = projectsCatalog.indexes.projects.map((id) => requireRecord(projectsCatalog.projects, id, "project index item"));
export const defaultProjectId = projectsCatalog.defaultProjectId;

export const stageLabels: Record<FeatureStage, string> = {
  mvp: "MVP",
  alpha: "Alpha",
  beta: "Beta",
  future: "Future",
};

export async function loadProjectDocIndex(projectId: string): Promise<ProjectDocIndex> {
  const project = requireRecord(projectsCatalog.projects, projectId, "project");
  const loader = requireRecord(projectDocumentLoaders, createProjectDocumentLoaderKey(project.documentFile), `project document loader for ${projectId}`);
  const projectDoc = (await loader()) as ProjectDoc;

  validateProjectDoc(projectDoc, projectId);

  const domainIndex = projectDoc.indexes.domains.map((id) => requireRecord(projectDoc.domains, id, `domain index item for ${projectId}`));
  const moduleIndex = projectDoc.indexes.modules.map((id) => requireRecord(projectDoc.modules, id, `module index item for ${projectId}`));
  const atomicFeatureIndex = projectDoc.indexes.features.map((id) => requireRecord(projectDoc.features, id, `feature index item for ${projectId}`));

  function getFeature(featureId: string): AtomicFeature {
    return requireRecord(projectDoc.features, featureId, `atomic feature in ${projectId}`);
  }

  function getDomain(domainId: string): ProjectDomain {
    return requireRecord(projectDoc.domains, domainId, `project domain in ${projectId}`);
  }

  function getModule(moduleId: string): ProjectModule {
    return requireRecord(projectDoc.modules, moduleId, `project module in ${projectId}`);
  }

  function getAtomicFeatureView(featureId: string): AtomicFeatureView {
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

  return {
    project,
    meta: projectDoc.meta,
    navigationItems: projectDoc.navigation,
    domainIndex,
    moduleIndex,
    atomicFeatureIndex,
    getAtomicFeatureView,
    getFeature,
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

function validateProjectsCatalog(catalog: ProjectsCatalog, documents: Record<string, () => Promise<unknown>>) {
  assertUnique(catalog.indexes.projects, "project index");
  requireRecord(catalog.projects, catalog.defaultProjectId, "default project");

  for (const projectId of catalog.indexes.projects) {
    const project = requireRecord(catalog.projects, projectId, "project");
    requireText(project.id, `project id for ${projectId}`);
    requireText(project.name, `project name for ${projectId}`);
    requireText(project.summary, `project summary for ${projectId}`);
    requireText(project.repositoryLabel, `project repository label for ${projectId}`);
    requireRecord(documents, createProjectDocumentLoaderKey(project.documentFile), `project document file for ${projectId}`);

    if (project.id !== projectId) {
      throw new Error(`Project id mismatch: ${projectId}`);
    }
  }
}

function createProjectDocumentLoaderKey(documentFile: ProjectCatalogEntry["documentFile"]) {
  if (!documentFile.startsWith("projects/") || !documentFile.endsWith(".json")) {
    throw new Error(`Project document file must match projects/*.json: ${documentFile}`);
  }

  return `../data/${documentFile}`;
}

function validateProjectDoc(doc: ProjectDoc, projectId: string) {
  assertUnique(doc.indexes.domains, `domain index for ${projectId}`);
  assertUnique(doc.indexes.modules, `module index for ${projectId}`);
  assertUnique(doc.indexes.features, `feature index for ${projectId}`);

  for (const domainId of doc.indexes.domains) {
    requireRecord(doc.domains, domainId, `domain in ${projectId}`);
  }

  for (const moduleId of doc.indexes.modules) {
    const projectModule = requireRecord(doc.modules, moduleId, `module in ${projectId}`);
    requireRecord(doc.domains, projectModule.domainId, `module domain for ${moduleId} in ${projectId}`);
  }

  for (const featureId of doc.indexes.features) {
    const feature = requireRecord(doc.features, featureId, `feature in ${projectId}`);
    requireText(feature.intent, `feature intent for ${featureId} in ${projectId}`);
    requireText(feature.description, `feature description for ${featureId} in ${projectId}`);
    requireMinLength(feature.description, 80, `feature description for ${featureId} in ${projectId}`);
    requireText(feature.acceptance, `feature acceptance for ${featureId} in ${projectId}`);
    requireRecord(doc.domains, feature.domainId, `feature domain for ${featureId} in ${projectId}`);
    const projectModule = requireRecord(doc.modules, feature.moduleId, `feature module for ${featureId} in ${projectId}`);

    if (projectModule.domainId !== feature.domainId) {
      throw new Error(`Feature domain/module mismatch: ${featureId} in ${projectId}`);
    }

    for (const dependencyId of feature.relations.dependsOn) {
      requireRecord(doc.features, dependencyId, `feature dependency for ${featureId} in ${projectId}`);
    }

    for (const supportedId of feature.relations.supports) {
      requireRecord(doc.features, supportedId, `supported feature for ${featureId} in ${projectId}`);
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
