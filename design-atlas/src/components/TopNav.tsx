import type { PageId, NavigationItem } from "../domain/projectDocIndex";
import { projectMeta } from "../domain/projectDocIndex";
import { ShinyText } from "./ui/ShinyText";

type TopNavProps = {
  activePageId: PageId;
  items: NavigationItem[];
  onPageChange: (pageId: PageId) => void;
};

export function TopNav({ activePageId, items, onPageChange }: TopNavProps) {
  return (
    <header className="top-nav">
      <button className="brand-button" type="button" onClick={() => onPageChange("atomic-features")}>
        <span className="brand-mark">E</span>
        <span>
          <strong><ShinyText>{projectMeta.title}</ShinyText></strong>
          <small>{projectMeta.subtitle}</small>
        </span>
      </button>

      <nav className="page-tabs" aria-label="项目文档页面">
        {items.map((item) => (
          <button className={item.id === activePageId ? "active" : ""} key={item.id} type="button" onClick={() => onPageChange(item.id)}>
            {item.label}
          </button>
        ))}
      </nav>
    </header>
  );
}
