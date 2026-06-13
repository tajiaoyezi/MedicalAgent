## Context

本 change 是项目初始化和阶段规划工作，不直接实现业务代码。主 PRD 已经定义 V1.0 POC 的交付边界、主验收闭环、九个建议 phase、P0 必做清单、V1.1/V1.2+ 路线图和验收标准。当前需要把这些内容转成 OpenSpec 可执行规划，供后续实施时逐 phase 执行。

关键约束：

- V1.0 是 POC 演示版，不是正式医院商用版。
- 实施范围只从主 PRD `22.1 P0 必做` 提取。
- 主 PRD `22.2` 和 `22.3` 只进入 roadmap / backlog。
- 医疗数字员工不进入 V1.0 POC 实施，只允许保留规划入口或即将上线说明。
- 当前文档发起 AIMed 和当前文档发起医学翻译属于 V1.0 P0。
- 任务必须可验收，并包含目标、范围、前置依赖、验收标准、测试/验证方式、风险。

## Goals / Non-Goals

**Goals:**

- 形成一个 OpenSpec change，描述 V1.0 POC 的范围治理、phase 计划和 roadmap 隔离规则。
- 按主 PRD `0.4` 生成九个 phase 的任务清单。
- 确保任务覆盖 `22.1 P0 必做`，且每个任务可独立验收。
- 将 V1.1/V1.2+ 内容记录为非阻塞 backlog / roadmap。

**Non-Goals:**

- 不实现应用代码、数据库迁移、UI 页面或后端服务。
- 不把遗漏项与补充需求清单作为 V1.0 实施范围输入。
- 不生成数字员工创建、运行、编排、工具调用、执行历史任务。
- 不把 V1.1/V1.2+ 规划作为 V1.0 验收前置条件。

## Decisions

### Decision 1: 使用单一规划 capability 承载本次初始化

本 change 只新增 `v1-poc-planning` 一个 capability，用于统一约束：

| 约束方向 | 内容 |
|---|---|
| 需求源治理 | 唯一需求源、P0 范围、优先级冲突裁决 |
| 阶段计划 | 九个 phase、依赖关系、task 字段结构 |
| 路线图隔离 | V1.1/V1.2+ 只进入 roadmap/backlog |
| 数字员工边界 | 仅保留规划入口，不实施运行能力 |

理由：用户期望生成 phase 和 task；将规划约束合并为一个 capability，可以避免生成多个看起来像 phase 的 spec 目录。

备选方案：拆成 scope、phase、roadmap 三个 capability。未采用，因为目录结构会让人误以为这些是 phase 目录。

### Decision 2: phase 顺序完全采用主 PRD 0.4

V1.0 POC phase 顺序固定为：

| Phase | 说明 | 依赖 |
|---|---|---|
| foundation | 单租户账号、权限、文档中心、对象存储、基础后台可运行 | 无 |
| onlyoffice-bridge | Word / PPT / Excel / PDF 接入，保存回调和版本链路可用 | foundation |
| model-and-parse | 公网 / 私有化模型配置、文档视觉解析和解析入库可用 | foundation |
| aimed-rag-citation | AIMed 六大模式、PubMed / 离线缓存、RAG、引用溯源可用 | onlyoffice-bridge, model-and-parse |
| ai-panel-recent-tasks | 医疗 AI 面板、写回确认、最近任务恢复可用 | onlyoffice-bridge, aimed-rag-citation |
| knowledge-admin | 13 个知识库、上传 / URL / PubMed 导入、索引和问答可用 | model-and-parse, aimed-rag-citation |
| medical-translation | 文件级医学翻译、版式还原、双语对照和历史可用 | onlyoffice-bridge, model-and-parse |
| template-center | 200 个模板、搜索筛选、使用模板和 ONLYOFFICE 打开可用 | onlyoffice-bridge |
| security-evidence | 脱敏、免责声明、审计、Demo 数据集和验收用例完整 | foundation through template-center |

理由：该顺序是主 PRD 明确给出的后续 phase / task 生成规则，且依赖关系符合技术落地顺序。

### Decision 3: tasks.md 使用详细任务字段而不是仅列 checkbox

每个任务仍使用 OpenSpec 可追踪的 `- [ ] X.Y` checkbox 格式，但在 checkbox 下补齐目标、范围、前置依赖、验收标准、测试/验证方式、风险。

理由：OpenSpec apply 阶段需要 checkbox 追踪，用户同时要求每个 task 可验收并包含完整实施信息。

### Decision 4: V1.1/V1.2+ backlog 不放入实施 checkbox

V1.1 和 V1.2+ 项目记录在 tasks.md 的 roadmap 区域中，但不使用 `- [ ]` checkbox，避免被 OpenSpec apply 当作 V1.0 待办。

理由：用户明确要求 roadmap / backlog 不进入 V1.0 POC 实施任务。

### Decision 5: 数字员工只作为范围边界处理

V1.0 允许保留医疗数字员工导航入口或规划中页面，但不生成任何创建、运行、编排、工具调用、执行历史任务。

理由：主 PRD 12.2、22.2、22.3、24.8 一致说明数字员工真实能力不进入 V1.0 POC 验收。

## Risks / Trade-offs

- [Risk] P0 范围很大，单个 tasks.md 会较长。Mitigation: 按 phase 分组，每个任务保留可执行字段，后续实施时可逐 phase 拆成更小 change。
- [Risk] 正文中存在数字员工、文档脑图、PPT 生成等后续能力描述，容易误入 V1.0。Mitigation: 以第 22 章为优先级裁决点，并在 spec 中写入排除规则。
- [Risk] “200 个真实可用模板”和 Demo 数据集需要大量资产准备。Mitigation: 在 template-center 和 security-evidence phase 中把资产清单、版权状态和验收证据作为任务验收条件。
- [Risk] 公网模型、私有化模型、PubMed、公网导入和视觉解析服务依赖外部可用性。Mitigation: 每个相关任务必须同时验证公网路径、私有化/离线路径或明确 fallback，并记录审计。
- [Risk] 医疗安全边界不清会导致高风险输出。Mitigation: security-evidence phase 统一补齐免责声明、脱敏门禁、医生确认和审计证据。

## Migration Plan

1. 使用本 change 的 proposal/spec/design/tasks 作为后续 V1.0 POC 实施入口。
2. 后续实施时优先从 Phase 1 foundation 开始，逐 phase 完成并打勾。
3. 如果需要更细粒度执行，可为单个 phase 创建新的 OpenSpec implementation change，但不得扩大 V1.0 P0 范围。
4. V1.1/V1.2+ 内容保持 roadmap 状态，直到用户明确启动对应版本 change。

## Open Questions

- V1.0 POC 的具体技术栈、前后端框架、数据库和对象存储实例是否已经确定。
- PubMed 离线缓存、Demo 数据集、医学翻译参考译文和 200 个模板资产是否已有现成来源。
- 公网模型、私有化模型、Embedding、Rerank、文档视觉解析服务的实际 provider 优先级是否需要在实施前单独确认。
