export type FeatureDeliveryStage = "mvp" | "alpha" | "beta" | "future";

export type ProjectDomain = {
  id: string;
  title: string;
  summary: string;
  accent: string;
};

export type ProjectModule = {
  id: string;
  domainId: string;
  title: string;
  summary: string;
};

export type AtomicFeatureReference = {
  featureId: string;
  title: string;
  routeId: "features" | "architecture" | "data-flow" | "implementation-roadmap";
};

export type AtomicFeatureConnectionView = {
  feature: AtomicFeature;
  dependencies: AtomicFeature[];
  dependents: AtomicFeature[];
  relatedModules: ProjectModule[];
  hasFlowSignals: boolean;
};

export type AtomicFeature = {
  id: string;
  title: string;
  intent: string;
  domainId: string;
  moduleId: string;
  stage: FeatureDeliveryStage;
  acceptance: string;
  dependsOn?: string[];
  architecture?: {
    provides?: string[];
    consumes?: string[];
    relatedModuleIds?: string[];
  };
  flow?: {
    inputs?: string[];
    outputs?: string[];
    emits?: string[];
    stateChanges?: string[];
  };
};

export type AtomicFeatureDetailView = AtomicFeature & {
  domain: ProjectDomain;
  module: ProjectModule;
  dependencies: AtomicFeature[];
  dependents: AtomicFeature[];
  relatedModules: ProjectModule[];
  references: AtomicFeatureReference[];
};

export type ModuleArchitectureView = ProjectModule & {
  domain: ProjectDomain;
  features: AtomicFeature[];
  relatedModules: ProjectModule[];
};

export type ArchitectureRelationView = {
  from: ProjectModule;
  to: ProjectModule;
  features: AtomicFeature[];
};

export type DataFlowStepView = {
  feature: AtomicFeature;
  module: ProjectModule;
  inputs: string[];
  outputs: string[];
  emits: string[];
  stateChanges: string[];
};

export type RoadmapStageView = {
  stage: FeatureDeliveryStage;
  title: string;
  outcome: string;
  features: AtomicFeature[];
};
