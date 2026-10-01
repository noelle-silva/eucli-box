# AI 工具开发构建说明

本目录是 eucli-box 全部 AI 工具的源码所在地，一个工具一个文件夹。

## 构建一个工具

进入要构建的工具目录，运行一条命令即可：

```
go run devtools/build
```

例如构建 `file_reader`：

```
cd apps/eucli-box/tools/file_reader
go run devtools/build
```

命令会自动读取当前工具自己的 `tool.json` 认出身份，构建成品并按四段开发版本铺入本地开发货架：

```
.dev-workspace/.dev-tools-runtime/eucli-store-sync/output/ai-tools
```

业务端注册该货架根后即可看到新成品。

## 指定版本

默认按「货架中同基线的最大开发尾号 + 1」自动递增；需要固定版本时显式传入：

```
go run devtools/build -version 0.2.0.9
```

## 说明

- 构建入口在 `.dev-tools/build/`，工具目录内**不需要放任何构建文件**，工具身份完全来自各自的 `tool.json`。
- 该命令只负责把仓库既有的铺货链（`eucli-store-sync`）以最浅的方式暴露出来，不重复任何构建逻辑。
- 命令必须在**工具目录内**运行；在非工具目录运行会明确报错。
