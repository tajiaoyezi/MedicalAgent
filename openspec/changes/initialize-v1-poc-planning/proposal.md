## Why

当前项目已有 V1.0 主 PRD，但还缺少可执行的 OpenSpec 初始化计划、阶段依赖和可验收任务清单。需要把主 PRD 的 P0 范围转成 V1.0 POC 的 phase 与 task，同时避免把历史遗漏清单、V1.1 路线图或 V1.2+ 后续规划误纳入首期实施。

## What Changes

- 建立 V1.0 POC 的规划规格，统一约束需求源、P0 范围、phase 顺序、task 字段和 roadmap 隔离规则。
- 按主 PRD `0.4 后续 phase / task 生成规则` 初始化九个 phase：foundation、onlyoffice-bridge、model-and-parse、aimed-rag-citation、ai-panel-recent-tasks、knowledge-admin、medical-translation、template-center、security-evidence。
- 只从主 PRD `22.1 P0 必做` 生成 V1.0 POC 实施 task。
- 将主 PRD `22.2 V1.1 路线图` 和 `22.3 V1.2+ 后续规划` 只记录为 roadmap / backlog，不进入 V1.0 POC checkbox task。
- 明确医疗数字员工 V1.0 边界：只允许保留规划入口或即将上线说明，不生成创建、运行、编排、工具调用、执行历史相关 task。
- 为每个 task 固定验收字段：目标、范围、前置依赖、验收标准、测试/验证方式、风险。

## Capabilities

### New Capabilities

- `v1-poc-planning`: 约束 V1.0 POC 的唯一需求源、P0 范围、phase/task 生成规则、医疗数字员工边界和 V1.1/V1.2+ roadmap 隔离规则。

### Modified Capabilities

- 无。当前 `openspec/specs/` 尚无已归档能力规格，本 change 只新增 V1.0 POC 初始化规划能力。

## Impact

- 影响 OpenSpec 规划产物：`proposal.md`、`specs/v1-poc-planning/spec.md`、`design.md`、`tasks.md`。
- 影响后续实施方式：后续 `/opsx:apply` 或人工实施必须按九个 phase 和 task 字段执行。
- 不直接修改应用代码、数据库或运行时配置。
- 不使用 `docs/MedOffice_AI_遗漏项与补充需求清单_V1.0.md` 作为 V1.0 POC 执行范围输入。
