## ADDED Requirements

### Requirement: 上传入口与权限分级
系统 SHALL 按角色分级提供知识库上传入口：平台管理员 MUST 可上传到任意知识库；知识库管理员 MUST 仅可上传到自己管理的知识库；普通用户上传 MUST 默认进入个人资料区、会话上下文或私有知识库，不得直接写入公共知识库。所有知识库（含 13 个预置库）SHALL 支持管理员或知识库管理员上传资料。

本能力的角色与上传权限判定 MUST 锚定 c01-foundation auth-rbac 的唯一真值，MUST NOT 平行重定义角色判定或在 c01 `permissions` 表自造平台级权限点：「平台管理员」对应 c01 `roles` 的 `admin` 角色；「知识库管理员」MUST NOT 被理解为 c01 `roles` 表的新全局角色（c01 `roles` 唯一真值仅 `admin`/`user`/`dept`/`doctor`/`reviewer` 五类），SHALL 定义为「在某具体知识库上持有上传/导入级及以上知识库 ACL 授予记录的用户（per-kb scoped grant）」，普通 c01 角色 MUST NOT 自动等同于库管理员；「上传/导入」上传权限属 PRD §19.1「知识库级 ACL」下的 per-kb 资源级 ACL 能力（落 `document_permissions` / 知识库 ACL 授予记录），区别于 c01 `permissions` 表的平台 RBAC 权限点，本能力 MUST NOT 在 c01 `permissions` 表自造 `kb:import`/`kb:upload` 等平台级权限点名。

#### Scenario: 平台管理员上传到任意库
- **WHEN** 平台管理员选择任意知识库并批量上传文档
- **THEN** 系统 SHALL 接受上传并进入入库前预览流程
- **AND** 记录导入人与导入时间

#### Scenario: 知识库管理员仅能上传到自管库
- **WHEN** 知识库管理员尝试上传到非自己管理的知识库
- **THEN** 系统 SHALL 拒绝上传并提示无权限

#### Scenario: 普通用户上传不入公共库
- **WHEN** 普通用户上传资料
- **THEN** 系统 SHALL 仅写入其个人资料区、会话上下文或私有知识库
- **AND** MUST NOT 进入任何公共知识库

#### Scenario: 批量上传逐项落库
- **WHEN** 管理员一次性上传多份文档
- **THEN** 系统 SHALL 为每份文档分别生成入库记录、解析状态与索引状态

### Requirement: 受控公网导入来源
系统 SHALL 支持受控公网导入：URL 导入、PubMed / PMC 导入、白名单官网来源导入。每条公网导入 MUST 通过来源白名单或管理员授权确认；命中白名单规则的导入 SHALL 记录其白名单规则 ID。URL/PMC/PubMed 取数路径进入授权闸门时，其初始授权三态标记（`authorized`/`preview_only`/`rejected`）MUST 取自 c04 pubmed-data-service 对每个 `RetrievedSource` 已返回的标记作为闸门输入，本能力 MUST NOT 从零独立重建该三态；本能力仅在 c04 标记之上叠加白名单匹配、管理员授权裁决与 staging 隔离，最终落库裁决以本 kb-import 契约为唯一真值。

#### Scenario: 白名单来源 URL 导入成功
- **WHEN** 管理员从命中来源白名单的官网 URL 发起导入
- **THEN** 系统 SHALL 允许抓取并进入入库前预览
- **AND** 在入库记录中写入命中的白名单规则 ID

#### Scenario: PubMed / PMC 导入
- **WHEN** 管理员通过 PubMed 或 PMC 检索结果发起导入
- **THEN** 系统 SHALL 拉取对应文献并记录来源类型为 PubMed/PMC、pubmed_id 或 DOI 等来源标识
- **AND** 进入入库前预览流程

#### Scenario: 受控公网导入消费 c04 授权标记作为闸门输入
- **WHEN** URL/PMC/PubMed 取数路径进入授权闸门
- **THEN** 系统 SHALL 以 c04 pubmed-data-service 返回的 `RetrievedSource` 的 `authorized`/`preview_only`/`rejected` 标记作为闸门初始输入，MUST NOT 从零重新判定该三态
- **AND** 仅在该标记之上叠加白名单匹配/管理员授权裁决与 staging 隔离，最终落库裁决以本 kb-import 契约为唯一真值

#### Scenario: 非白名单 URL 需管理员授权
- **WHEN** 管理员从未命中白名单的合法 URL 发起导入
- **THEN** 系统 SHALL 要求管理员显式授权确认后方可继续
- **AND** 记录授权确认人

### Requirement: 未授权商业数据库导入禁止（产品红线）
系统 MUST NOT 默认抓取或批量导入未授权商业数据库；MUST NOT 导入未授权商业数据库的页面、下载链接或镜像站内容。任何 URL 导入 MUST 先经来源白名单或管理员授权确认，否则禁止写入正式公共知识库。

#### Scenario: 拒绝未授权商业库批量抓取
- **WHEN** 用户尝试对未授权商业数据库发起默认抓取或批量导入
- **THEN** 系统 SHALL 阻止该导入并提示来源未授权
- **AND** 不抓取、不写入任何知识库，并记录到 `audit_logs`

#### Scenario: 拒绝镜像站与下载链接导入
- **WHEN** 导入来源为未授权商业数据库的镜像站或文献下载链接
- **THEN** 系统 SHALL 拒绝导入并提示来源被红线禁止

### Requirement: 授权状态不明确仅临时预览
当导入来源的版权/授权状态不明确时，系统 SHALL 仅允许其进入临时预览区，MUST NOT 写入正式公共知识库。临时预览资料 SHALL 与正式入库资料隔离存储，并 SHALL 不进入公共库的检索与问答索引。

#### Scenario: 授权不明仅进临时预览
- **WHEN** 某 URL 导入既未命中白名单也未获明确授权确认
- **THEN** 系统 SHALL 将抓取内容放入临时预览区，授权/版权状态标记为「不明确」
- **AND** MUST NOT 将其写入正式公共知识库或纳入公共库检索索引

#### Scenario: 临时预览资料不可被问答检索到
- **WHEN** 终端用户在公共知识库发起检索或问答
- **THEN** 系统 SHALL 不返回任何处于临时预览状态的资料

### Requirement: 入库前预览确认（人工确认链路）
所有上传与导入资料 MUST 在写入正式知识库前进入预览确认环节，由具备权限的操作人确认后方可入库。预览 SHALL 展示来源、解析结果概览与授权/版权状态，确认前 MUST NOT 进入正式检索索引。

#### Scenario: 入库前必须预览确认
- **WHEN** 管理员上传或导入资料完成抓取与解析
- **THEN** 系统 SHALL 先展示预览（来源、解析结果、授权状态）
- **AND** 仅在操作人点击确认入库后才写入正式知识库与索引

#### Scenario: 取消预览不落库
- **WHEN** 操作人在预览环节取消入库
- **THEN** 系统 SHALL 丢弃该资料，不写入正式知识库、不建立索引

### Requirement: 公网导入前 PHI/PII 识别与脱敏门禁（消费 c09 redaction-gateway）
PHI/PII 识别与脱敏引擎（redaction-gateway）的唯一 owner 为 c09 security-evidence；c01 不实现该能力、c03 仅在公网出口预留门禁接缝。本能力 MUST NOT 自行实现 PHI/PII 识别脱敏，仅消费 c09 redaction-gateway 的判定接缝。上传与公网导入资料在进入向量化与检索前 MUST 经 c09 redaction-gateway 的 PHI / PII 识别与脱敏判定。本期默认公网关闭、私有化优先：redaction-gateway 未接入前不启用公网，仅私有化/离线路径跑通闭环（§16.4/§24.9）。涉及调用公网模型（含公网解析/向量化/抓取）前 MUST 先完成 PHI/PII 识别与脱敏；当识别失败、脱敏置信度不足或 redaction-gateway 不可用时，系统 MUST NOT 调用公网模型（按「识别服务不可用」处理、默认拒绝公网），并 SHALL 按平台脱敏策略阻断或降级（可切换私有化模型/私有化解析）。

#### Scenario: 调用公网模型前完成脱敏
- **WHEN** 导入资料需经公网模型解析或向量化
- **THEN** 系统 SHALL 先调用 c09 redaction-gateway 执行 PHI/PII 识别与脱敏，确认无残留敏感信息后才调用公网模型

#### Scenario: 门禁不可用默认拒绝公网
- **WHEN** c09 redaction-gateway 识别失败、脱敏置信度不足或 redaction-gateway 尚未接入/不可用
- **THEN** 系统 MUST NOT 调用公网模型
- **AND** SHALL 阻断该导入或切换至私有化模型/私有化解析路径，并记录到 `audit_logs`

### Requirement: 本地/批量上传持久化入库前消费 c09 上传闸（含「阻止上传」策略）
PRD §19.4 规定 PHI/PII 识别发生在「上传时」与「调用模型时」两个时点，二者为独立拦截点。本能力的「公网导入前 PHI/PII 识别与脱敏门禁」仅覆盖出网/向量化时点（调用公网模型时），不覆盖「内容持久化入库」这一上传时点。c09 security-evidence 是上传闸（redaction-gateway 上传入口拦截）的唯一 owner，其「上传时 PHI/PII 识别与『阻止上传』策略执行」契约的上传入口枚举（文档中心 / AIMed / 医学翻译 / 知识库四类）为该枚举的唯一真值，本能力的知识库本地/批量上传入口以 c09 owner 枚举为准纳入该上传闸契约、复用同一契约而不另行重定义入口清单。本能力 MUST NOT 自行实现 PHI/PII 识别脱敏，仅作为上传入口前置消费 c09 上传闸契约。知识库本地上传与批量上传的内容在持久化入 `kb_documents`/向量化前 MUST 先经 c09 redaction-gateway 上传闸做 PHI/PII 识别（识别范围含姓名/身份证号/手机号/住院号·门诊号/医保号/地址/检查号·影像号及可配置敏感词），并按策略处理：默认「识别并提示」/「脱敏后送模型」/「阻止上传」；当策略=阻止上传且命中 PHI/PII 时，系统 MUST 拒绝入库，并写 `result=失败`、`failure_reason` 非空的 `audit_logs`，脱敏命中由 c09 写入 `privacy_redaction_events`。本上传闸与「公网导入前 PHI/PII 识别与脱敏门禁」为两个独立执行点（上传持久化入库时 vs 调用公网模型时），与 c01 文档中心 / c04 AIMed / c07 翻译上传入口口径一致，redaction-gateway owner 仍归 c09、本能力仅前置消费。本能力既有「入库前预览确认」为版权/授权人工确认，不等同于 PHI「阻止上传」策略，二者并存。

#### Scenario: 本地/批量上传持久化前经 c09 上传闸识别
- **WHEN** 管理员或知识库管理员通过本地上传或批量上传向知识库提交资料
- **THEN** 系统 SHALL 在内容持久化入 `kb_documents`/向量化前先经 c09 redaction-gateway 上传闸完成 PHI/PII 识别与策略处理
- **AND** 上传闸与公网导入前门禁为两个独立执行点

#### Scenario: 命中 PHI 且策略为「阻止上传」时拒绝入库并留痕
- **WHEN** 上传/批量上传内容命中 PHI/PII 且当前策略配置为「阻止上传」
- **THEN** 系统 MUST 拒绝该资料入库，写一条 `result=失败`、`failure_reason` 非空的 `audit_logs`
- **AND** 脱敏命中由 c09 redaction-gateway 写入 `privacy_redaction_events`，本能力仅前置消费、MUST NOT 自建该识别脱敏能力

### Requirement: 入库资料必录元数据字段
所有入库资料 MUST 记录 PRD 第 11.5.1 节规定的 10 个字段槽位：来源 URL / 文件来源、来源类型、导入人、导入时间、版权/授权状态、版本、解析状态、索引状态、白名单规则 ID（`whitelist_rule_id`）、授权确认人（`authorized_by`）。本 Requirement 区分「记录槽位 MUST 存在」与「值 MUST 非空」两类语义以消除可验证边界歧义：来源 URL/文件来源、来源类型、导入人、导入时间、版权/授权状态、版本、解析状态、索引状态这 8 个字段为「值 MUST 非空」的硬门禁（缺值 MUST NOT 完成正式入库）；`whitelist_rule_id` 与 `authorized_by` 为「记录槽位 MUST 存在、值按来源路径条件填充」——白名单命中路径填 `whitelist_rule_id`、管理员显式授权路径填 `authorized_by`、PubMed/PMC 与平台管理员直接上传路径二者可为空（或系统值），其为空不构成入库阻断。c06 作为 PRD §10.6 六类重建/索引触发源中 `manual_reindex` 的产生方，管理员触发重建索引时 SHALL 向 c01 所建 `document_events` 产生 `event_type=manual_reindex` 事件（携带 c01 §10.6 规定的稳定契约字段），由 c03 document-parsing 消费触发重解析；解析作业生命周期与重建动作本身的审计写 `audit_logs`，不写 `document_events`。

#### Scenario: 入库记录包含全部必录字段
- **WHEN** 一份资料完成入库
- **THEN** 入库记录 SHALL 同时包含来源 URL/文件来源、来源类型、导入人、导入时间、版权/授权状态、版本、解析状态、索引状态、白名单规则 ID 与授权确认人

#### Scenario: 缺失非空硬门禁字段阻断入库
- **WHEN** 一份待入库资料缺少来源类型、版权/授权状态等「值 MUST 非空」的硬门禁字段
- **THEN** 系统 SHALL 阻止其正式入库并提示缺失字段

#### Scenario: PubMed/上传路径 authorized_by 为空不构成入库阻断
- **WHEN** 经 PubMed/PMC 导入或平台管理员直接上传的资料其 `authorized_by` 为空（该路径无人工授权确认人），但 8 个硬门禁字段均非空
- **THEN** 系统 SHALL 允许其正式入库
- **AND** `authorized_by` 为空不被判为缺失必录字段、不阻断入库

#### Scenario: 解析与索引状态可追踪
- **WHEN** 资料入库后解析或索引仍在进行
- **THEN** 入库记录的解析状态与索引状态 SHALL 反映当前进度（如待解析/解析中/解析完成、待索引/索引中/索引完成/失败）
- **AND** 失败时 SHALL 可由管理员触发重建索引

#### Scenario: 管理员触发重建索引产生 manual_reindex 事件
- **WHEN** 管理员对某资料触发重建索引
- **THEN** 系统 SHALL 向 c01 所建 `document_events` 产生一条 `event_type=manual_reindex` 的事件，携带 `document_id`、`version_id`、`tenant_id`、`occurred_at`、`payload` 等 c01 §10.6 规定的稳定契约字段
- **AND** 该事件由 c03 document-parsing 作为重解析触发输入消费（c06 为 §10.6 `manual_reindex` 的产生方、c03 为消费方），重建动作本身的审计写 `audit_logs`

### Requirement: 消费 c03「索引就绪」事件刷新索引状态与文档计数
c03 document-parsing 是「索引就绪」事件的唯一 owner/产生方，该事件由 c04（检索索引构建）与 c06（知识库入库与重建索引收尾）共同消费。本能力 SHALL 作为该事件的知识库侧消费方：当某文档（含首次入库与 `manual_reindex` 重解析）完成解析/分块/向量化并由 c03 发出「索引就绪」事件后，本能力 MUST 消费该事件，将对应 `kb_documents.index_status` 置为 `indexed`，并在同一事务内增量刷新该知识库的 `document_count`（仅计 `index_status=indexed` 的文档）与 `updated_at`。该「索引就绪」事件为 `index_status→indexed` 状态推进与卡片物化计数刷新的唯一触发源，本能力 MUST NOT 在事件到达前自行将 `index_status` 置为 `indexed`。重建索引（管理员 `manual_reindex` → c03 重解析 → 再发「索引就绪」）的收尾走同一消费路径，重解析完成后据该事件校正计数。

#### Scenario: 消费 c03 索引就绪事件置 indexed 并刷新计数
- **WHEN** 某 `kb_documents` 文档完成解析/分块/向量化、c03 发出该文档的「索引就绪」事件
- **THEN** 本能力 SHALL 消费该事件，将对应 `kb_documents.index_status` 置为 `indexed`
- **AND** 在同一事务内增量刷新该知识库 `document_count`（仅计 `index_status=indexed`）与 `updated_at`，「索引就绪」事件为该刷新的唯一触发源

#### Scenario: 重建索引收尾走同一索引就绪事件消费路径
- **WHEN** 管理员触发 `manual_reindex`、c03 完成重解析后再次发出该文档的「索引就绪」事件
- **THEN** 本能力 SHALL 通过同一消费路径将 `index_status` 重新置为 `indexed` 并校正该库 `document_count` 与 `updated_at`

### Requirement: 导入与授权行为审计留痕
系统 SHALL 将所有上传、公网导入、授权确认、预览入库与拒绝/阻断行为写入 `audit_logs`，记录操作人、tenant_id、kb_id、来源、授权确认人与白名单规则 ID，供管理员事后审计。

#### Scenario: 导入行为写入审计日志
- **WHEN** 管理员完成一次导入或入库确认
- **THEN** 系统 SHALL 写入 `audit_logs`，包含操作人、tenant_id、kb_id、来源、授权确认人与白名单规则 ID（如有）

#### Scenario: 被红线阻断的导入同样留痕
- **WHEN** 一次导入因未授权商业库或脱敏门禁被阻断
- **THEN** 系统 SHALL 将该阻断事件及原因写入 `audit_logs`
