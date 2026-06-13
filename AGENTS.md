# 仓库贡献指南

## 项目结构与模块组织

本仓库当前用于维护 MedOffice AI V1.0 概念验证版的产品文档与 OpenSpec 规划产物，暂未建立应用源码目录。

- `docs/`：产品文档目录。`docs/MedOffice_AI_完整产品需求文档_V1.0.md` 是 V1.0 执行范围的唯一权威来源。
- `docs/MedOffice_AI_遗漏项与补充需求清单_V1.0.md`：仅作历史追溯和审查参考，不用于扩大 V1.0 范围。
- `docs/old/`：历史 PDF 资料归档。
- `openspec/config.yaml`：OpenSpec 配置、中文输出规则和范围约束。
- `openspec/changes/initialize-v1-poc-planning/`：当前 V1.0 POC 的 proposal、design、spec 和 task 清单。

后续引入源码时，建议使用 `src/` 存放实现，`tests/` 存放测试，`assets/` 存放静态资源或演示素材。

## 构建、测试与开发命令

当前尚无应用构建流水线。规划阶段主要使用 OpenSpec 和 Git 做校验：

- `openspec list --json`：查看当前活动变更。
- `openspec status --change "initialize-v1-poc-planning" --json`：查看规划产物状态。
- `openspec validate "initialize-v1-poc-planning"`：提交或评审前校验当前变更。
- `git status -sb`：确认暂存区和工作区状态。

确定应用技术栈后，应在本节补充实际的启动、构建、测试命令。

## 编码风格与命名约定

仓库内说明性文字默认使用简体中文。工具要求的关键字、路径、命令、API 名称和能力 ID 可保留英文。OpenSpec capability 目录使用小写 kebab-case，例如 `v1-poc-planning`。

OpenSpec 固定语法必须保持原样，例如 `## ADDED Requirements`、`### Requirement:`、`#### Scenario:`、`WHEN`、`THEN`。Requirement 名称、Scenario 名称和描述内容应使用中文。

## 测试与验收要求

规划阶段的验证包括 OpenSpec CLI 校验，以及对照主 PRD 做人工范围复核。`tasks.md` 中每个任务都必须可验收，并包含：目标、范围、前置依赖、验收标准、测试/验证方式、风险。

进入实现阶段后，测试应跟随选定技术栈落地，并在本文件补充准确的测试命令和命名规则。

## 提交与拉取请求规范

当前提交历史采用简短的 Conventional Commit 风格，例如 `chore: initial project setup` 和 `docs: add v1 poc openspec planning`。继续使用 `docs:`、`chore:`、`feat:`、`fix:`、`test:` 等前缀。

默认分支 `master` 必须保持为保护分支。所有代码、文档和配置变更都必须先提交到非主分支，再通过 Pull Request 合入；禁止直接 push 到 `master`。当前无强制评审人数要求，后续增加协作者或 CI 后可提高审批和检查门槛。

拉取请求应包含变更摘要、影响路径、已执行的验证命令；涉及界面变化时补充截图。任何影响 V1.0 范围的 PR 都必须引用主 PRD 对应章节，并说明相关 OpenSpec spec 或 task 的调整。

## 安全与配置注意事项

不要提交 `.claude/`、`.codex/`、`.cursor/` 等本地工具目录。不要把密钥、API token、私有模型凭据或环境专属连接串写入受版本控制的文件。
