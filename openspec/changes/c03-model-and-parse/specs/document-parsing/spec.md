## ADDED Requirements

### Requirement: 保存/上传后异步发起解析作业
文档上传成功或经 ONLYOFFICE 保存回调创建新版本（§9.8）后，系统 SHALL 异步发起文档解析作业，并在 `document_parse_jobs` 中创建一条作业记录，关联 `document_id` 与触发的 `document_version`。解析 MUST 异步执行，不阻塞上传响应或保存回调（保存回调成功率须 ≥ 99%）。

#### Scenario: 上传后异步创建解析作业
- **WHEN** 用户上传一份 PDF/DOCX 文档且入库成功
- **THEN** 系统立即返回上传成功，并异步在 `document_parse_jobs` 创建一条状态为待处理的作业，关联该 `document_id` 与当前版本

#### Scenario: 保存回调触发新版本解析
- **WHEN** ONLYOFFICE 保存回调写入对象存储并创建新的 `document_version`
- **THEN** 系统不阻塞保存回调返回，异步针对新版本创建解析作业，完成后发出「索引就绪」事件（RAG/BM25 索引构建由 c04/c06 消费该事件完成）

### Requirement: 解析状态机与失败处理
`document_parse_jobs` SHALL 维护明确的状态机，对外至少包含 `pending`、`parsing`、`succeeded`、`failed` 四个状态，成功终态统一为 `succeeded`（design D7 状态机图中的 `detecting / visual_parsing / chunking / embedding / indexing_handoff` 为内部子状态，对外（验收/审计/上层查询）统一归并为 `parsing`；本 spec 不使用 `done` 标识，design 已与本 spec 一致使用 `succeeded` 成功终态、不存在 `done` 别名）。状态流转 MUST 单向推进或在失败后允许重试。解析失败时系统 MUST 记录 `failure_reason`，并将作业置为 `failed`，且 MUST 不写入半成品 chunk。文档上传解析成功率目标 SHALL ≥ 95%。

#### Scenario: 解析成功状态流转
- **WHEN** 一个 `pending` 作业被工作进程取走并完成 chunk 切分入库
- **THEN** 作业状态依次流转为 `parsing` 再到 `succeeded`，并记录完成时间

#### Scenario: 解析失败记录原因且不写半成品
- **WHEN** 解析过程中文档损坏或解析服务异常
- **THEN** 作业状态置为 `failed` 并记录 `failure_reason`，已产生的部分 chunk 被回滚，不写入索引

#### Scenario: 失败作业可重试
- **WHEN** 管理员对一个 `failed` 作业触发重试
- **THEN** 系统重新将作业置为 `pending` 并重新执行解析，重试动作写入审计日志

### Requirement: 文本型文档 chunk 切分与索引就绪事件
对文本型文档（可直接抽取文本的 PDF/DOCX 等），解析作业 SHALL 完成 chunk 切分并写入 `document_chunks`，随后 SHALL 经 `Embed` capability 写入 `embeddings`，并发出「索引就绪」事件。本期 c03 **仅产出 chunk + embedding 并发出「索引就绪」事件，不构建任何检索索引**：BM25 全文索引与向量倒排索引的构建装载属 §16.2 检索流程，由 c04（检索侧）消费该「索引就绪」事件后构建/刷新 BM25 全文索引与向量倒排索引供检索、c06（知识库侧）消费同一事件完成知识库入库与重建索引收尾。该「索引就绪」事件的下游消费方=c04（唯一检索索引构建方），c04 据此消除孤儿事件、闭合写侧→检索侧 handoff。对扫描/复杂文档，解析作业 MUST 以视觉解析服务的结构化输出作为切分输入（见 `visual-parsing-service`），而非直接抽取文本。

#### Scenario: 文本型文档直接切分入库
- **WHEN** 一份可直接抽取文本的 DOCX 进入解析
- **THEN** 系统按段落/章节切分为 chunk 写入 `document_chunks`，经 `Embed` 写入 `embeddings`，并发出「索引就绪」事件（BM25/向量索引构建由 c04/c06 消费该事件完成，c03 不构建检索索引）

#### Scenario: 扫描文档以视觉解析输出为切分输入
- **WHEN** 一份扫描 PDF 无法直接抽取文本进入解析
- **THEN** 系统先调用文档视觉解析服务获取结构化输出，再以该输出（文本/页码/段落/标题层级）作为 chunk 切分依据，而非直接文本抽取

### Requirement: chunk 元数据完整性
写入 `document_chunks` 的每个 chunk SHALL 至少包含 §16.3 规定的元数据字段：`document_id`、`source_type`、`source_title`、`source_url`、`pubmed_id`、`doi`、`journal`、`year`、`section`、`page`、`paragraph_index`、`chunk_text`、`embedding`、`chunk_acl`。其中 §16.3 单一 `acl` 字段在本期正名为 `chunk_acl` 物理列（chunk 级 ACL），由 c03 作为 `document_chunks` 唯一建表 owner 物化，供 c06 写入覆盖值、供 c04 按 `chunk_acl` 维过滤。`document_acl` 维不落 chunk 物理列，而是文档级过滤维度，由 c01 的 `document_permissions` 派生。其中 `page` 与 `paragraph_index` MUST 可用于后续引用定位与溯源；无对应值的字段 MUST 显式置空而非缺省省略。

#### Scenario: chunk 写入完整元数据
- **WHEN** 解析作业为某文档生成 chunk
- **THEN** 每个 chunk 均带有 `document_id`、`source_type`、`section`、`page`、`paragraph_index`、`chunk_text`、`chunk_acl` 等 §16.3 字段，缺失值显式置空

#### Scenario: chunk 携带页码与段落定位
- **WHEN** 一份带页码的 PDF 被切分为多个 chunk
- **THEN** 每个 chunk 记录其来源 `page` 与 `paragraph_index`，供后续引用源定位（页码误差 ≤ 1 页）使用

### Requirement: chunk 的 ACL 与租户隔离继承
每个 chunk SHALL 继承其来源文档的 `tenant_id`，并将 chunk 级 ACL 写入 `document_chunks` 的 `chunk_acl` 物理列。`chunk_acl` 默认继承来源文档级 ACL，但 MUST 允许 c06 写入比文档级更严的范围（即 chunk 级可严于文档级，不得放宽）。后续检索 MUST 能按 §11.9 六维 `tenant_id`、`kb_id`、`user_id`、`role`、`document_acl`、`chunk_acl` 过滤；其中 `document_acl` 由 c01 的 `document_permissions` 派生（经 join / c06 注入预计算可见 document 集合执行）、不落 chunk 物理列，`chunk_acl` 为本表物理列。本期解析入库默认写入与来源文档一致的 `chunk_acl`，不得放宽访问范围。

`embeddings` 表 SHALL 携带 `chunk_id` 外键引用 `document_chunks(id)`，每条向量行经该外键回连其 chunk，从而继承 chunk 的 `tenant_id` 与 `chunk_acl`（与 §16.3「`embedding` 属 chunk 元数据」一致）。`embeddings` MUST NOT 重复物化 `tenant_id` 物理列，以免与 chunk 维形成双源；§11.9 六维过滤（首维 `tenant_id`）MUST 经「embedding → chunk → 六维过滤」链路施加，向量行不得脱离其 chunk 的租户/ACL 维被独立召回。

#### Scenario: chunk 继承来源文档 ACL
- **WHEN** 一份仅对特定角色可见的文档被解析为 chunk
- **THEN** 每个 chunk 的 `chunk_acl` 与 `tenant_id` 默认与来源文档一致，不向未授权范围放开

#### Scenario: 来源文档权限收紧后 chunk 同步约束
- **WHEN** 来源文档的访问权限被收紧
- **THEN** 重新解析或更新后写入的 chunk `chunk_acl` 反映收紧后的范围，检索不会暴露超出来源文档权限的内容

#### Scenario: chunk_acl 列可独立物化且可严于文档级
- **WHEN** c06 对某文档中部分 chunk 写入比来源文档级 ACL 更严格的 `chunk_acl`（chunk 更严）
- **THEN** `document_chunks` 的 `chunk_acl` 列与来源文档级 ACL 分别保留、可按 `chunk_acl` 列单独查询过滤；用户在文档级可见但对该更严 chunk 无权时，检索按 `chunk_acl` 维过滤掉该 chunk，供 c04 chunk_acl 维过滤与 c06 写入消费（§11.9 六维）

#### Scenario: 跨租户 embedding 不脱离 chunk 过滤被独立召回
- **WHEN** A 租户用户对向量库发起检索，候选 `embeddings` 行中混入 B 租户 chunk 对应的向量行（embedding 自身未物化 `tenant_id` 列）
- **THEN** 检索 MUST 经「embedding → `chunk_id` 外键回连 `document_chunks` → §11.9 六维（首维 `tenant_id`）过滤」链路施加约束，B 租户 chunk 对应的 embedding 因其 chunk 的 `tenant_id` 不匹配被过滤、不进入 A 租户召回结果，向量行不得脱离其 chunk 的租户/ACL 维被独立召回

### Requirement: 解析作业可审计
解析作业的创建、状态流转、失败原因与重试等作业生命周期关键动作 SHALL 仅写入 `audit_logs`（复用 c01 既有列：操作者、`role`、`tenant_id`、操作类型、对象、时间、`result`、`failure_reason`），记录 `document_id`、版本、作业状态、`failure_reason` 与操作者（系统或管理员）。该审计 MUST 支持对单个文档解析全过程的溯源。作业生命周期事件 MUST NOT 写入 `document_events`：`document_events` 仅承载 PRD §10.6 的 6 类触发源，解析作业的 create/parsing/failed/retry 不属于该 6 类枚举。本能力对 `document_events` 的关系是**纯消费方**：c03 在 `document_events` 上**不产生任何 `event_type`**，是 §10.6 全部 6 类触发源（`upload_success`、`save_new_version`、`ai_writeback`、`translation_done`、`template_created`、`manual_reindex`）的唯一重解析/索引消费方——逐类产生方为 `upload_success`=c01、`save_new_version`/`ai_writeback`=c02、`translation_done`=c07、`template_created`=c08、`manual_reindex`=c06，c03 消费其中任一类即为对应 `document_id`/`version_id` 异步创建解析/重解析作业，绝不向 `document_events` 回写作业生命周期事件。

#### Scenario: 解析全过程可溯源
- **WHEN** 审计人员查询某文档的解析记录
- **THEN** 系统返回该文档解析作业的创建、状态流转、失败原因与重试的时间序列审计记录（全部落 `audit_logs`），可完整溯源

#### Scenario: 解析失败原因落审计
- **WHEN** 某文档解析失败
- **THEN** 系统仅在 `audit_logs` 记录一条 `result=失败` 的失败作业记录及 `failure_reason`，供后续排查；不向 `document_events` 写入任何作业生命周期事件

### Requirement: 消费 document_events 全部 6 类触发源发起重新解析与索引
c03 在 `document_events` 上是**纯消费方、不产生任何 `event_type`**。c03 SHALL 消费 PRD §10.6 规定的全部 6 类触发源——`upload_success`（c01 产生）、`save_new_version`（c02 产生）、`ai_writeback`（c02 产生）、`translation_done`（c07 产生）、`template_created`（c08 产生）、`manual_reindex`（c06 产生）——作为重新解析与索引的触发输入，逐类为对应 `document_id`/`version_id` 异步创建解析/重解析作业。`manual_reindex` 由 c06（管理员重建索引）产生、c03 消费触发重解析；c03 MUST NOT 自行产生 `manual_reindex` 或任何其它 `document_events`。重解析生成新作业、旧 chunk 标记 superseded，作业生命周期审计仅落 `audit_logs`（见「解析作业可审计」Requirement），不向 `document_events` 回写。

#### Scenario: 消费 upload_success / save_new_version / ai_writeback 发起解析
- **WHEN** `document_events` 中出现 `event_type ∈ {upload_success, save_new_version, ai_writeback}` 的事件
- **THEN** c03 作为消费侧据此为对应 `document_id`/`version_id` 异步创建解析作业，且不向 `document_events` 回写任何作业生命周期事件

#### Scenario: 消费 translation_done / template_created 发起解析
- **WHEN** `document_events` 中出现 `event_type ∈ {translation_done, template_created}` 的事件（分别由 c07 翻译完成、c08 模板复制创建产生）
- **THEN** c03 作为消费侧为对应译文新版本 / 模板首版本 `document_id`/`version_id` 异步创建解析作业，使其内容可被切分入库并经「索引就绪」事件进入检索

#### Scenario: 消费 c06 产生的 manual_reindex 发起重解析
- **WHEN** `document_events` 中出现 `event_type=manual_reindex` 的事件（由 c06 管理员重建索引产生，携带 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)` 稳定字段）
- **THEN** c03 作为消费侧为对应 `document_id`/`version_id` 创建重解析作业（重解析生成新作业、旧 chunk 标记 superseded），完成后发出「索引就绪」事件供 c04 构建检索索引；c03 不产生该事件、也不向 `document_events` 回写作业生命周期事件
