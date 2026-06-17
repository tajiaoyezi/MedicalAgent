## 1. 数据模型与迁移（c06 新建 §18 命名的 knowledge_bases/kb_documents 基表）

- [x] 1.1 **新建 `knowledge_bases` 基表**（PRD §18 命名，c01/c02/c03 均未建，c06 为唯一建表 owner）：字段含 `kb_id`（主键）、`tenant_id`、`name`、`description`、`created_by`、`is_seed`、`is_pinned`、`manual_weight`（可空）、`data_source`、`member_count`、`document_count`、`created_at`、`updated_at`，编写正向/可回滚迁移，验证：迁移可正向执行、knowledge_bases 表创建成功且 13 字段齐备、不与 c01/c03/c04 所建表重名冲突（对应 §18、D8、§24.3「13 个知识库卡片」）。
- [x] 1.2 **新建 `kb_documents` 基表**（PRD §18 命名，c06 为唯一建表 owner）：含 §11.5.1 导入 10 必录字段（来源 URL/文件来源、来源类型、导入人、导入时间、版权·授权状态、版本、解析状态、索引状态、`whitelist_rule_id`、`authorized_by`），并加 `tenant_id`/`kb_id` 外键与 `document_id`（关联 c01 documents），验证：建表后 10 必录字段齐备可写入、tenant_id/kb_id 外键约束生效（对应 §18、D8、「入库资料必录元数据字段」Requirement）。
- [x] 1.3 新增最小「来源白名单」配置结构（域名/来源标识 → `whitelist_rule_id`、是否允许、授权说明、作用域），并被 `kb_documents.whitelist_rule_id` 引用，验证：可配置一条白名单规则并被导入记录引用（对应 D4、「受控公网导入来源」）。
- [x] 1.4 消费/写入由其它 owner 所建表：`document_chunks`/`embeddings`（c03 所建表，c06 仅向 c03 所建的 `chunk_acl` 列写入 chunk 级 ACL 覆盖值与 §16.3 元数据列、不改结构）、`citations`（c04 所建表，c06 写入/读取引用，不建表）、`conversations`/`messages`（c04 所建表，c06 通过 c04 的 module/source 维写入知识库问答会话、`module=kb_qa`/`source=「医疗知识库问答」`、不建表）、`document_permissions`/`audit_logs`/`recent_tasks`（c01 所建表，c06 仅写入符合 owner 契约的记录、不建表）、`privacy_detection_rules`/`privacy_redaction_events`（c09 所建表，c06 仅消费其脱敏判定接缝），仅校验本 phase 写入所需字段存在，验证：`chunk_acl` 列名与 c03/c04 一致（不出现「单一 acl」与「chunk_acl」并存指同一列）、契约字段对齐 §16.3、不重复建他人 owner 的表（对应 D8、Decision A、Decision B、Risks「依赖 c03/c04 接口」）。

## 2. 预置 13 个医疗知识库（种子）

- [x] 2.1 编写版本化 seed 配置文件，写入 §11.2 的 13 个 `is_seed=true` 知识库（含名称、用途简介、`data_source` 标签），不在代码中硬编码清单字符串，验证：13 库名称与简介逐项对照 §11.2 表格一致（对应 §24.3「13 个默认知识库清单符合 PRD」、D1）。
- [x] 2.2 执行 seed 后校验首页恰好展示 13 个预置库卡片，验证：首页返回库数 = 13 且名称依次匹配 §11.2（对应 §24.3「展示 13 个知识库卡片」、「默认展示 13 个知识库卡片」Scenario）。
- [ ] 2.3 为每个预置库通过第 4 组导入管线导入至少 1 份授权/开放来源的演示文档（演示资料库），空库保留导入管线就绪。**c06 为这批知识库演示文档的唯一资产装载 owner**：每库 ≥1 份演示文档经本 phase D3/D4 导入管线装载并标 `authorized`，c09（§20.4 内置 Demo 数据集/验收测试集）仅引用本批已装载文档纳入验收清单、不重复装载（与 200 模板由 c08 装载、c09 引用同款消歧）。验证：13 库每库 ≥1 份可被检索问答的演示文档且演示资料经授权状态机标 `authorized`、c09 不另行重复导入（对应「每个默认库含演示资料」Scenario、Risks「演示库资料版权」、F16）。
- [ ] 2.4 验证预置空库仍提供上传/导入/检索/问答/溯源/权限过滤入口，验证：打开无正式资料的预置库时上述能力不被禁用（对应「预置空库仍具备完整能力」Scenario）。

## 3. 知识库首页卡片与排序

- [x] 3.1 实现卡片 9 字段查询输出（名称/ID/创建人/简介/成员人数/文档数量/更新时间/数据源/置顶状态），验证：单库卡片同时返回全部 9 字段（对应「卡片展示全部规定字段」Scenario）。
- [x] 3.2 实现 `document_count`（按 `index_status=indexed` 且当前 `tenant_id` 可见范围计数）与 `member_count`（数据源 = 该知识库 ACL/`document_permissions` 授权用户去重计数，不依赖独立 `kb_members` 表）物化计数，`document_count` 的刷新以 c06 消费 c03「索引就绪」事件（见 5.4a，置 `index_status=indexed`）为唯一触发源、随该事件与成员（知识库授权）变更事务内增量更新，验证：收到「索引就绪」事件后卡片文档数量自增、`member_count` 等于该库授权用户去重数、不实时全表聚合（对应「文档数量按可见范围计数」「成员人数取知识库授权用户去重计数」「入库或更新后刷新更新时间」Scenario、D2、Decision E）。
- [x] 3.3 实现卡片更新时间在新增/更新/删除文档完成后同步刷新，验证：完成一次入库后该库 `updated_at` 与文档数量同步刷新（对应「入库或更新后刷新更新时间」Scenario）。
- [x] 3.4 在 SQL 层实现确定性多级排序 `is_pinned DESC, manual_weight DESC NULLS LAST, updated_at DESC, created_at DESC`，验证：置顶库最前、非置顶按权重降序、同权重按更新时间倒序、无配置回退创建时间倒序四级规则与分页稳定（对应 §24.3「配置排序权重和置顶」、§11.3 四个排序 Scenario、D2）。
- [x] 3.5 实现管理员配置「置顶/手动权重」入口（仅平台管理员或对应库管理员），验证：变更置顶或权重后列表顺序即时重算（对应 §24.3「管理员可配置排序权重和置顶」、「置顶库排在最前」Scenario、§11.5）。
- [x] 3.6 实现「创建知识库」入口（§11.5/§17.3，RBAC 管控，普通用户隐藏不可调用）：授权判定 MUST 以「持有 c01 `permissions` 表登记的租户级 `kb:create` 权限点」为唯一授权谓词（`kb:create` 由 c01 auth-rbac 唯一定义、默认授予 `admin`，c06 仅引用不自造），MUST NOT 以待创建（尚不存在）库对象上的 per-kb 管理级 ACL 表达创建授权；创建后在 `knowledge_bases` 落一条带 `tenant_id`/`name`/`created_by` 的空库并即时入列表，验证：持 `kb:create` 用户（默认 `admin`）创建空库成功且即时可见、不持 `kb:create` 的普通用户无创建入口且接口调用被拒、创建授权按租户级 `kb:create` 而非 per-kb ACL 判定（对应「持有 kb:create 权限点的用户创建空知识库」「创建授权按租户级 kb:create 权限点而非 per-kb ACL 判定」「无 kb:create 权限点的用户无创建知识库入口」Scenario、§11.5/§17.3、F14、Decision E）。

## 4. 受控导入管线（四段式 + 授权状态机）

- [x] 4.1 实现统一入库流水线骨架「来源适配器 → 暂存预览(staging) → 授权闸门 → 入库」，并使 staging 与正式库物理隔离（临时预览区不进正式检索索引），验证：staging 资料不出现在正式库检索结果（对应「临时预览资料不可被问答检索到」Scenario、D3）。
- [x] 4.2 实现上传适配器与上传入口权限分级（平台管理员→任意库、库管理员→自管库、普通用户→个人区/会话上下文/私有库），验证：库管理员上传非自管库被拒、普通用户上传不进任何公共库（对应 §24.3「管理员可上传资料」「普通用户不能直接写入公共知识库」、「上传入口与权限分级」全部 Scenario）。
- [ ] 4.3 实现批量上传逐项落库（每份文档独立入库记录、解析状态、索引状态），验证：一次上传 N 份生成 N 条入库记录（对应「批量上传逐项落库」Scenario）。
- [x] 4.4 实现 URL/白名单官网来源适配器与白名单匹配：命中白名单 → `authorized` 且写 `whitelist_rule_id`；未命中合法 URL → 要求管理员显式授权并记 `authorized_by`；授权不明 → `preview_only` 仅临时预览，验证：三条分支分别落对应授权状态（对应 §24.3「URL/白名单来源导入」「未授权 URL 必须被阻止或仅进临时预览」、「受控公网导入来源」「授权状态不明确仅临时预览」Requirement、D4）。
- [x] 4.5 实现未授权商业数据库红线阻断：默认抓取/批量导入、镜像站、下载链接来源 → `rejected`，不抓取不写库，验证：对未授权商业库/镜像站/下载链接发起导入被阻断并提示来源被红线禁止（对应 §24.3「未授权 URL 必须被阻止」、「未授权商业数据库导入禁止（产品红线）」全部 Scenario）。
- [ ] 4.6 实现 PubMed/PMC 来源适配器，复用 c04 PubMed 接入取结构化文献并记来源类型与 `pubmed_id`/`doi` —— 连通性路径：公网可用时真实拉取 PubMed/PMC 文献入预览；URL/PMC/PubMed 取数路径进入授权闸门时，其初始授权标记 MUST 取自 c04 pubmed-data-service 返回的 `RetrievedSource` 的 `authorized`/`preview_only`/`rejected` 作为闸门输入，c06 仅在其上叠加白名单/管理员授权裁决与 staging 隔离、MUST NOT 从零重建该三态（最终落库裁决以 c06 kb-import 契约为唯一真值），验证：导入记录含来源类型 PubMed/PMC 与 pubmed_id/DOI、且授权三态初值来自 c04 标记不与 c04 漂移（对应 §24.3「PubMed/PMC 导入」、「PubMed / PMC 导入」「受控公网导入来源消费 c04 授权标记」Scenario、D4、Decision E）。
- [ ] 4.7 PubMed/PMC 离线降级路径：公网不可用时切换 c03/c04「离线 PubMed 缓存」作为适配器后端从离线缓存取文献入库，验证：断网环境下仍可从离线缓存完成一篇文献导入演示（对应 D7、rules「外部接入须覆盖离线/私有化降级」）。
- [ ] 4.8 URL/白名单导入离线降级路径：公网不可用时该来源置不可用并引导改用「批量上传已下载授权文件」完成入库（同样过预览+授权闸门），验证：无公网时通过上传授权文件完成等效入库、闭环不中断（对应 D7、Risks「离线环境 URL/白名单导入不可用」）。
- [x] 4.9 实现入库前预览确认（人工确认链路）：展示来源/解析结果概览/授权·版权状态，确认前不进正式索引；确认入库才写正式库，取消则丢弃不落库，验证：未确认资料不入索引、取消后无残留（对应 §24.3「未授权 URL 仅进临时预览」配套、「入库前预览确认」全部 Scenario、§11.5.1）。

## 5. 导入脱敏门禁与必录元数据

- [x] 5.1 在向量化/检索前消费 c09 redaction-gateway 的 PHI/PII 识别与脱敏判定接缝（redaction-gateway 唯一 owner=c09，c01 不实现、c03 仅留接缝），确认无残留敏感信息后才允许调用公网模型（公网解析/向量化/抓取）；本期默认公网关闭、redaction-gateway 未接入前仅走私有化/离线，验证：含 PHI 资料先脱敏再调用公网模型、未接入门禁时不启用公网（对应「公网导入前 PHI/PII 识别与脱敏门禁」「调用公网模型前完成脱敏」Scenario、Decision B、红线）。
- [x] 5.2 实现脱敏门禁阻断/降级（默认拒绝公网的保守降级）：c09 识别判定失败、置信度不足或 redaction-gateway 不可用时禁止调用公网模型，按平台策略阻断入正式公共库或降级私有化解析/模型，并写由 c09 所建的 `privacy_redaction_events`，验证：识别不可用时公网模型不被调用且事件留痕（对应「识别失败禁止调用公网模型」Scenario、Decision B、D7、红线）。
- [ ] 5.2a 知识库本地/批量上传入口接入 c09 上传闸（与出网/向量化前门禁为两个独立执行点）：上传/批量上传内容在持久化入 `kb_documents`/向量化前先经 c09 redaction-gateway 上传闸做 PHI/PII 识别（识别范围含姓名/身份证号/手机号/住院号·门诊号/医保号/地址/检查号·影像号及可配置敏感词），按「识别并提示/脱敏后送模型/阻止上传」策略处理；策略=阻止上传且命中时拒绝入库并写 `result=失败`、`failure_reason` 非空的 `audit_logs`，脱敏命中由 c09 写 `privacy_redaction_events`；redaction-gateway owner 归 c09、c06 仅前置消费（以 c09「上传时 PHI/PII 识别与『阻止上传』策略执行」契约的上传入口枚举——文档中心/AIMed/医学翻译/知识库四类——为唯一真值，c06 知识库本地/批量上传入口以 c09 owner 枚举为准纳入该契约），验证：含 PHI 的上传在「阻止上传」策略下被拒入库且留痕、与公网导入前门禁为两个独立执行点（对应「本地/批量上传持久化入库前消费 c09 上传闸（含「阻止上传」策略）」全部 Scenario、Decision B、§19.4、红线）。
- [x] 5.3 实现入库 10 必录字段强制校验：缺来源类型/版权·授权状态等必录字段则阻断正式入库并提示缺失，验证：缺关键元数据的资料无法完成入库（对应「入库资料必录元数据字段」「缺失关键元数据阻断入库」Scenario）。
- [x] 5.4a 接入 c03「索引就绪」事件作为 `index_status=indexed` 与 `document_count` 刷新的唯一触发源：c06 作为该事件的知识库侧消费方（与 c04 共同消费），文档完成解析/分块/向量化、c03 发出「索引就绪」事件后，把对应 `kb_documents.index_status` 置 `indexed` 并在同一事务内增量刷新该库 `document_count`（仅计 `index_status=indexed`）与 `updated_at`；事件到达前 MUST NOT 自行置 `indexed`；重建索引（manual_reindex→c03 重解析→再发索引就绪）收尾走同一消费路径，验证：收到「索引就绪」事件后该文档 index_status=indexed、document_count 与 updated_at 同步刷新、且无该事件时不置 indexed（对应「消费 c03 索引就绪事件置 indexed 并刷新计数」「重建索引收尾走同一索引就绪事件消费路径」Scenario、§17.3、§11.2、D2）。
- [x] 5.4 实现解析状态/索引状态可追踪（待解析/解析中/解析完成、待索引/索引中/索引完成/失败），失败时管理员可触发重建索引并校正物化计数；管理员触发重建索引时向 c01 所建 `document_events` 产生 `event_type=manual_reindex`（携带 document_id/version_id/tenant_id/occurred_at/payload 等 c01 §10.6 契约字段、由 c03 消费触发重解析），解析作业生命周期与重建动作审计写 `audit_logs` 不写 `document_events`，验证：状态机推进可见、重建索引后计数被校正、且产生一条 `event_type=manual_reindex` 的 `document_events`（对应 §17.3「解析状态/索引状态/重建索引」、「解析与索引状态可追踪」「管理员触发重建索引产生 manual_reindex 事件」Scenario、§10.6、§11.5、Decision A、Risks「物化计数不一致」）。

## 6. 检索问答接入 c04（知识库作为数据源）

- [x] 6.1 把知识库注册为 c04 §16.2 检索流程的数据源类别，前置「数据源选择 = 选定的一个或多个 kb_id」，验证：可指定单/多 kb_id 进入 c04 检索流程（对应 §24.3「支持跨库检索」、D5）。
- [x] 6.2 实现全局搜索三模式（关键词=BM25、语义=向量、混合=合并去重+rerank）复用 c04 内核，验证：三模式分别返回 BM25 命中/语义相关/合并去重结果（对应「全局搜索」前三 Scenario）。
- [x] 6.3 实现多维筛选（按知识库/文档类型/更新时间/来源/权限）作为检索前置条件，其中「文档类型」维取自 `kb_documents.document_id` 关联 c01 `documents` 的文件类型（pdf/ofd/doc/docx/xlsx/xls/ppt/pptx/png/jpg 等，§8.6.4/§8.6.5）、「来源」维取自 `source_type`（来源类型），两维用不同承载字段区分、MUST NOT 用同一字段兼表，验证：设置筛选后仅返回同时满足全部条件的结果，且按「文档类型=docx」与按「来源=PubMed」分别命中文件类型字段与来源类型字段、两维不混同（对应 §11.6、「多维筛选生效」「按文档类型筛选命中文件类型字段」「按来源筛选命中来源类型字段」Scenario、F11）。
- [x] 6.4 实现多库选择知识库问答流程（检索 chunk → rerank → 生成答案 → 标注引用 → 展示来源文档）复用 c04，验证：勾选多库问答返回带引用与来源文档的答案（对应 §24.3「支持知识库问答」、「跨多库问答」Scenario、§11.7）。
- [x] 6.5 实现答案医疗免责声明与「草稿/辅助建议」默认标记，及无召回时不臆造（提示未找到可溯源依据），验证：答案带免责声明并标草稿、无相关 chunk 时不编造无引用建议（对应「答案展示免责声明并标注草稿」「无依据时不臆造答案」Scenario、红线）。
- [ ] 6.6 知识库问答模型 fallback 降级路径：复用 c04/c03 模型 Provider（公网→私有化）fallback，验证：无公网时由私有化模型生成带引用答案（对应 D7、rules「外部接入须覆盖离线/私有化降级」）。
- [ ] 6.7 问答生成调用公网模型前消费 c09 redaction-gateway 脱敏门禁（与 kb-import 导入侧门禁为两个独立执行点）：用户问题/检索注入上下文经 c09 识别脱敏后才调公网模型；识别失败/置信度不足/服务不可用/门禁未接入时禁止调用公网模型并切 c03 私有化模型，调用留痕写 `audit_logs`、脱敏事件由 c09 在 c03 公网出口写 `privacy_redaction_events`。本期默认公网关闭、真实判定接入与公网放开随 c09 落地验收，验证：含 PHI 问题先脱敏再调公网模型、识别不可用时不调用公网且切私有化（对应「知识库问答生成调用公网模型前 PHI/PII 脱敏门禁」全部 Scenario、Decision D、§19.4、红线）。
- [x] 6.8 知识库问答（`module=kb_qa`）答案下发前接入 c05 ai-writeback-confirmation 的 message 级高风险确认链路（前置消费、不自建判定）：答案下发前交由 c05 服务端 `risk_type` 分类器判定，命中高风险（诊疗/用药/医嘱/临床文书/患者个体信息）时以 `message_id` 为键落 c05 所建 `writeback_confirmations`，按 `confirmed_role∈{doctor,reviewer}` 裁决，普通用户只能生成草稿/提交审核、MUST NOT 完成最终确认与下发；`risk_type` 判定与确认记录 owner 归 c05，c06 仅触发链路并记录问答行为到 `audit_logs`；该链路的 message 级生产方枚举（AIMed 答案 / 知识库问答 kb_qa / 医学翻译文书三类）以 c05/c09 owner 枚举为唯一真值，c06（kb_qa）仅作 message 级生产方挂载、以 owner 枚举为准，验证：高风险 kb_qa 答案普通用户不能直接下发、医生/审核角色确认后方可下发、确认记录由 c05 以 message_id 落 `writeback_confirmations`（对应「知识库问答高风险答案下发前的人工确认」全部 Scenario、Decision B、§19.2）。

## 7. 溯源与引用定位

- [ ] 7.1 导入入库时把 §16.3 chunk 元数据（source_type/source_title/source_url/pubmed_id/doi/journal/year/section/page/paragraph_index/chunk_text）正确写入 chunk，验证：演示库 chunk 元数据齐备可支撑段落级溯源（对应 D5、§16.3）。
- [x] 7.2 实现答案引用溯源到 知识库/源文档/章节/页码/段落/chunk/原文片段（复用 c04 `citations`），验证：查看任一引用可定位到上述七级（对应 §24.3「答案可溯源到段落」、「溯源到段落级」Scenario、§11.8）。
- [ ] 7.3 实现引用可点击跳转到来源文档位置，验证：点击引用跳转到对应来源位置且引用可点击率 ≥95%（对应「引用可点击跳转」Scenario、§20.3 指标）。
- [ ] 7.4 对知识库问答测试集运行引用定位验收，验证：引用源定位成功率 ≥90% 且页码误差 ≤1 页（对应「引用定位准确率达标」Scenario）。

## 8. 终端用户功能隔离与权限过滤

- [ ] 8.1 实现终端用户默认隐藏管理类入口（导入知识库/新建知识库/公开知识/我管理的/我加入的/历史会话侧边栏），验证：普通用户首页不展示上述入口（对应「终端用户功能隔离」「普通用户隐藏管理类入口」Scenario、§11.4）。
- [x] 8.2 实现普通用户可见 kb 集合 = 预设公共库 ∪ 被授权私有库，不返回其它租户或未授权私有库，验证：普通用户列表仅含预设库与授权私有库（对应「普通用户仅见预设库与授权私有库」Scenario）。
- [x] 8.3 实现知识库级 ACL（读取/问答/上传导入/管理四类权限）与绕过 UI 的接口级鉴权，「读取/问答/上传导入/管理」四类为 PRD §19.1 per-kb 资源级 ACL 能力（落 `document_permissions` / 知识库 ACL 授予记录），MUST 锚定 c01 auth-rbac 角色/权限点唯一真值、MUST NOT 在 c01 `permissions` 表自造 `kb:*` 平台级权限点：「库管理员身份」= 在某具体知识库持有管理级 per-kb ACL 授予，普通 c01 角色（如 `user`）不自动等同库管理员、「平台管理员」=c01 `admin` 角色，验证：普通用户直接调用无权管理接口被拒并写 `audit_logs`、库管理员越界管理被拒、仅持普通 c01 角色而无该库管理级 ACL 授予者不被判为库管理员（对应「知识库级权限」「库管理员身份取自 per-kb 管理级 ACL 授予而非新全局角色」「普通用户绕过 UI 直接调用被拒绝」「知识库管理员仅能管理自己的库」Scenario、F10 角色锚定）。
- [x] 8.4 导入侧物化两维 ACL（document_acl 与 chunk_acl 分别落点，不混为单一 acl）：写入时把 `tenant_id`/`kb_id` 落 `kb_documents`、`document_acl` 落 `document_permissions`、`chunk_acl` 覆盖值写入 c03 所建 `document_chunks.chunk_acl` 列（c06 仅写值不建列；以 c03 新增的 `chunk_acl` 列为前置依赖），验证：入库后 document 级与 chunk 级两维过滤条件均已分别物化、`chunk_acl` 列名与 c03/c04 一致（对应 §11.9、Decision A、D6 写入侧）。
- [x] 8.5 读取侧把 `user_id`/`role`/可见 `kb_id` 集合以及基于 `document_permissions` 预计算的可见 `document_id` 集合下推到 c04 召回前权限过滤步骤（非应用层后置裁剪），由 c04 rag-retrieval 执行 document_acl 与 chunk_acl 两维过滤。验收前置：依赖 c04 rag-retrieval 已把单一 acl 扩展为可区分 document_acl/chunk_acl 两维过滤（接受 c06 注入的可见 document 集合），否则 8.6 的 document 级越权用例不可验。验证：六维（tenant_id/kb_id/user_id/role/document_acl/chunk_acl）任一不满足的内容不进候选/不参与 rerank/不进引用与来源列表（对应「RAG 权限过滤维度」全部 Scenario、§11.9、D6 读取侧、Decision D）。
- [x] 8.6 验证 document 级与 chunk 级两维 ACL 与跨租户隔离：用户无 document_acl 的整篇文档不进候选、更严格 chunk_acl 的 chunk 不注入答案上下文、跨租户检索不返回他租内容，验证：越权 document 与越权 chunk 均不出现在答案/引用/来源（对应「document/chunk 级 ACL 隔离」「跨租户检索被隔离」「跨租户访问被隔离」Scenario、Decision D）。

## 9. 审计、问答日志与最近任务

- [ ] 9.1 将上传/公网导入/授权确认/预览入库/拒绝·阻断行为写入 `audit_logs`（操作人、tenant_id、kb_id、来源、授权确认人、白名单规则 ID），含被红线阻断事件，验证：一次导入与一次被阻断导入均留痕（对应「导入与授权行为审计留痕」全部 Scenario）。
- [ ] 9.2 将检索与问答行为写入 `audit_logs` 并生成问答日志（用户、tenant_id、所选 kb_id、查询、返回引用、时间），验证：完成问答后审计与问答日志均生成（对应「检索与问答行为审计」「问答写入审计与问答日志」Scenario）。
- [ ] 9.3 实现管理员在权限范围内查看问答日志，验证：管理员后台展示其权限内问答记录与对应引用来源（对应 §11.5「查看问答日志」、「管理员查看问答日志」Scenario）。
- [x] 9.4 知识库问答会话持久化与最近任务写入（c06 为知识库问答会话唯一写入方）：(a) 把会话/消息写入 c04 所建 `conversations`/`messages`，并通过 c04 的 `module`/`source` 两个独立维标记：`module=kb_qa`（机器枚举值，c04 owner 定义、取值域 {aimed, kb_qa}）、`source=「医疗知识库问答」`（§6.4 中文规范值），以区分 AIMed、按 tenant_id/user_id 隔离；(b) 向 c01 所建 `recent_tasks` 写一条 `source=医疗知识库问答`、`ref_type=conversation`、`ref_id=conversation_id` 的记录，以 `(ref_type,ref_id)` 为幂等键 upsert，验证：完成一次问答后会话落 conversations/messages 且 `module=kb_qa`、非 AIMed 模式（`module≠aimed`）、c05 可按 `module=kb_qa` 识别恢复、recent_tasks 出现该会话条目（source/ref 字段可观测）且可由 c05 按 ref_id 恢复（对应 §24.3「历史进入最近任务」、「问答历史进入最近任务」「知识库问答会话持久化到 conversations/messages」「知识库问答写入最近任务」Scenario、Decision B）。

## 10. 主验收闭环（§24.3 端到端）

- [ ] 10.1 端到端连通性演示（公网可用）：登录 → 知识库首页展示 13 卡片 → 管理员配置排序权重/置顶 → 管理员上传资料并经 URL/PubMed/PMC/白名单导入 → 预览确认入库 → 跨库检索 → 多库问答 → 答案溯源到段落 → 问答历史进入最近任务，验证：§24.3 全部正向验收点逐条通过。
- [ ] 10.2 端到端红线负向演示：未授权 URL/商业库导入被阻止或仅进临时预览、普通用户上传不能写入公共库、临时预览资料检索不可见，验证：§24.3 全部负向验收点逐条通过（对应「未授权 URL 必须被阻止或仅进临时预览」「普通用户不能直接写入公共知识库」）。
- [ ] 10.3 离线/私有化降级闭环演示（无公网）：以离线 PubMed 缓存导入 + 上传授权文件替代 URL 抓取 + 私有化解析/向量/模型完成 13 库检索问答与溯源，验证：内网无公网环境下主验收闭环（检索/问答/溯源）仍可完整演示（对应 D7、rules「外部接入须覆盖离线/私有化降级」）。
