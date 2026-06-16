# Recent Tasks Specification

## Purpose
最近任务完整能力（建于 c01 recent-tasks-shell 列表壳之上）：六类来源混合（AIMed / 医疗知识库问答 / 医疗数字员工占位 / 医学翻译 / 在线文档 AI 操作 / 模板生成文档）、在线文档 AI 操作（doc_ai）条目写入（写入侧 owner=c05，`ref_type`=`writeback_confirmation` / `ref_id`=`writeback_ref`，按操作粒度幂等 upsert）、六类来源各自的恢复内容（§6.6）、六类来源列表项操作差异（仅会话来源可继续追问）、删除时按来源解析关联文档的差异处理。§6.5 展示规则与 §6.7 删除规则的唯一真值归 c01 recent-tasks-shell，本能力遵循其同名 Scenario、MUST NOT 重复定义。

## Requirements

### Requirement: 最近任务混合六类来源
系统 SHALL 在最近任务中混合展示六类来源产生的历史记录（§6.4）：AIMed 学术助手、医疗知识库问答、医疗数字员工、医学翻译、在线文档 AI 操作、模板生成文档。每条最近任务记录 MUST 标识其来源模块，并按 tenant_id / user_id 隔离，用户 MUST NOT 看到其它租户或非授权用户的任务。医疗数字员工在 V1.0 仅作来源占位，其创建 / 运行 / 编排不在本能力范围内。

#### Scenario: 六类来源汇聚到统一列表
- **WHEN** 用户打开最近任务
- **THEN** 系统按 tenant_id / user_id 过滤后，将上述六类来源的历史记录汇聚为统一列表，每条记录标识其来源模块
- **AND** 列表 MUST NOT 包含其它租户或非授权用户的任务

#### Scenario: 数字员工来源占位
- **WHEN** 最近任务中存在医疗数字员工来源的历史
- **THEN** 系统仅展示其历史条目，MUST NOT 在本能力内提供数字员工的创建 / 运行 / 编排入口

### Requirement: 在线文档 AI 操作写入最近任务
在线文档 AI 操作（doc_ai）来源的最近任务条目写入由本能力（c05）负责（写入侧 owner=c05）。一次在线文档 AI 操作（写回确认 / 文档内 AI 处理）产生或更新时，系统 MUST 向 c01 所建 `recent_tasks` upsert 一条记录，`source` 使用 c01 枚举规范值「在线文档 AI 操作」、`ref_type`=`writeback_confirmation`、`ref_id`=`writeback_ref`（指向 `writeback_confirmations` 的单次写回确认记录 id，按操作粒度区分；同一文档的多次不同 AI 操作 / 选区各对应不同 `writeback_ref`，故为各自独立条目），按 `tenant_id` / `user_id` 隔离，并以 (`ref_type`,`ref_id`) 为幂等键保证同一次操作重复投递不产生重复条目（§6.6 在线文档 AI 按单次操作恢复选区 / AI 操作类型 / 输出结果 / 写回记录）。

doc_ai 条目的 `title` 取值规则由本能力定义：§6.5「标题按任务最初提出时用户输入的原始问题生成」对在线文档 AI 操作不直接适用（doc_ai 由文档内 AI 操作触发、无「用户原始问题」），故 doc_ai 条目的 `title` MUST 取「AI 操作类型 + 目标文档名」组合（如「全文润色 · 病例分析报告.docx」），其中 AI 操作类型取该次写回确认记录的 `operation_type`（全文润色 / 选区润色 / 校对 / 选区翻译 / 补引用 / 插入标注 / AI 论文排版）、目标文档名取 `writeback_confirmations.document_id` 回源的文档名；当首次操作为选区类且需更可辨识时，MAY 在文档名后附首次操作的选区 / 输入摘要（截断展示）。目标文档名缺失（如文档已删）时 MUST 回退为仅「AI 操作类型」。`title_preview` 仍按 §6.5 取该 `title` 前 10 字、悬浮展示完整 `title`（展示规则唯一真值归 c01 recent-tasks-shell）。`ref_type` 取值 MUST 与 `ref_id` 实际回源表唯一对应：doc_ai 取 `ref_type=writeback_confirmation`（回源 `writeback_confirmations`），区别于 c08 模板生成来源的 `ref_type=document`（回源 `documents` 行主键），使恢复分发器仅凭 `ref_type` 即可判定回源表、MUST NOT 出现同一 `ref_type` 指向两张不同源表的过载；任何消费方 MUST NOT 按 `ref_type=document` 直推 `ref_id` 为 `document_id`，关联文档一律经本能力解析 `related_document_id`。其余五类来源（aimed→c04 `ref_type=conversation`、kb_qa→c06 `ref_type=conversation`、translation→c07 `ref_type=translation_job`、template→c08 `ref_type=document`、数字员工占位不写）的写入由各产生来源的对应 change 负责，本能力不替它们写入。

#### Scenario: 在线文档 AI 操作按操作粒度 upsert 最近任务
- **WHEN** 用户完成一次在线文档 AI 操作（如确认写回）
- **THEN** 系统向 `recent_tasks` upsert 一条 source=在线文档 AI 操作、ref_type=writeback_confirmation、ref_id=writeback_ref（指向 `writeback_confirmations` 的单次写回确认记录 id）、带 tenant_id / user_id 隔离的记录
- **AND** 该条 `title` MUST 取「AI 操作类型 + 目标文档名」（AI 操作类型取确认记录 `operation_type`、目标文档名取 `document_id` 回源文档名，如「全文润色 · 病例分析报告.docx」），目标文档名缺失时回退为仅「AI 操作类型」，`title_preview` 取该 `title` 前 10 字
- **AND** 对同一 (ref_type, ref_id) 即同一次写回确认重复投递时按幂等键更新同一条记录，MUST NOT 产生重复条目
- **AND** 同一文档上的不同 AI 操作（不同选区 / 操作类型）因 `writeback_ref` 不同 MUST 各自独立成条，MUST NOT 被折叠为同一条最近任务

#### Scenario: ref_type 唯一对应回源表
- **WHEN** 恢复分发器收到一条 doc_ai 来源任务（ref_type=writeback_confirmation）与一条 template 来源任务（ref_type=document）
- **THEN** 系统仅凭 `ref_type` 即判定回源表：`writeback_confirmation` 回源 `writeback_confirmations`、`document` 回源 `documents`，MUST NOT 因两类共用同一 `ref_type` 而需二次按 source 分支判定
- **AND** 任何消费方 MUST NOT 按 `ref_type=document` 直推 `ref_id` 为 `document_id` 去读 doc_ai 来源；doc_ai 的关联文档一律经本能力解析 `related_document_id`

### Requirement: 最近任务六类来源展示与操作
§6.5 展示规则（标题前 10 字 / 悬浮全标题 / updated_at 倒序 / 今天-7天-30天-1年-全部分组 / 按模块多选筛选）由 c01 recent-tasks-shell 列表壳定义为唯一真值，本能力 MUST NOT 重复定义其 Scenario，遵循 c01 同名 Scenario。本能力（§6.6 六类来源恢复 owner）仅在该列表壳之上补充六类来源的列表项操作差异：列表项操作 MUST 提供查看、继续追问、重命名、删除、批量删除，其中「继续追问」仅对会话类来源（AIMed / 医疗知识库问答）可用；对非会话来源（在线文档 AI / 医学翻译 / 模板生成 / 数字员工占位），系统 MUST NOT 提供「继续追问」入口（不展示或禁用），仅保留查看、重命名、删除、批量删除。

#### Scenario: 非会话来源不提供继续追问
- **WHEN** 用户查看一条非会话来源任务（在线文档 AI / 医学翻译 / 模板生成 / 数字员工占位）的列表项操作
- **THEN** 系统 MUST NOT 提供「继续追问」入口（不展示或禁用），仅保留查看、重命名、删除、批量删除

#### Scenario: 会话来源可继续追问
- **WHEN** 用户对一条会话类来源（AIMed / 医疗知识库问答）任务点击「继续追问」
- **THEN** 系统恢复其会话并在原会话上继续追问

### Requirement: 六类来源的恢复内容
系统 SHALL 按 §6.6 为每类来源恢复其对应内容，最近任务恢复成功率 MUST ≥ 98%：AIMed 恢复问答记录、模式、上传文件、知识库选择、引用资料、Agent 状态；医疗知识库恢复问答记录、知识库选择、检索源、引用段落；医学翻译恢复原文文件、译文文件、语言方向、术语库、翻译进度、历史版本；在线文档 AI 恢复文档 ID、选区、AI 操作类型、输出结果、写回记录；模板生成恢复模板 ID、生成文档、使用时间、编辑状态。数字员工来源在 V1.0 仅作来源占位、显示「规划中」、MUST NOT 提供恢复（其执行历史恢复属 §22.2 V1.1）。恢复涉及的引用与检索源 MUST 可溯源 / 引用定位，且 MUST 按 tenant_id / kb_id / user_id / role / document_acl / chunk_acl 六维过滤（§11.9；六维过滤由 c04 rag-retrieval / c06 召回前执行，本能力消费其结果）。

#### Scenario: 恢复 AIMed 任务
- **WHEN** 用户对一条 AIMed 来源任务点击「查看」或「继续追问」
- **THEN** 系统恢复其问答记录、模式、上传文件、知识库选择、引用资料与 Agent 状态，并可在原会话上继续追问
- **AND** 引用资料 MUST 可点击定位至来源，引用源定位成功率 ≥ 90%

#### Scenario: 恢复在线文档 AI 操作
- **WHEN** 用户对一条在线文档 AI 来源任务点击「查看」
- **THEN** 系统经 `ref_id`=`writeback_ref`（指向 `writeback_confirmations` 的单次操作记录、非裸 `document_id`）回源该次操作，从该确认记录的非哈希字段恢复：文档 ID（`document_id`）、选区（`confirmed_scope`）、AI 操作类型（`operation_type`）、输出结果（`output_version_id` 指向的 `document_versions`）与写回记录（确认记录本体），并可重新定位到该文档的对应选区
- **AND** 同一文档名下的多次不同操作各自按其 `writeback_ref` 逐次恢复，MUST NOT 因 document_id 相同而互相覆盖

#### Scenario: 恢复医学翻译任务
- **WHEN** 用户对一条医学翻译来源任务点击「查看」
- **THEN** 系统恢复原文文件、译文文件、语言方向、术语库、翻译进度与历史版本

#### Scenario: 恢复医疗知识库问答任务
- **WHEN** 用户对一条医疗知识库问答来源任务点击「查看」或「继续追问」
- **THEN** 系统经 ref_id=conversation_id 消费 c06 知识库问答会话，恢复其问答记录、知识库选择、检索源与引用段落（详情数据由 c06 保证，本能力仅负责恢复编排）
- **AND** 检索源与引用段落 MUST 按 tenant_id / kb_id / user_id / role / document_acl / chunk_acl 六维过滤（§11.9），越权来源 MUST NOT 展示

#### Scenario: 恢复模板生成任务
- **WHEN** 用户对一条模板生成来源任务点击「查看」
- **THEN** 系统经 ref_id=document_id（c08 产物）恢复模板 ID、生成文档、使用时间与编辑状态，并打开该生成文档（详情数据由 c08 保证，本能力仅负责恢复编排）

#### Scenario: 恢复时按权限过滤来源
- **WHEN** 用户恢复的任务包含知识库检索源或引用段落
- **THEN** 系统 MUST 按 tenant_id / kb_id / user_id / role / document_acl / chunk_acl 六维过滤（§11.9，六维过滤由 c04 rag-retrieval / c06 召回前执行、本能力消费其结果），越权来源 MUST NOT 在恢复内容中展示
- **AND** 当部分 chunk 设置了严于文档级的 chunk_acl 且用户无权访问时，该 chunk 维内容 MUST NOT 出现在恢复的引用段落中

#### Scenario: 恢复来源已失效
- **WHEN** 任务恢复所需的关联资源（如已删除的文档或翻译文件）不存在
- **THEN** 系统 MUST 提示该部分内容已不可用，并恢复其余可用内容，而非整体失败

### Requirement: 六类来源恢复任务的关联文档差异处理
§6.7 删除规则（二次确认 / 默认不删关联文件 / 仅勾选「同时删除关联文档」才删 / 单条与批量删除 / 删除审计）以 c01 recent-tasks-shell 列表壳为唯一真值，本能力 MUST NOT 重复定义其 Scenario，遵循 c01 同名 Scenario。本能力仅补充「同时删除关联文档」时按各来源 `related_document_id` 解析其关联文档对象的差异：在线文档 AI 来源关联写回生成的文档 / 副本、医学翻译来源关联译文副本、模板生成来源关联生成文档；AIMed / 医疗知识库问答会话来源默认无独立关联文档；数字员工占位来源无关联文档。当来源 `ref_id` 为空时，关联文档解析为空操作。§6.7「同步更新历史记录」的可验收落点由本能力承载：删除一条最近任务（无论 `ref_id` 是否为空、是否勾选「同时删除关联文档」）时，「同步更新历史记录」即对 `recent_tasks` 该条软删（移除出最近任务列表与各来源恢复入口），本能力 MUST NOT 在未勾选「同时删除关联文档」时改动任何来源源表（`conversations` / `messages` / `writeback_confirmations` / `translation_jobs` / `documents` 等），即该软删本身即为「同步更新历史记录」的完整达成。

#### Scenario: 按来源解析关联文档对象
- **WHEN** 用户对某条最近任务确认删除并勾选「同时删除关联文档」
- **THEN** 本能力按该条来源类型解析 `related_document_id`：doc_ai 解析写回生成文档 / 副本、translation 解析译文副本、template 解析生成文档，会话来源（aimed / kb_qa）与数字员工占位无独立关联文档
- **AND** 解析得到的关联文档删除仍交由 c01 列表壳删除规则执行（经 ACL 校验、软删进回收站、写删除审计）；`ref_id` 为空时关联文档解析为空操作

#### Scenario: 删除非空 ref_id 任务时源表不被改动（§6.7 同步更新历史记录）
- **WHEN** 用户删除一条 `ref_id` 非空的最近任务（doc_ai / translation / template / aimed / kb_qa 任一）且不勾选「同时删除关联文档」
- **THEN** 系统对 `recent_tasks` 该条软删完成移除（从最近任务列表与各来源恢复入口消失），§6.7「同步更新历史记录」即以该软删为完整达成
- **AND** 各来源源表（`conversations` / `messages` / `writeback_confirmations` / `translation_jobs` / `documents` 等）MUST NOT 被改动，源实体保留可独立访问
