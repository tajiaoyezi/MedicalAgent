## Context

本 change 是项目初始化和阶段规划工作，不直接实现业务代码。主 PRD 已经定义 V1.0 POC 的交付边界、主验收闭环、九个建议 phase、P0 必做清单、V1.1/V1.2+ 路线图和验收标准。当前重建的重点是修正 OpenSpec apply 语义：规划结果不能被当成当前 change 的实施 checkbox。

关键约束：

- V1.0 是 POC 演示版，不是正式医院商用版。
- 实施范围只从主 PRD `22.1 P0 必做` 提取。
- 主 PRD `22.2` 和 `22.3` 只进入 roadmap / backlog。
- 医疗数字员工不进入 V1.0 POC 实施，只允许保留规划入口或即将上线说明。
- 当前文档发起 AIMed 和当前文档发起医学翻译属于 V1.0 P0。
- 后续 task 必须可验收，并包含目标、范围、前置依赖、验收标准、测试/验证方式、风险。

## Goals / Non-Goals

**Goals:**

- 形成一个 OpenSpec 规划 change，描述 V1.0 POC 的范围治理、phase 计划和 roadmap 隔离规则。
- 按主 PRD `0.4` 生成九个 phase 的后续实施任务计划。
- 确保 `openspec apply initialize-v1-poc-planning` 不会启动业务功能开发。
- 为后续每个 phase 创建独立 implementation change 提供拆分依据。

**Non-Goals:**

- 不实现应用代码、数据库迁移、UI 页面或后端服务。
- 不把未来 V1.0 业务任务作为当前 change 的待执行 checkbox。
- 不把遗漏项与补充需求清单作为 V1.0 实施范围输入。
- 不生成数字员工创建、运行、编排、工具调用、执行历史任务。
- 不把 V1.1/V1.2+ 规划作为 V1.0 验收前置条件。

## Decisions

### Decision 1: 保留单一规划 capability

本 change 只新增 `v1-poc-planning` 一个 capability，用于统一约束需求源治理、phase 顺序、task 字段结构、roadmap 隔离和数字员工边界。这样避免重新出现多个 spec 目录被误认为 phase 目录的问题。

备选方案是拆成 scope、phase、roadmap 三个 capability。未采用，因为用户已经明确期望生成 phase 和 task，而不是多个看起来像 phase 的 spec capability。

### Decision 2: 未来实施任务不用 checkbox

`tasks.md` 中只保留本规划 change 已完成的可执行规划任务 checkbox。47 个 V1.0 后续业务任务用编号标题和字段描述，不使用 `- [ ]`，并明确标注为“非当前 apply 任务”。

理由：OpenSpec apply 会解析 checkbox。如果把未来业务任务写成 `- [ ]`，apply 会正确地把它们当作当前待执行任务，这与规划 change 的 Non-Goals 冲突。

### Decision 3: 后续实施按 phase 创建独立 change

后续推荐按以下顺序创建 implementation change：

| 顺序 | Phase | 建议 change |
|---:|---|---|
| 1 | foundation | `implement-foundation-phase` |
| 2 | onlyoffice-bridge | `implement-onlyoffice-bridge-phase` |
| 3 | model-and-parse | `implement-model-and-parse-phase` |
| 4 | aimed-rag-citation | `implement-aimed-rag-citation-phase` |
| 5 | ai-panel-recent-tasks | `implement-ai-panel-recent-tasks-phase` |
| 6 | knowledge-admin | `implement-knowledge-admin-phase` |
| 7 | medical-translation | `implement-medical-translation-phase` |
| 8 | template-center | `implement-template-center-phase` |
| 9 | security-evidence | `implement-security-evidence-phase` |

每个 implementation change 可以把对应 phase 的后续任务转换为 `- [ ]` checkbox，并按真实技术栈补充实现细节。

### Decision 4: 技术栈仍作为后续 implementation decision

当前规划不隐式决定前后端框架、数据库、对象存储、鉴权方案、模型 provider 或部署方式。启动 `foundation` implementation change 前必须先明确这些工程决策。

## Risks / Trade-offs

- [Risk] 规划文件中保留大量后续 task，文件会较长。Mitigation: 使用 phase 分组，并明确它们不是当前 apply checkbox。
- [Risk] 后续实施时可能忘记重新创建 implementation change。Mitigation: 在 spec、design、tasks 中重复写入“按 phase 创建独立 change”的规则。
- [Risk] 主 PRD P0 范围很大，单个 phase 仍可能过大。Mitigation: implementation change 可继续按子任务拆分，但不得扩大 V1.0 P0 范围。
- [Risk] 技术栈未定会阻塞真实开发。Mitigation: 把技术栈选择放入 `foundation` 前置决策，不在本规划 change 中隐式决定。

## Migration Plan

1. 删除旧的 `initialize-v1-poc-planning` change 产物。
2. 重建本规划 change，并只保留已完成的规划任务 checkbox。
3. 将 V1.0 POC 业务任务作为非当前 apply 的 phase/task plan 记录。
4. 后续实施时从 `implement-foundation-phase` 开始创建新的 OpenSpec change。

## Open Questions

- V1.0 POC 的具体技术栈、前后端框架、数据库和对象存储实例是否已经确定。
- PubMed 离线缓存、Demo 数据集、医学翻译参考译文和 200 个模板资产是否已有现成来源。
- 公网模型、私有化模型、Embedding、Rerank、文档视觉解析服务的实际 provider 优先级是否需要在实施前单独确认。
