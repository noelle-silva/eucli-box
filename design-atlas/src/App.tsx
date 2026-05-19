import { useState } from "react";
import { AtomicFeaturePage } from "./components/AtomicFeaturePage";
import { TopNav } from "./components/TopNav";
import type { PageId } from "./domain/projectDocIndex";
import { navigationItems } from "./domain/projectDocIndex";

export function App() {
  const [activePageId, setActivePageId] = useState<PageId>("atomic-features");

  return (
    <div className="app-shell">
      <TopNav activePageId={activePageId} items={navigationItems} onPageChange={setActivePageId} />
      {activePageId === "atomic-features" ? <AtomicFeaturePage /> : <PlaceholderPage pageId={activePageId} />}
    </div>
  );
}

function PlaceholderPage({ pageId }: { pageId: PageId }) {
  return (
    <main className="page-shell custom-scroll-area">
      <section className="placeholder-page">
        <span className="eyebrow">Coming Next</span>
        <h1>{navigationItems.find((item) => item.id === pageId)?.label}</h1>
        <p>页面机制已预留，下一步会继续从同一份 JSON 多级索引派生视角。</p>
      </section>
    </main>
  );
}
