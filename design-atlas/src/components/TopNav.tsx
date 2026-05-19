import type { PageId, ProjectDocIndex } from "../domain/projectDocIndex";
import { ShinyText } from "./ui/ShinyText";

type TopNavProps = {
  activePageId: PageId;
  index: ProjectDocIndex;
  onPageChange: (pageId: PageId) => void;
  onProjectPickerOpen: () => void;
};

export function TopNav({ activePageId, index, onPageChange, onProjectPickerOpen }: TopNavProps) {
  return (
    <header className="top-nav">
      <button className="brand-button" type="button" aria-label="切换项目" onClick={onProjectPickerOpen}>
        <span className="brand-mark">{index.project.name.slice(0, 1).toUpperCase()}</span>
        <span>
          <strong><ShinyText>{index.meta.title}</ShinyText></strong>
          <small>{index.project.repositoryLabel}</small>
        </span>
      </button>

      <nav className="page-tabs" aria-label="项目文档页面">
        {index.navigationItems.map((item) => (
          <button className={item.id === activePageId ? "active" : ""} key={item.id} type="button" onClick={() => onPageChange(item.id)}>
            {item.label}
          </button>
        ))}
      </nav>
    </header>
  );
}
