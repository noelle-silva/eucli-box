# eucli-box Design Atlas

一个独立的 Vite + React + TypeScript 可视化工程，用来把 `eucli-box` 的项目构想展示成带左侧导航的分区式设计驾驶舱。

## 目标

- 用 UI 可视化项目定位、三层架构、子系统、feature 优先级、协议、同步、权限、Agent、供应商、记忆、设计决策和路线图。
- 使用统一路由数据模型驱动页面，不把所有内容塞进单个长页面。
- 支持 Mermaid 内容块，用同一套机制展示架构图、同步流、权限流和 Agent 主链路。
- 保持数据层、布局层、视觉系统和业务可视化组件分离，避免补丁式堆叠。
- 作为后续讨论 MVP 范围、协议字段、数据模型和实现顺序的共识界面。

## 结构

```text
src/
  App.tsx
  main.tsx
  components/
    layout/
      RoutePage.tsx
      Sidebar.tsx
    visual-system/
      GradientText.tsx
      ShinyText.tsx
      SpotlightPanel.tsx
      index.ts
    ArchitectureMap.tsx
    DecisionBoard.tsx
    FeatureMatrix.tsx
    MermaidDiagram.tsx
    MvpFlow.tsx
    Roadmap.tsx
    SubsystemGrid.tsx
  data/
    designAtlas.ts
    designRoutes.ts
  styles/
    global.css
    visual-system.css
```

## 命令

```bash
npm install
npm run dev
npm run build
npm run preview
```

## 设计原则

- 路由数据化：左侧导航和右侧内容由 `src/data/designRoutes.ts` 统一驱动。
- 内容资产化：feature、子系统、决策、风险、路线图集中在 `src/data/designAtlas.ts`。
- 布局机制化：`Sidebar` 负责分类导航，`RoutePage` 负责统一渲染文档块。
- 图表机制化：Mermaid 通过 `MermaidDiagram` 按需加载，避免把图表渲染器塞进首屏主包。
- 视觉系统化：高级视觉能力集中在 `src/components/visual-system` 与 `src/styles/visual-system.css`。
- 响应式优先：桌面端是固定侧边栏，移动端自动转为上方导航区。
