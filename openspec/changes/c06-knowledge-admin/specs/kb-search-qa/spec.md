## ADDED Requirements

### Requirement: 全局搜索（关键词/语义/混合 + 多维筛选）
系统 SHALL 提供全局搜索，支持关键词搜索、语义搜索与混合搜索三种检索模式，并支持按知识库、文档类型、更新时间、来源与权限多维筛选。混合搜索 MUST 合并 BM25 关键词检索与向量检索结果并去重。所有检索结果 MUST 经权限过滤后返回。

PRD §11.6「按文档类型筛选」与「按来源筛选」为两个独立筛选维度，本能力 MUST 用两个不同的承载字段区分二者、MUST NOT 用同一字段兼表两维：
- 「文档类型」维 MUST 取自 `kb_documents.document_id` 关联的 c01 `documents` 行的文件类型（取值为 §8.6.4/§8.6.5 支持的文件格式 pdf/ofd/doc/docx/xlsx/xls/ppt/pptx/png/jpg 等），表达文件载体类型，本能力不在 `kb_documents` 上另建重复列。
- 「来源」维 MUST 取自 §16.3 chunk 元数据/`kb_documents` 的 `source_type`（来源类型，如上传/URL/PubMed/PMC/白名单官网），表达资料来源渠道。

两维取值域互不重叠，筛选时可独立施加且各自命中正确字段。

#### Scenario: 关键词搜索返回命中
- **WHEN** 用户以关键词搜索模式输入查询词
- **THEN** 系统 SHALL 通过 BM25 关键词检索返回命中的文档/chunk 列表

#### Scenario: 语义搜索返回相关结果
- **WHEN** 用户以语义搜索模式输入自然语言查询
- **THEN** 系统 SHALL 通过向量检索返回语义相关的 chunk 列表

#### Scenario: 混合搜索合并去重
- **WHEN** 用户以混合搜索模式查询
- **THEN** 系统 SHALL 合并 BM25 关键词检索与向量检索结果、去重并 rerank 后返回

#### Scenario: 多维筛选生效
- **WHEN** 用户在搜索时按知识库、文档类型、更新时间、来源或权限设置筛选条件
- **THEN** 系统 SHALL 仅返回同时满足全部筛选条件的结果

#### Scenario: 按文档类型筛选命中文件类型字段
- **WHEN** 用户选定「文档类型=docx」作为筛选条件
- **THEN** 系统 SHALL 仅返回其 `kb_documents.document_id` 关联 c01 `documents` 文件类型为 docx 的结果
- **AND** 该筛选取自文件类型字段，与「来源」维（`source_type`）互不混同

#### Scenario: 按来源筛选命中来源类型字段
- **WHEN** 用户选定「来源=PubMed」作为筛选条件
- **THEN** 系统 SHALL 仅返回 `source_type`（来源类型）为 PubMed 的结果
- **AND** 该筛选取自来源类型字段，与「文档类型」维（文件类型）互不混同

#### Scenario: 检索结果按权限过滤
- **WHEN** 用户检索时其对部分文档/chunk 无访问权限
- **THEN** 系统 SHALL 不在结果中返回该用户无权访问的文档与 chunk

### Requirement: 多库选择的知识库问答流程
系统 SHALL 支持用户选择一个或多个知识库后发起问答，问答流程 MUST 依次执行：检索相关 chunk → rerank → 生成答案 → 标注引用 → 展示来源文档。生成答案 MUST 基于检索到的 chunk，MUST NOT 输出无依据的诊疗建议，且答案 SHALL 展示医疗免责声明并默认标记为草稿/辅助建议。该「草稿/辅助建议」标记 MUST 按 c09 security-compliance「医疗安全边界与系统定位约束」标记契约执行（owner=c09，本能力为消费方），与 c04 AIMed 答案、c07 医学翻译文书三类 message 级生产方对草稿标记的 owner 引用口径一致，本能力 MUST NOT 自定义该标记语义。

#### Scenario: 跨多库问答
- **WHEN** 用户勾选多个知识库并输入问题
- **THEN** 系统 SHALL 在所勾选且用户有权访问的知识库范围内检索 chunk、rerank、生成答案并标注引用与展示来源文档

#### Scenario: 答案展示免责声明并标注草稿
- **WHEN** 系统生成知识库问答答案
- **THEN** 答案 SHALL 展示医疗免责声明
- **AND** 默认标记为草稿/辅助建议，不作为自动诊断或处方依据
- **AND** 该草稿/辅助建议标记 owner=c09 security-compliance（「医疗安全边界与系统定位约束」标记契约），本能力为消费方、不自定义该标记语义

#### Scenario: 无依据时不臆造答案
- **WHEN** 检索未能召回与问题相关的 chunk
- **THEN** 系统 SHALL 提示未找到可溯源依据，而非编造无引用的诊疗建议

#### Scenario: 检索流程遵循权限过滤前置
- **WHEN** 系统执行问答检索
- **THEN** 检索流程 SHALL 在 BM25 关键词检索与向量检索之前先执行权限过滤（Query Rewrite → 数据源选择 → 权限过滤 → 检索 → 合并去重 → rerank → 生成带引用答案）

### Requirement: 知识库问答高风险答案下发前的人工确认（消费 c05 message 级确认链路）
知识库问答答案与 AIMed 答案同样落 c04 所建 `conversations`/`messages` 表（`module=kb_qa`），是 message 级医学文书，其中药品说明书/用药安全、临床指南、临床路径等预置库的问答会直接产出用药/诊疗类高风险结论。因此知识库问答答案在下发给用户前，当命中高风险（诊疗、用药、医嘱、临床文书或患者个体信息）时 MUST 接入 c05 ai-writeback-confirmation 的 message 级高风险确认链路（以 `message_id` 为键），与 AIMed 答案、医学翻译文书复用同一条确认链路。`risk_type` 高风险判定与 `confirmed_role` 角色裁决的唯一 owner 为 c05 服务端，本能力 MUST NOT 自建高风险判定或确认记录，仅作为该链路的 message 级生产方前置消费：本能力 SHALL 在知识库问答答案下发前将待下发内容交由 c05 服务端 `risk_type` 分类器判定。该 message 级确认链路的生产方枚举（AIMed 答案 / 知识库问答 kb_qa / 医学翻译文书三类 message 产生方）以 c05 ai-writeback-confirmation 与 c09 security-compliance 的 owner 枚举为唯一真值，本能力（kb_qa）仅作为该链路的 message 级生产方挂载、以 owner 枚举为准前置消费，不另行重定义该链路的生产方清单或裁决口径。命中高风险时，确认 MUST 按 `confirmed_role∈{doctor,reviewer}` 裁决并以 `message_id` 为键落 c05 所建 `writeback_confirmations`，普通用户只能生成草稿或提交审核、MUST NOT 完成最终确认与下发；具备 `doctor` 或 `reviewer` 角色者方可确认下发。确认记录与审计由 c05 owner 写入，本能力仅触发该链路并记录问答行为到 `audit_logs`。

#### Scenario: 高风险知识库问答答案需医生或审核角色确认后下发
- **WHEN** 知识库问答（`module=kb_qa`）生成的答案被 c05 服务端 `risk_type` 分类器识别为高风险（命中诊疗/用药/医嘱/临床文书/患者个体信息）
- **THEN** 系统 MUST 在答案下发前进入 c05 message 级高风险确认链路（以 `message_id` 为键），按 `confirmed_role∈{doctor,reviewer}` 裁决
- **AND** 普通用户 MUST NOT 完成最终确认与下发，仅能生成草稿或提交审核

#### Scenario: 高风险判定与确认记录归 c05、c06 仅前置消费
- **WHEN** 知识库问答答案下发前接入高风险确认链路
- **THEN** `risk_type` 判定与 `writeback_confirmations` 确认记录 MUST 由 c05 owner 写入，本能力 MUST NOT 自建判定或确认表
- **AND** 本能力仅触发该链路并将问答行为写入 `audit_logs`

### Requirement: 知识库问答生成调用公网模型前 PHI/PII 脱敏门禁（消费 c09 redaction-gateway）
PHI/PII 识别与脱敏引擎（redaction-gateway）的唯一 owner 为 c09 security-evidence；本能力 MUST NOT 自行实现 PHI/PII 识别脱敏，仅在公网出口消费该门禁接缝。知识库问答的答案生成路径会用用户问题（可能含姓名/住院号/门诊号/医保号等 PHI/PII）及检索注入上下文调用公网模型，因此当问答生成需调用公网模型时，系统 SHALL 先消费 c09 redaction-gateway 对发送内容做 PHI/PII 识别与脱敏判定；当识别失败、脱敏置信度不足、识别服务不可用或 redaction-gateway 未接入时，系统 MUST NOT 调用公网模型，MUST 切换至 c03 私有化模型路径继续生成带引用答案（本期默认公网关闭、私有化优先，§16.4/§24.9）。脱敏命中与策略由 c09 redaction-gateway 在 c03 公网出口统一写入 `privacy_redaction_events`，本能力仅消费门禁判定、不另维护该表字段口径，问答调用留痕写 `audit_logs`。本门禁与 kb-import 导入侧门禁为两个独立执行点（导入入向量化前 vs 问答生成调用公网模型前）。

#### Scenario: 问答生成含 PHI 时脱敏后调用公网模型
- **WHEN** 知识库问答的用户问题或检索注入上下文含潜在 PHI/PII 且需使用公网模型生成答案
- **THEN** 系统 SHALL 先经 c09 redaction-gateway 识别脱敏，确认无残留敏感信息后才调用公网模型，脱敏事件由该门禁写入 `privacy_redaction_events`

#### Scenario: 识别失败或不可用时禁用公网切私有化
- **WHEN** c09 redaction-gateway 识别失败、脱敏置信度不足或 redaction-gateway 尚未接入/不可用
- **THEN** 系统 MUST NOT 调用公网模型
- **AND** SHALL 切换至 c03 私有化模型路径完成带引用答案生成，并记录到 `audit_logs`

### Requirement: 答案溯源与引用定位
本能力复用 c04 citation-tracing 的引用结构与定位机制，**引用溯源阈值以 PRD 第 20.3 节与 c04 citation-tracing 为唯一真值**（引用源定位成功率 ≥ 90%、引用定位页码误差 ≤ 1 页、引用可点击率 ≥ 95%），本 spec 不另行重定义阈值，仅追加知识库多库选择/多维筛选差异。知识库问答答案 MUST 可溯源并定位到：知识库、源文档、章节、页码、段落、chunk 与原文片段。引用 SHALL 可点击跳转到来源。

#### Scenario: 溯源到段落级
- **WHEN** 用户查看问答答案的某条引用
- **THEN** 系统 SHALL 定位到对应的知识库、源文档、章节、页码、段落、chunk 与原文片段

#### Scenario: 引用可点击跳转
- **WHEN** 用户点击答案中的引用标记
- **THEN** 系统 SHALL 跳转或展示该引用对应的来源文档位置
- **AND** 引用可点击率 SHALL ≥ 95%

#### Scenario: 引用定位准确率达标
- **WHEN** 对知识库问答测试集运行验收
- **THEN** 引用源定位成功率 SHALL ≥ 90%
- **AND** 引用定位页码误差 SHALL ≤ 1 页

#### Scenario: 问答历史进入最近任务
- **WHEN** 用户完成一次知识库问答
- **THEN** 该问答会话 SHALL 进入最近任务，可被恢复

### Requirement: 知识库问答会话持久化与最近任务写入
知识库问答会话的持久化与最近任务写入由本 phase（c06）负责，c06 为知识库问答会话语义在 §18「问答历史」中的唯一写入方。c06 MUST 复用 c04 所建的 `conversations`/`messages` 表持久化知识库问答会话与消息，并通过 c04 在 `conversations` 上提供的 `module`/`source` 两个独立维区分会话来源：`module`（机器枚举值）MUST 取 c04 owner 定义的 `kb_qa`、`source`（§6.4 中文规范值）MUST 取「医疗知识库问答」，二者为不同字段，MUST NOT 用同一字面量同时表述。`module` 取值域 {aimed, kb_qa} 由 c04 owner 唯一定义，c06 写入时 MUST 取 `kb_qa`，使 c05 恢复编排可按 `module=kb_qa` 识别本会话并提供「继续追问」。c06 MUST NOT 自建知识库问答会话表，也 MUST NOT 把知识库会话写成 AIMed 模式。每次知识库问答成功后，c06 MUST 向 c01 所建 `recent_tasks` 表写入一条 `source=医疗知识库问答`、`ref_type=conversation`、`ref_id=conversation_id`、按 `tenant_id`/`user_id` 隔离的记录，`source` 取值 MUST 为 c01 recent-tasks-shell 定义的 §6.4 规范值「医疗知识库问答」；写入 MUST 以 `(ref_type, ref_id)` 为幂等键 upsert（同一会话多次问答只更新同一条最近任务）。最近任务的展示规则（§6.5）与跨源恢复编排（§6.6）归 c05 recent-tasks，本能力仅负责知识库问答侧的会话持久化与条目写入。

#### Scenario: 知识库问答会话持久化到 conversations/messages
- **WHEN** 用户完成一次知识库问答
- **THEN** 系统 SHALL 将该会话与消息写入 c04 所建 `conversations`/`messages`，并在 `conversations` 上标记 `module=kb_qa`（机器枚举值，c04 owner 定义）、`source=「医疗知识库问答」`（§6.4 中文规范值）
- **AND** 该会话 MUST NOT 被标记为 AIMed 模式（`module≠aimed`），且按 `tenant_id`/`user_id` 隔离，c05 恢复编排可按 `module=kb_qa` 识别并恢复

#### Scenario: 知识库问答写入最近任务
- **WHEN** 一次知识库问答成功
- **THEN** 系统 SHALL 向 `recent_tasks` 写入一条 `source=医疗知识库问答`、`ref_type=conversation`、`ref_id=conversation_id`、带 `tenant_id`/`user_id` 隔离的记录
- **AND** 以 `(ref_type, ref_id)` 为幂等键 upsert，同一会话重复问答只更新同一条最近任务

#### Scenario: 最近任务可回源恢复该会话
- **WHEN** c05 recent-tasks 按某条 `source=医疗知识库问答` 的 `ref_id=conversation_id` 发起恢复
- **THEN** 系统 SHALL 能依据该 `conversation_id` 从 `conversations`/`messages` 取回问答记录、知识库选择、检索源与引用段落（§6.6）供 c05 恢复编排消费

### Requirement: RAG 权限过滤维度
本能力**复用 c04 rag-retrieval 召回前权限过滤契约为唯一真值**，本 spec 不另行重定义过滤执行逻辑，仅补充知识库特有的 `kb_id` 多库选择与 `document_acl`/`chunk_acl` 两维落点差异。RAG 检索（含全局搜索与知识库问答）时系统 MUST 强制按以下六维过滤：tenant_id、kb_id、user_id、role、document_acl、chunk_acl，其中 `document_acl`（文档级，落 `document_permissions`）与 `chunk_acl`（chunk 级，落 c03 所建 `document_chunks.chunk_acl` 列，c06 仅写值）是两个独立维度，由 c04 rag-retrieval 在召回前分别执行（本 phase 把知识库可见性映射到这两维或注入基于 `document_permissions` 预计算的可见 document 集合，不在应用层各自维护过滤）。任一维度不满足的内容 MUST NOT 进入检索结果、rerank 与答案生成上下文。

#### Scenario: 六维度强制过滤
- **WHEN** 系统执行任意 RAG 检索
- **THEN** 系统 SHALL 把 tenant_id、kb_id、user_id、role、可见 document 集合（document_acl 维）与 chunk_acl 维下推至 c04 rag-retrieval 召回前权限过滤步骤
- **AND** 六维任一不满足的候选内容均被 c04 在召回阶段过滤

#### Scenario: document 级 ACL 隔离
- **WHEN** 用户对某文档整体不具备 document_acl 访问权限
- **THEN** 系统 SHALL 不将该文档的任何 chunk 纳入候选集、rerank 与答案生成上下文
- **AND** 该文档不出现在引用与来源文档列表中

#### Scenario: chunk 级 ACL 隔离
- **WHEN** 同一文档内部分 chunk 设置了更严格的 chunk_acl 且用户无权访问
- **THEN** 系统 SHALL 不将该 chunk 注入答案生成上下文，也不在引用中暴露其原文片段

#### Scenario: 跨租户检索被隔离
- **WHEN** 某租户用户的检索请求可能命中其它租户的知识库内容
- **THEN** 系统 SHALL 按 tenant_id 过滤，不返回任何跨租户内容

#### Scenario: 越权内容不进入引用
- **WHEN** 候选 chunk 因 document_acl 或 role 不匹配被过滤
- **THEN** 该内容 MUST NOT 出现在答案、引用或来源文档列表中

### Requirement: 检索与问答行为审计
系统 SHALL 将检索与问答行为写入 `audit_logs` 并生成问答日志，记录用户、tenant_id、所选 kb_id、查询、返回引用与时间，问答日志 SHALL 可供管理员查看。

#### Scenario: 问答写入审计与问答日志
- **WHEN** 用户完成一次知识库问答
- **THEN** 系统 SHALL 写入 `audit_logs` 并生成问答日志，包含用户、tenant_id、所选 kb_id、查询与返回引用

#### Scenario: 管理员查看问答日志
- **WHEN** 管理员在后台查看问答日志
- **THEN** 系统 SHALL 展示其权限范围内的问答记录与对应引用来源
