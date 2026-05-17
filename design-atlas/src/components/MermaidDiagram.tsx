import { useEffect, useId, useState } from "react";

type MermaidModule = typeof import("mermaid");

let mermaidLoader: Promise<MermaidModule> | undefined;

async function loadMermaid() {
  if (!mermaidLoader) {
    mermaidLoader = import("mermaid").then((module) => {
      module.default.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: "base",
        themeVariables: {
          background: "transparent",
          primaryColor: "#eef2ff",
          primaryTextColor: "#0f172a",
          primaryBorderColor: "#7c3aed",
          lineColor: "#6366f1",
          secondaryColor: "#ecfeff",
          tertiaryColor: "#f8fafc",
          fontFamily: "Inter, Segoe UI, Microsoft YaHei, sans-serif",
        },
      });

      return module;
    });
  }

  return mermaidLoader;
}

type MermaidDiagramProps = {
  chart: string;
};

export function MermaidDiagram({ chart }: MermaidDiagramProps) {
  const reactId = useId();
  const diagramId = `mermaid-${reactId.replace(/[^a-zA-Z0-9_-]/g, "")}`;
  const [svg, setSvg] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let disposed = false;

    async function renderChart() {
      setError(null);
      setSvg("");

      try {
        const mermaid = await loadMermaid();
        const result = await mermaid.default.render(diagramId, chart);

        if (!disposed) {
          setSvg(result.svg);
        }
      } catch (renderError) {
        if (!disposed) {
          setError(renderError instanceof Error ? renderError.message : String(renderError));
        }
      }
    }

    renderChart();

    return () => {
      disposed = true;
    };
  }, [chart, diagramId]);

  if (error) {
    return (
      <div className="mermaid-diagram mermaid-diagram-error">
        <strong>Mermaid 渲染失败</strong>
        <pre>{error}</pre>
      </div>
    );
  }

  if (!svg) {
    return <div className="mermaid-diagram mermaid-diagram-loading">正在加载 Mermaid 图表渲染器...</div>;
  }

  return <div className="mermaid-diagram" dangerouslySetInnerHTML={{ __html: svg }} />;
}
