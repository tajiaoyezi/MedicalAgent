# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> 本仓库的 OpenSpec 产物与交互回复一律使用**简体中文**（代码标识符、API 名、表名、文件名保留英文原文）。详见 `openspec/config.yaml` 的 `context` 段。

## 仓库性质

这是一个 **规格先行（spec-only）仓库**：目前**没有任何应用代码**，全部内容是 MedOffice AI（医疗智能办公空间，以 ONLYOFFICE Docs 替代 WPS）V1.0 POC 的 OpenSpec 规格集。开发方式是 OpenSpec 规格驱动（`@fission-ai/openspec` v1.4.1，schema=`spec-driven`）：先把 9 个有序 change 的 proposal/design/tasks/specs 写定并通过校验，再逐 phase `apply` 落地实现代码。

因此本仓库的「build / lint / test」等价物是 **OpenSpec 校验与生命周期命令**，而非编译测试。

## 常用命令

```bash
openspec validate --changes --strict   # 校验所有 change（提交前必须 9/9 通过；CI 守门等价物）
openspec list                          # 列出所有 change
openspec list --specs                  # 列出已部署 specs（当前为空，尚未 sync）
openspec status --change "<name>" --json   # 查看某 change 的 artifact 完成度与路径
openspec show <change-or-spec>         # 查看单个 change / spec
openspec view                          # 交互式 dashboard
```

规格生命周期通过 `.cursor/skills` 与 `.codex/skills`（以及本仓 `/openspec-propose` 等 skill）驱动，对应斜杠命令：

- `/openspec-propose` — 新建 change 并生成 proposal/design/tasks 全套 artifact。
- `/opsx:apply` — 实现某 change 的 tasks（从 `c01-foundation` 起，按依赖序 c01→c09）。
- `/opsx:sync` — 把 change 的 delta spec 智能合并进 `openspec/specs/` 主规格（agent 驱动，非整段复制）。
- `/opsx:archive` — 实现完成后归档 change（移入 `openspec/changes/archive/` 并更新主规格）。

> 校验只认结构：Scenario 必须**恰好 4 个井号**（`#### Scenario:`），否则会被静默丢弃；tasks 必须是 `- [ ] X.Y` 复选框格式，否则 apply 阶段不跟踪。change 名必须以**字母开头**（这是 9 个目录用 `c0N` 前缀而非纯数字的原因）。

## Git 工作流：禁止直接提交主分支，提交即开 PR + 子 agent 审查后合入

**主分支 `master` 禁止直接提交。**（这是团队/agent 约定；`master` 当前未开 GitHub branch protection，靠本规则自觉执行。）任何改动（含规格、文档、后续实现代码）都走「feature 分支 → PR → 子 agent 审查 → 合入」闭环：

1. **开分支**：从最新 `master` 切 feature 分支（建议命名 `<type>/<scope>`，如 `feat/c01-apply`、`docs/claude-md`、`chore/...`）。若误在 `master` 上改动，先 `git switch -c <branch>` 再提交。
   - **开分支前先核对 base 已同步**：确认本地 `master` 与 `origin/master` 一致、且本地无未推送的提交（`git fetch origin && git log --oneline origin/master..master` 应为空）。否则这些未推送提交会被一并卷进 PR diff，squash 合并时被压成单个提交（曾因此把 9 个 per-change 提交压平）。如本地 `master` 领先 origin，先单独把它推上去再切分支。
2. **提交并开 PR**：在 feature 分支提交后 `git push -u origin <branch>`，用 `gh pr create` 开 PR（base=`master`）。提交信息用 **Conventional Commits 前缀 + 简体中文描述**，type/scope 与分支命名一致，如 `feat(spec): c01-foundation — …`、`docs: 新增 …`、`chore(openspec): …`。
3. **自动派子 agent 审查**：开完 PR **立即用 Claude Code 的 Task/子 agent 能力（Agent 工具）派一个子 agent 审查该 PR**（diff、规格一致性、横切契约对称性、医疗安全红线、`openspec validate --changes --strict`）。子 agent 返回结构化结论：通过 / 有问题（含问题清单与严重级）。*降级口径*：若当前运行时不具备派子 agent 的能力（如纯 CLI / CI），改为人工审查后再合入，不得跳过审查直接合并。
4. **按结论闭环**：
   - **无问题** → `gh pr merge --squash --delete-branch` 合并入 `master` 并删除 feature 分支。
   - **有问题** → 在**同一 feature 分支**修复并追加提交、push（PR 自动更新），**重新派子 agent 审查**；如此循环，直到审查通过再合入。
5. 合入后回到第 1 步开始下一项工作。

> 例外：仅当用户明确要求「直接提交/直接合并/跳过审查」时才偏离此闭环；否则默认每一笔改动都经 PR + 子 agent（或降级人工）审查。`gh` 已认证为 `tajiaoyezi`，remote=`origin`。

## 不可违背的需求来源与范围口径

这些约束**覆盖**默认行为，写任何 artifact 前必须遵守（出处：`openspec/config.yaml`、PRD §0.4 / §22）：

- **唯一权威需求源** = `docs/MedOffice_AI_完整产品需求文档_V1.0.md`。`docs/MedOffice_AI_遗漏项与补充需求清单_V1.0.md` **只作历史差距追溯与审查检查表**，不作为 phase/task 生成输入。正文与第 22 章优先级冲突时以第 22 章为准。
- **V1.0 范围 = 仅 PRD §22.1「P0 必做」**。§22.2（V1.1）/ §22.3（V1.2+）只进 backlog，不生成 V1.0 change/task。
- **医疗数字员工**在 V1.0 仅保留导航「规划中」入口，其创建/运行/编排/执行历史**一律不生成 V1.0 task**（延期 V1.1/V1.2）。
- **医疗安全红线**（每个相关 spec 都要落到场景）：医学结论须可溯源、默认草稿需人工确认；不得定位为自动诊断/处方/医嘱/替代医生；高风险内容进医生（或授权审核角色）确认链路；多租户隔离 + RBAC + 文档级/知识库级 ACL；公网模型调用前必做 PHI/PII 识别脱敏，失败则禁用公网、切私有化模型；未授权商业数据库不默认抓取。
- **离线优先**：任何核心闭环都要有离线/私有化降级路径（离线 PubMed 缓存、私有化模型、私有化解析/识别服务）。

## 9-Phase 架构（依赖序，目录前缀 c01…c09）

PRD §0.4 把 V1.0 拆为 9 个有序 phase，逐级依赖（`openspec/changes/cNN-*/`，各含 `proposal.md` `design.md` `tasks.md` `specs/<capability>/spec.md`）：

1. `c01-foundation` — 账号/RBAC、门户外壳与主题、文档中心与版本、对象存储、管理后台基础、`recent_tasks` 壳。**所有后续 phase 的前置依赖**。
2. `c02-onlyoffice-bridge` — ONLYOFFICE 编辑器集成、Bridge API、保存回调与 AI 写回、版本落盘。
3. `c03-model-and-parse` — 模型 Provider 抽象（公网/私有化双入口 + fallback）、文档视觉/文本解析流水线、`document_chunks`/`embeddings` 写库。
4. `c04-aimed-rag-citation` — AIMed 六模式、RAG 检索编排、Citation 溯源、检索权限六维过滤。
5. `c05-ai-panel-recent-tasks` — 医疗 AI 右侧面板（润色/校对/翻译选区）、写回确认链路、最近任务恢复。
6. `c06-knowledge-admin` — 13 类医疗知识库管理、KB 级 ACL、URL 白名单导入、重建索引。
7. `c07-medical-translation` — 医学翻译（术语一致性、选区翻译、译稿写回）。
8. `c08-template-center` — 医疗模板中心。
9. `c09-security-evidence` — 安全与举证：PHI/PII 识别脱敏门禁（redaction-gateway）、审计、合规证据。

## 跨 change 横切契约（改动时必须全端点同步）

这是本规格集**最易踩坑、最需通读多文件才能理解**的部分。横切契约在并行多 change 生成时反复出现「只接一个端点、对称端点没接」的 high 缺陷（详见记忆 `medoffice-crosscut-contracts-lesson`）。**铁律：每个 event_type / 枚举值 / 物理列 / 权限点只允许一个 owner，消费方一律「引用」而非重定义；改任一端先列全端点清单再一次性同步下发。**

| 契约 | owner（产生/建表方） | 消费/引用方 | 要点 |
|---|---|---|---|
| §18 核心表 | 各表唯一建表 owner（如 `recent_tasks`=c01，`document_chunks`=c03） | 其余 phase 只 `ALTER` 补列，绝不重复建表 | c05 在 `recent_tasks` 上补列而非重建 |
| `document_events`（闭合 6 类） | 各 event_type 唯一产生方：upload_success=c01；save_new_version/ai_writeback=c02；translation_done=c07；template_created=c08；manual_reindex=c06 | **c03 是全 6 类的唯一消费方**，自身不产生 | 审计动作只进 `audit_logs`，不进 `document_events` |
| `chunk_acl` | c03（`document_chunks` 物理列） | c04 检索只消费、不建列 | `document_acl` 是源自 `document_permissions` 的**文档级过滤维**，非 chunk 字段 |
| 高风险确认链路 | c05（message 级确认） | c09 等 | 多态：`subject_type ∈ {document, message, translation_job}` + `subject_id`；RBAC 权限点 `highrisk:confirm` 由 c01 定义 |
| 上传 PHI 门禁 | c09（redaction-gateway，唯一 PHI/PII 识别脱敏实现） | c01/c04/c06/c07 四上传入口全接 | 识别失败/不可用时缺省放行降级口径需一致；脱敏事件统一写 `privacy_redaction_events` |
| `feedbacks`（泛化） | c04 | 各反馈来源 | 多态 `subject_type`/`subject_id` |

数据模型尽量复用 PRD 第 18 章表命名：`documents` / `document_versions` / `document_permissions` / `document_events` / `knowledge_bases` / `document_chunks` / `embeddings` / `translation_jobs` / `model_providers` / `recent_tasks` / `audit_logs` / `privacy_redaction_events` / `feedbacks` 等。

## 当前状态

规格已写完并经多轮对抗式审查收敛到 0 high/critical，`openspec validate --changes --strict` 9/9 通过，已逐 change 提交在 `master`。**c01-foundation 已 apply 落地**；**c02-onlyoffice-bridge 实施中**（PR #11）。下一步：c02 合入后继续 c03 起 `/opsx:apply`。
