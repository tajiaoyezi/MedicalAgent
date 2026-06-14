# Recent Tasks Shell Specification

## Purpose
最近任务的唯一聚合表 recent_tasks 与门户最近任务列表壳：展示规则、六来源多选筛选、删除与二次确认、ref_type 弱引用契约；各来源恢复内容映射留后续 phase 扩展。

## Requirements

### Requirement: 最近任务最小数据模型
系统 SHALL 建立 `recent_tasks` 表作为最近任务的唯一聚合表（本 change 为该表唯一建表 owner），最小字段 MUST 至少包含 `task_id`、`tenant_id`、`user_id`、`source`、`title`、`ref_type`、`ref_id`、`updated_at`、`deleted_at`。`source` 枚举规范值由本 change（recent-tasks-shell spec）唯一定义，MUST 对齐 PRD §6.4 六类来源名：`AIMed 学术助手` / `医疗知识库问答` / `医疗数字员工` / `医学翻译` / `在线文档 AI 操作` / `模板生成文档`；所有写入方（c04 写 `AIMed 学术助手`、c06 写 `医疗知识库问答`、c07 写 `医学翻译`、c08 写 `模板生成文档`、c05 写 `在线文档 AI 操作`，数字员工占位不写）MUST 使用上述规范值，MUST NOT 使用缩写或别名。`ref_type` / `ref_id` 为指向具体回源对象的弱引用（本期可为空或仅文档来源填充），用于后续按 `ref_id` 回源恢复；写入侧 MUST 以 `(ref_type, ref_id)` 作为幂等键 upsert。本 change 作为 `ref_type` / `ref_id` 弱引用契约唯一 owner，`ref_type` 取值规范集 MUST 唯一对应其回源表，且每个 `ref_type` 取值 MUST 唯一对应一张回源表（不得过载）：`conversation`（回源 `conversations` 行主键，会话来源）/ `document`（回源 `documents` 行主键，仅指向 documents 表，MUST NOT 指向其它表）/ `translation_job`（回源 `translation_jobs` 行主键，翻译任务来源）/ `writeback_confirmation`（回源 `writeback_confirmations` 行主键，在线文档 AI 写回确认来源——对齐 PRD §6.6 在线文档 AI 恢复内容含「写回记录」，c05 doc_ai 来源以 `ref_type=writeback_confirmation` / `ref_id=writeback_ref` 写入而非 `ref_type=document`）。任何消费方（含列表壳删除、审计、恢复分发器）MUST 仅凭 `ref_type` 即可判定回源表，MUST NOT 按 `ref_type=document` 直推 `ref_id` 为 `document_id` 之外的语义；关联文档一律经 c05 解析的 `related_document_id` 取得，MUST NOT 把非 `document` 取值的 `ref_id` 当作 `document_id` 解析。所有最近任务记录 MUST 按 `tenant_id` / `user_id` 隔离，用户 MUST NOT 看到其它租户或非授权用户的任务。各来源的恢复内容字段映射（§6.6）不在本 change 实现，由 c05 ai-panel-recent-tasks 在本表上扩展，本期只保证表结构能承载且列表壳能展示。

#### Scenario: 表承载六类来源标识与弱引用
- **WHEN** 任一来源向 `recent_tasks` 写入一条记录
- **THEN** 该记录携带对齐 §6.4 的 `source`、`title` 以及 `ref_type` / `ref_id` 弱引用，并带 `tenant_id` / `user_id` 隔离列

#### Scenario: ref_type 唯一对应回源表
- **WHEN** 消费方（列表壳删除 / 审计 / 恢复分发器）按某条最近任务的 `ref_type` 判定回源对象
- **THEN** `ref_type` 取值 MUST 落在规范集 `conversation` / `document` / `translation_job` / `writeback_confirmation` 之内，且各取值唯一对应回源表：`conversation`→`conversations`、`document`→`documents`、`translation_job`→`translation_jobs`、`writeback_confirmation`→`writeback_confirmations`
- **AND** `ref_type=document` 时 `ref_id` MUST 仅为 `documents` 行主键；消费方 MUST NOT 把非 `document` 取值的 `ref_id`（如 `writeback_confirmation` 的 `writeback_ref`）当作 `document_id` 解析，关联文档一律经 c05 解析的 `related_document_id` 取得

#### Scenario: 按租户与用户隔离
- **WHEN** 用户查询最近任务列表
- **THEN** 系统按 `tenant_id` / `user_id` 过滤，MUST NOT 返回其它租户或非授权用户的任务

#### Scenario: 数字员工来源仅占位
- **WHEN** 最近任务中存在医疗数字员工来源的记录
- **THEN** 系统仅将其作为来源占位 / 显示「规划中」，本 change MUST NOT 提供数字员工执行历史的恢复（其执行历史恢复属 §22.2 V1.1）

### Requirement: 最近任务列表壳展示
系统 SHALL 提供门户最近任务列表壳，按 PRD §6.5 实现展示规则：标题首页默认显示前 10 个字、鼠标悬浮显示完整标题；按 `updated_at` 最近更新时间倒序排序；按今天 / 7 天内 / 30 天内 / 1 年内 / 全部分组；支持按 §6.4 六类来源模块多选筛选；列表项壳层操作 MUST 提供查看、重命名、删除、批量删除。列表壳本期只负责展示、分组、筛选与条目壳操作；各来源「继续追问 / 恢复上下文」的回源恢复编排（§6.6）由 c05 ai-panel-recent-tasks 负责，本 change 不实现恢复内容映射。

#### Scenario: 标题截断与悬浮全标题
- **WHEN** 列表展示一条标题超过 10 字的最近任务
- **THEN** 首页默认显示标题前 10 个字，鼠标悬浮时显示该任务的完整标题

#### Scenario: 倒序排序与时间分组
- **WHEN** 用户查看最近任务列表
- **THEN** 系统按 `updated_at` 倒序排列，并分组为今天 / 7 天内 / 30 天内 / 1 年内 / 全部

#### Scenario: 按来源模块多选筛选
- **WHEN** 用户在筛选器中多选若干 §6.4 来源模块
- **THEN** 系统仅展示所选模块的任务，未选中模块的任务被隐藏

#### Scenario: 重命名最近任务标题
- **WHEN** 用户对某条任务选择「重命名」并提交新标题
- **THEN** 系统更新 `recent_tasks.title`，不影响其 `ref_type` / `ref_id` 与回源对象

### Requirement: 最近任务列表壳删除
系统 SHALL 按 PRD §6.7 实现最近任务的删除规则：删除（含单条删除与批量删除）MUST 弹出二次确认；确认后 MUST 对该条 `recent_tasks` 记录自身置 `deleted_at` 软删，本期「确认后移除最近任务」即以该软删完成视为达成。PRD §6.7「同步更新历史记录」在本 phase 收敛为：跨来源历史同步（各来源源表如 conversations/messages、translation_jobs 等的回写）依赖 c05 与各来源源表、不在本 phase 验收范围；本期 `ref_id` 为空时该跨源同步步骤为空操作。默认 MUST NOT 删除已生成的关联文件，仅当用户显式勾选「同时删除关联文档」时才删除关联文档，删除关联文档前 MUST 校验当前用户对该文档具备删除权限并写删除审计。

#### Scenario: 删除前二次确认
- **WHEN** 用户对某条最近任务点击「删除」
- **THEN** 系统弹出二次确认，用户取消则不执行任何删除

#### Scenario: 默认保留关联文档
- **WHEN** 用户确认删除但未勾选「同时删除关联文档」
- **THEN** 系统对该 `recent_tasks` 记录置 `deleted_at` 软删（即视为完成移除），但 MUST NOT 删除已生成的关联文件
- **AND** `ref_id` 为空时跨来源历史同步为空操作，跨源回写依赖 c05 与各来源源表、不在本 phase 验收

#### Scenario: 勾选同时删除关联文档
- **WHEN** 用户确认删除并勾选「同时删除关联文档」
- **THEN** 系统在软删最近任务的同时删除其关联文档，删除前 MUST 校验当前用户对关联文档具备删除权限（按 `tenant_id` / `user_id` / `role` / `acl`），并写入删除审计

#### Scenario: 批量删除二次确认
- **WHEN** 用户多选若干最近任务并执行「批量删除」
- **THEN** 系统弹出二次确认，确认后批量对所选 `recent_tasks` 记录置 `deleted_at` 软删完成移除
