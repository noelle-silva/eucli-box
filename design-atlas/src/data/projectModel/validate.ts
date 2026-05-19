import type { AtomicFeature, ProjectDomain, ProjectModule } from "./types";

type ProjectModelInput = {
  domains: ProjectDomain[];
  modules: ProjectModule[];
  features: AtomicFeature[];
};

export function validateProjectModel({ domains, modules, features }: ProjectModelInput) {
  const domainIds = createUniqueIdSet(domains, "project domain");
  const moduleIds = createUniqueIdSet(modules, "project module");
  const featureIds = createUniqueIdSet(features, "atomic feature");

  for (const domain of domains) {
    requireText(domain.title, `Project domain ${domain.id} title`);
    requireText(domain.summary, `Project domain ${domain.id} summary`);
    requireText(domain.accent, `Project domain ${domain.id} accent`);
  }

  for (const projectModule of modules) {
    requireText(projectModule.title, `Project module ${projectModule.id} title`);
    requireText(projectModule.summary, `Project module ${projectModule.id} summary`);

    if (!domainIds.has(projectModule.domainId)) {
      throw new Error(`Project module references missing domain: ${projectModule.id} -> ${projectModule.domainId}`);
    }
  }

  for (const feature of features) {
    requireText(feature.title, `Atomic feature ${feature.id} title`);
    requireText(feature.intent, `Atomic feature ${feature.id} intent`);
    requireText(feature.acceptance, `Atomic feature ${feature.id} acceptance`);

    if (!domainIds.has(feature.domainId)) {
      throw new Error(`Atomic feature references missing domain: ${feature.id} -> ${feature.domainId}`);
    }

    if (!moduleIds.has(feature.moduleId)) {
      throw new Error(`Atomic feature references missing module: ${feature.id} -> ${feature.moduleId}`);
    }

    const projectModule = modules.find((moduleItem) => moduleItem.id === feature.moduleId);

    if (projectModule?.domainId !== feature.domainId) {
      throw new Error(`Atomic feature domain/module mismatch: ${feature.id} -> ${feature.domainId}/${feature.moduleId}`);
    }

    for (const dependencyId of feature.dependsOn ?? []) {
      if (!featureIds.has(dependencyId)) {
        throw new Error(`Atomic feature references missing dependency: ${feature.id} -> ${dependencyId}`);
      }
    }

    for (const relatedModuleId of feature.architecture?.relatedModuleIds ?? []) {
      if (!moduleIds.has(relatedModuleId)) {
        throw new Error(`Atomic feature references missing related module: ${feature.id} -> ${relatedModuleId}`);
      }
    }
  }
}

function createUniqueIdSet(items: Array<{ id: string }>, label: string) {
  const ids = new Set<string>();

  for (const item of items) {
    requireText(item.id, `${label} id`);

    if (ids.has(item.id)) {
      throw new Error(`Duplicate ${label} id: ${item.id}`);
    }

    ids.add(item.id);
  }

  return ids;
}

function requireText(value: string, label: string) {
  if (value.trim().length === 0) {
    throw new Error(`${label} must not be empty`);
  }
}
