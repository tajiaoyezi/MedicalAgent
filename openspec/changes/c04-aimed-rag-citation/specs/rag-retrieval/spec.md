## ADDED Requirements

### Requirement: RAG 检索流程编排
RAG 检索 SHALL 按固定流程执行：Query Rewrite → 数据源选择 → 权限过滤 → BM25 关键词检索 → 向量检索 → 合并去重 → rerank → source compression → 注入 Agent 上下文 → 生成带引用答案。数据源选择 MUST 遵循当前 AIMed 模式的数据源约束（由声明式 `mode_policy` 的 allow_pubmed/allow_upload/allow_kb/allow_current_doc 维驱动），仅在该模式允许的数据源（PubMed/离线缓存、上传文件、医疗知识库、当前文档）中检索。其中医疗知识库（allow_kb 维）按 §8.2 仅通用问答（general）可用，深度文献伴读 / 科研态势分析 / 循证证据溯源 / 智能综述生成 / 学术写作辅助五模式 MUST NOT 检索医疗知识库，KB 内容 MUST NOT 进入候选集、rerank、上下文注入或引用。BM25 与向量检索结果 MUST 合并去重后再 rerank。注入上下文的内容 MUST 携带可溯源标识，使生成答案的关键结论可附引用角标。PubMed RAG Hit@5 验收目标 MUST ≥ 80%。

#### Scenario: 完整检索流程产出带引用上下文
- **WHEN** 用户在允许检索的模式发送医学问题
- **THEN** 系统依次执行 Query Rewrite、数据源选择、权限过滤、BM25 与向量检索、合并去重、rerank、source compression，并注入带可溯源标识的上下文
- **AND** 生成的关键结论可附引用角标定位来源

#### Scenario: 数据源选择遵循模式约束
- **WHEN** 当前模式数据源约束仅含 PubMed
- **THEN** 数据源选择阶段 MUST 仅选择 PubMed/离线缓存，排除上传文件与知识库

#### Scenario: 智能综述生成/学术写作辅助排除医疗知识库
- **WHEN** 当前模式为智能综述生成或学术写作辅助（含上传文件但 allow_kb=✗，不含医疗知识库）
- **THEN** 数据源选择 MUST 排除医疗知识库，检索结果与引用中均不出现 KB 内容
- **AND** 上传文件（及 PubMed/当前文档）等该模式允许的数据源仍可正常检索

#### Scenario: 通用问答可检索医疗知识库
- **WHEN** 当前模式为通用问答（general，allow_kb=✓）且问题命中医疗知识库内容
- **THEN** 数据源选择 MUST 将医疗知识库纳入检索范围，命中的 KB chunk 可进入候选集并按权限过滤后参与 rerank 与引用

#### Scenario: BM25 与向量结果合并去重再 rerank
- **WHEN** BM25 与向量检索返回重叠 chunk
- **THEN** 系统合并去重后再做 rerank，最终上下文中同一来源不重复出现

### Requirement: 消费 c03 索引就绪事件构建检索索引
RAG 检索的索引层 SHALL 消费 c03 解析流水线在 `indexing_handoff` 阶段发出的「索引就绪」事件，据此为对应 document_id/version_id 的 `document_chunks` / `embeddings` 构建/刷新 BM25 全文索引与向量倒排索引，供后续检索使用。c03 仅写入 chunk + embedding 并发出「索引就绪」事件、不构建检索索引；本能力作为该事件的唯一检索侧消费方，MUST 在收到事件后完成 BM25 全文索引与向量索引的构建/装载，使新解析内容进入可检索状态。索引构建 MUST 按 (document_id, version_id) 幂等，重复事件不重复构建，旧版本索引在新版本就绪后失效/被替换。

#### Scenario: 收到索引就绪事件构建检索索引
- **WHEN** c03 对某 document_id/version_id 完成 chunk + embedding 写库并发出「索引就绪」事件
- **THEN** 本能力消费该事件，为该版本的 chunk 构建/刷新 BM25 全文索引与向量倒排索引，使其进入可检索状态
- **AND** 索引构建按 (document_id, version_id) 幂等，重复事件不重复构建

#### Scenario: 新版本索引就绪后旧索引失效
- **WHEN** 同一 document_id 的更高版本「索引就绪」事件到达
- **THEN** 本能力以新版本索引替换/失效旧版本索引，检索仅命中最新已就绪版本

### Requirement: chunk 元数据
检索使用的文档 chunk SHALL 包含完整元数据字段：document_id、source_type、source_title、source_url、pubmed_id、doi、journal、year、section、page、paragraph_index、chunk_text、embedding、chunk_acl。其中 chunk_acl 是 chunk 级 ACL，即 c03 document_chunks 表的 chunk_acl 物理列（唯一 owner=c03，默认继承来源文档 ACL，可写入严于文档级的范围），本 phase 仅消费不建该列。document_acl（文档级 ACL，源自 document_permissions）不是 chunk 自带字段，而是 §11.9 的独立文档级过滤维度，由「检索权限与多租户过滤」Requirement 按 document_permissions 派生执行。引用溯源 MUST 依据这些元数据定位来源（如 page/paragraph_index 用于上传文件页码段落定位，pubmed_id 用于 PubMed 文章定位）。

#### Scenario: chunk 携带溯源定位字段
- **WHEN** 系统检索到一个上传文件来源的 chunk
- **THEN** 该 chunk 元数据包含 document_id、source_type、page、paragraph_index、section 等字段，可用于页码段落定位

#### Scenario: PubMed chunk 携带 PMID
- **WHEN** 系统检索到一个 PubMed 来源的 chunk
- **THEN** 该 chunk 元数据包含 pubmed_id、source_title、journal、year，可用于 PubMed 文章定位

### Requirement: 检索权限与多租户过滤
RAG 检索的权限过滤阶段 SHALL 按 tenant_id / kb_id / user_id / role / document_acl / chunk_acl 六维过滤 chunk（对齐 §11.9），用户 MUST NOT 检索到无权访问的内容。其中 document_acl 与 chunk_acl 是两个独立维度：document_acl 维按文档级权限执行，chunk_acl 维按 chunk 元数据执行；二者均 MUST 在 BM25/向量检索结果进入 rerank 与上下文注入之前生效，未授权 document 与未授权 chunk MUST 在结果与引用中均不出现。document_acl 维对「上传文件 / 当前文档 / 团队文档」等 c04 自有来源 MUST 由本能力直接按 c01 `document_permissions`（owner=c01，phase 1 即就绪）派生的可见 document 集合在本 phase 独立执行，不依赖 c06；对知识库（kb）来源，document_acl 维可由 c06 注入基于 `document_permissions` 预计算的可见 document 集合作为优化路径，但该 c06 注入仅是 kb 来源的执行优化、MUST NOT 作为 document_acl 维在所有来源上的唯一执行方。

#### Scenario: 越权 chunk 被 chunk_acl 维过滤
- **WHEN** 用户检索时命中其无 chunk_acl 权限的知识库 chunk
- **THEN** 权限过滤阶段在 chunk_acl 维移除该 chunk，结果与引用中均不出现该内容

#### Scenario: 越权文档被 document_acl 维过滤
- **WHEN** 用户检索命中的 chunk 属于其不具备 document_acl 访问权限的知识库（kb）文档（不在 c06 注入的可见 document 集合内）
- **THEN** 权限过滤阶段在 document_acl 维移除该文档的全部 chunk，未授权文档不进入候选集、rerank 与引用

#### Scenario: c04 自有来源文档不依赖 c06 的 document_acl 过滤
- **WHEN** 用户对 AIMed 上传文件 / 当前文档 / 团队文档来源检索且命中其不具 `document_permissions` 访问权的文档
- **THEN** 本能力直接按 c01 `document_permissions`（phase 1 即就绪）派生的可见 document 集合在 document_acl 维移除该文档全部 chunk，未授权文档不进入候选集、rerank 与引用
- **AND** 该过滤在本 phase（c04）独立生效、MUST NOT 依赖 c06 注入

#### Scenario: 跨租户内容隔离
- **WHEN** 用户检索的候选 chunk 含其它 tenant_id 的内容
- **THEN** 系统按 tenant_id 过滤，仅返回本租户可访问内容

#### Scenario: 权限过滤先于上下文注入
- **WHEN** BM25 与向量检索返回候选 chunk
- **THEN** 系统先执行 tenant_id/kb_id/user_id/role/document_acl/chunk_acl 过滤，再进入 rerank 与上下文注入

### Requirement: 公网模型调用前脱敏门禁
当 RAG 注入上下文后需调用公网模型生成答案时，系统 SHALL 消费 c09 的 redaction-gateway（PHI/PII 识别与脱敏引擎，唯一 owner=c09）对发送内容做识别与脱敏；本 phase 不自行实现 PHI/PII 识别脱敏，仅在公网出口调用该门禁接缝。识别失败、置信度不足、识别服务不可用或 redaction-gateway 未接入时，系统 MUST NOT 调用公网模型，MUST 切换至 c03 私有化模型路径继续生成（本期默认公网关闭、私有化优先）。脱敏命中与策略由 c09 redaction-gateway 在公网出口统一写入 privacy_redaction_events，本 phase 仅消费门禁判定、不另维护该表字段口径。

#### Scenario: 注入上下文含 PHI 时脱敏后调用
- **WHEN** 注入上下文或用户问题含潜在 PHI/PII 且使用公网模型
- **THEN** 系统先经 c09 redaction-gateway 脱敏再调用公网模型生成答案，脱敏事件由该门禁写入 privacy_redaction_events

#### Scenario: 脱敏失败切私有化模型
- **WHEN** PHI/PII 识别失败或服务不可用
- **THEN** 系统禁止调用公网模型，改用私有化模型完成生成
