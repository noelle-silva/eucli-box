import { useEffect, useState } from "react";
import { AtomicFeaturePage } from "./components/AtomicFeaturePage";
import { ProjectPickerModal } from "./components/ProjectPickerModal";
import { TopNav } from "./components/TopNav";
import type { PageId, ProjectDocIndex } from "./domain/projectDocIndex";
import { defaultProjectId, loadProjectDocIndex, projectCatalogIndex } from "./domain/projectDocIndex";

export function App() {
  const [activeProjectId, setActiveProjectId] = useState(defaultProjectId);
  const [projectIndex, setProjectIndex] = useState<ProjectDocIndex | undefined>();
  const [loadError, setLoadError] = useState<string | undefined>();
  const [activePageId, setActivePageId] = useState<PageId>("atomic-features");
  const [isProjectPickerOpen, setIsProjectPickerOpen] = useState(false);

  useEffect(() => {
    let isCurrentRequest = true;
    setLoadError(undefined);
    setProjectIndex(undefined);

    loadProjectDocIndex(activeProjectId)
      .then((nextProjectIndex) => {
        if (isCurrentRequest) {
          setProjectIndex(nextProjectIndex);
          setActivePageId("atomic-features");
        }
      })
      .catch((error: unknown) => {
        if (isCurrentRequest) {
          setProjectIndex(undefined);
          setLoadError(error instanceof Error ? error.message : "Project document loading failed");
        }
      });

    return () => {
      isCurrentRequest = false;
    };
  }, [activeProjectId]);

  if (loadError) {
    return (
      <div className="app-shell">
        <main className="page-shell custom-scroll-area">
          <section className="placeholder-page error-page">
            <span className="eyebrow">Project Load Error</span>
            <h1>项目索引加载失败</h1>
            <p>{loadError}</p>
          </section>
        </main>
      </div>
    );
  }

  if (!projectIndex) {
    return (
      <div className="app-shell">
        <main className="page-shell custom-scroll-area">
          <section className="placeholder-page">
            <span className="eyebrow">Loading Project</span>
            <h1>正在加载项目索引</h1>
            <p>文档容器正在按当前项目读取独立 JSON，并构建页面所需的领域、模块和原子功能索引。</p>
          </section>
        </main>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <TopNav activePageId={activePageId} index={projectIndex} onPageChange={setActivePageId} onProjectPickerOpen={() => setIsProjectPickerOpen(true)} />
      {activePageId === "atomic-features" ? <AtomicFeaturePage index={projectIndex} /> : <PlaceholderPage index={projectIndex} pageId={activePageId} />}
      {isProjectPickerOpen ? (
        <ProjectPickerModal
          activeProjectId={activeProjectId}
          projects={projectCatalogIndex}
          onClose={() => setIsProjectPickerOpen(false)}
          onProjectSelect={(projectId) => {
            setActiveProjectId(projectId);
            setIsProjectPickerOpen(false);
          }}
        />
      ) : null}
    </div>
  );
}

function PlaceholderPage({ index, pageId }: { index: ProjectDocIndex; pageId: PageId }) {
  return (
    <main className="page-shell custom-scroll-area">
      <section className="placeholder-page">
        <span className="eyebrow">Coming Next</span>
        <h1>{index.navigationItems.find((item) => item.id === pageId)?.label}</h1>
        <p>页面机制已预留，下一步会继续从同一份 JSON 多级索引派生视角。</p>
      </section>
    </main>
  );
}
