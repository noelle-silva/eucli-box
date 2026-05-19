import { atomicFeatures, projectDomains, projectModules } from "./catalog";
import type { ArchitectureRelationView, AtomicFeature, AtomicFeatureConnectionView, AtomicFeatureDetailView, DataFlowStepView, FeatureDeliveryStage, ModuleArchitectureView, RoadmapStageView } from "./types";

export const featureDeliveryStageLabels: Record<FeatureDeliveryStage, string> = {
  mvp: "MVP 必须闭环",
  alpha: "Alpha 结构化",
  beta: "Beta 多实例",
  future: "未来演进",
};

export const atomicFeatureConnections: AtomicFeatureConnectionView[] = atomicFeatures.map((feature) => ({
  feature,
  dependencies: (feature.dependsOn ?? []).map(getAtomicFeature),
  dependents: atomicFeatures.filter((candidate) => candidate.dependsOn?.includes(feature.id)),
  relatedModules: (feature.architecture?.relatedModuleIds ?? []).map(getModule),
  hasFlowSignals: Boolean(feature.flow),
}));

export const architectureModules: ModuleArchitectureView[] = projectModules
  .map((projectModule) => {
    const domain = getDomain(projectModule.domainId);
    const features = atomicFeatures.filter((feature) => feature.moduleId === projectModule.id);
    const relatedModuleIds = new Set(features.flatMap((feature) => feature.architecture?.relatedModuleIds ?? []));

    return {
      ...projectModule,
      domain,
      features,
      relatedModules: projectModules.filter((relatedModule) => relatedModuleIds.has(relatedModule.id)),
    };
  })
  .filter((moduleView) => moduleView.features.length > 0);

export const architectureRelations: ArchitectureRelationView[] = createArchitectureRelations();

export const dataFlowSteps: DataFlowStepView[] = atomicFeatures
  .filter((feature) => feature.flow)
  .map((feature) => ({
    feature,
    module: getModule(feature.moduleId),
    inputs: feature.flow?.inputs ?? [],
    outputs: feature.flow?.outputs ?? [],
    emits: feature.flow?.emits ?? [],
    stateChanges: feature.flow?.stateChanges ?? [],
  }));

export const roadmapStages: RoadmapStageView[] = [
  {
    stage: "mvp",
    title: "MVP 必须闭环",
    outcome: "先让单实例完成命令入口、实时事件、会话、工具调用、权限和供应商调用。",
    features: getFeaturesByStage("mvp"),
  },
  {
    stage: "alpha",
    title: "Alpha 结构化",
    outcome: "让工具、Agent、供应商、提示词和可观察 step 进入结构化管理。",
    features: getFeaturesByStage("alpha"),
  },
  {
    stage: "beta",
    title: "Beta 多实例",
    outcome: "让中心权威、本地副本、分频道同步和冲突控制形成多实例协作基础。",
    features: getFeaturesByStage("beta"),
  },
  {
    stage: "future",
    title: "未来演进",
    outcome: "补齐记忆、事件恢复、密钥轮换和更高信任级别的工具执行模型。",
    features: getFeaturesByStage("future"),
  },
];

export function hasAtomicFeature(featureId: string) {
  return atomicFeatures.some((feature) => feature.id === featureId);
}

export function getAtomicFeatureDetail(featureId: string): AtomicFeatureDetailView {
  const feature = getAtomicFeature(featureId);

  return {
    ...feature,
    domain: getDomain(feature.domainId),
    module: getModule(feature.moduleId),
    dependencies: (feature.dependsOn ?? []).map(getAtomicFeature),
    dependents: atomicFeatures.filter((candidate) => candidate.dependsOn?.includes(feature.id)),
    relatedModules: (feature.architecture?.relatedModuleIds ?? []).map(getModule),
    references: [
      { featureId: feature.id, title: "原子功能列表", routeId: "features" },
      { featureId: feature.id, title: "架构图", routeId: "architecture" },
      { featureId: feature.id, title: "数据流向图", routeId: "data-flow" },
      { featureId: feature.id, title: "实现路线", routeId: "implementation-roadmap" },
    ],
  };
}

function createArchitectureRelations(): ArchitectureRelationView[] {
  const relationByKey = new Map<string, ArchitectureRelationView>();

  for (const feature of atomicFeatures) {
    const from = getModule(feature.moduleId);

    for (const relatedModuleId of feature.architecture?.relatedModuleIds ?? []) {
      const to = getModule(relatedModuleId);
      const key = `${from.id}->${to.id}`;
      const existing = relationByKey.get(key);

      if (existing) {
        existing.features.push(feature);
        continue;
      }

      relationByKey.set(key, { from, to, features: [feature] });
    }
  }

  return [...relationByKey.values()];
}

function getFeaturesByStage(stage: FeatureDeliveryStage) {
  return atomicFeatures.filter((feature) => feature.stage === stage);
}

function getAtomicFeature(featureId: string): AtomicFeature {
  const feature = atomicFeatures.find((item) => item.id === featureId);

  if (!feature) {
    throw new Error(`Missing atomic feature while deriving view: ${featureId}`);
  }

  return feature;
}

function getDomain(domainId: string) {
  const domain = projectDomains.find((item) => item.id === domainId);

  if (!domain) {
    throw new Error(`Missing project domain while deriving view: ${domainId}`);
  }

  return domain;
}

function getModule(moduleId: string) {
  const projectModule = projectModules.find((item) => item.id === moduleId);

  if (!projectModule) {
    throw new Error(`Missing project module while deriving view: ${moduleId}`);
  }

  return projectModule;
}
