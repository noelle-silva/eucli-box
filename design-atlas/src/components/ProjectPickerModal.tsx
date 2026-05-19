import type { ProjectCatalogEntry } from "../domain/projectDocIndex";

type ProjectPickerModalProps = {
  activeProjectId: string;
  projects: ProjectCatalogEntry[];
  onClose: () => void;
  onProjectSelect: (projectId: string) => void;
};

export function ProjectPickerModal({ activeProjectId, projects, onClose, onProjectSelect }: ProjectPickerModalProps) {
  return (
    <div className="modal-layer" role="presentation" onMouseDown={onClose}>
      <section aria-labelledby="project-picker-title" aria-modal="true" className="project-picker-modal" role="dialog" onMouseDown={(event) => event.stopPropagation()}>
        <header className="modal-head">
          <div>
            <span className="eyebrow">Project Registry</span>
            <h2 id="project-picker-title">选择项目仓库</h2>
            <p>每个项目拥有独立 JSON 文档和独立索引。切换项目后，页面只读取当前项目的数据。</p>
          </div>
          <button className="close-button" type="button" onClick={onClose}>关闭</button>
        </header>

        <div className="project-picker-grid">
          {projects.map((project) => {
            const isActive = project.id === activeProjectId;

            return (
              <button className={isActive ? "project-option active" : "project-option"} key={project.id} type="button" onClick={() => onProjectSelect(project.id)}>
                <span className="project-option-mark">{project.name.slice(0, 1).toUpperCase()}</span>
                <span className="project-option-body">
                  <strong>{project.name}</strong>
                  <small>{project.repositoryLabel}</small>
                  <span>{project.summary}</span>
                </span>
                {isActive ? <em>当前</em> : null}
              </button>
            );
          })}
        </div>
      </section>
    </div>
  );
}
