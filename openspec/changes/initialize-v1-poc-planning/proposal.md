## Why

当前项目需要把主 PRD 的 V1.0 POC 范围转成可审查的 OpenSpec 规划基线，但不能让规划 change 被 `openspec apply` 误识别为立即实施 47 个业务任务。需要重新生成规划产物，区分“本 change 的规划任务”和“后续 implementation change 的业务任务”。

## What Changes

- 重建 `initialize-v1-poc-planning`，仅作为 V1.0 POC 项目初始化和阶段规划 change。
- 固定唯一权威需求源为 `docs/MedOffice_AI_完整产品需求文档_V1.0.md`。
- 从主 PRD `22.1 P0 必做` 生成 V1.0 POC phase/task 计划，但这些未来实施项不使用 `- [ ]` checkbox。
- 将主 PRD `22.2 V1.1 路线图` 和 `22.3 V1.2+ 后续规划` 记录为 roadmap/backlog，不进入 V1.0 POC 当前实施任务。
- 保留医疗数字员工的规划入口边界，不生成创建、运行、编排、工具调用、执行历史相关实施任务。
- 明确后续每个 phase 应单独创建 implementation change，再用 checkbox task 执行。

## Capabilities

### New Capabilities

- `v1-poc-planning`: 约束 V1.0 POC 的需求源、P0 范围、phase/task 计划、规划型 change 与实施型 change 的边界、医疗数字员工排除规则和 roadmap/backlog 隔离规则。

### Modified Capabilities

- 无。当前 `openspec/specs/` 尚无已归档能力规格，本 change 只新增 V1.0 POC 规划能力。

## Impact

- 影响 OpenSpec 规划产物：`proposal.md`、`specs/v1-poc-planning/spec.md`、`design.md`、`tasks.md`。
- 不直接修改应用代码、数据库、运行时配置或部署资源。
- 后续实施必须从本规划派生新的 phase implementation change，例如 `implement-foundation-phase`。
- `openspec apply initialize-v1-poc-planning` 不应启动任何 V1.0 业务功能开发任务。
