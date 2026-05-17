type ArchitectureNode = {
  id: string;
  title: string;
  role: string;
  responsibilities: string[];
};

const nodes: ArchitectureNode[] = [
  {
    id: "client",
    title: "自定义客户端",
    role: "显示器 + 遥控器",
    responsibilities: ["发起会话", "展示聊天 / 工具 / 状态", "批准权限", "管理配置界面"],
  },
  {
    id: "runtime",
    title: "eucli-box 实例",
    role: "Agent Runtime 核心",
    responsibilities: ["会话管理", "Agent 循环", "工具执行", "权限控制", "供应商调用"],
  },
  {
    id: "storage",
    title: "数据存储中心",
    role: "权威数据源",
    responsibilities: ["会话与聊天记录", "配置与权限", "Agent 身份", "同步事件流"],
  },
  {
    id: "tools",
    title: "工具子进程",
    role: "隔离执行单元",
    responsibilities: ["stdin/stdout JSON", "任意语言实现", "崩溃不影响实例"],
  },
  {
    id: "providers",
    title: "模型供应商",
    role: "LLM 能力来源",
    responsibilities: ["Anthropic", "OpenAI-compatible", "模型组与熔断"],
  },
  {
    id: "plugins",
    title: "系统插件",
    role: "后台动态能力",
    responsibilities: ["占位符动态值", "记忆/RAG", "时间/天气/表情包"],
  },
];

const relations = [
  { from: "client", to: "runtime", label: "REST + WebSocket", description: "客户端发命令，实例推事件" },
  { from: "runtime", to: "storage", label: "Pull Sync", description: "实例按频道拉取和上传增量" },
  { from: "runtime", to: "tools", label: "spawn", description: "实例启动工具子进程执行任务" },
  { from: "runtime", to: "providers", label: "LLM API", description: "实例调用模型完成推理" },
  { from: "runtime", to: "plugins", label: "dynamic slots", description: "插件提供占位符与后台能力" },
];

export function ArchitectureMap() {
  const runtime = nodes.find((node) => node.id === "runtime");
  const satellites = nodes.filter((node) => node.id !== "runtime");

  if (!runtime) {
    throw new Error("ArchitectureMap requires a runtime node.");
  }

  return (
    <section className="architecture-diagram" aria-label="eucli-box system collaboration diagram">
      <div className="architecture-intro">
        <span className="diagram-kicker">System Collaboration</span>
        <h2>这张图表达什么？</h2>
        <p>它展示 eucli-box 不是一个单独页面，而是由客户端、运行时实例、数据中心、工具、供应商和系统插件协作组成的 Agent 服务器体系。</p>
      </div>

      <div className="architecture-core-layout">
        <div className="architecture-satellite-grid">
          {satellites.map((node) => (
            <ArchitectureCard node={node} key={node.id} />
          ))}
        </div>

        <div className="architecture-core-column">
          <ArchitectureCard node={runtime} isCore />
          <div className="architecture-relation-list" aria-label="system relations">
            {relations.map((relation) => (
              <article className="architecture-relation" key={`${relation.from}-${relation.to}`}>
                <span>{relation.label}</span>
                <strong>{getNodeTitle(relation.from)} → {getNodeTitle(relation.to)}</strong>
                <p>{relation.description}</p>
              </article>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

function ArchitectureCard({ node, isCore = false }: { node: ArchitectureNode; isCore?: boolean }) {
  return (
    <article className={isCore ? "architecture-card architecture-card-core" : "architecture-card"}>
      <div className="architecture-card-head">
        <span>{node.role}</span>
        <strong>{node.title}</strong>
      </div>
      <ul>
        {node.responsibilities.map((responsibility) => (
          <li key={responsibility}>{responsibility}</li>
        ))}
      </ul>
    </article>
  );
}

function getNodeTitle(nodeId: string) {
  const node = nodes.find((item) => item.id === nodeId);

  if (!node) {
    throw new Error(`Unknown architecture node: ${nodeId}`);
  }

  return node.title;
}
