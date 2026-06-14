## 1. 数据模型与迁移（复用 PRD §18 命名）

- [ ] 1.1 新增迁移：创建 `model_providers` 表，落 §16.4 连接级字段（`provider`、`base_url`/内网 `base_url`、`api_key`|`token`、`model`、超时、重试、`network_policy`、`enabled`、`default_priority`、`deployment_kind`=public|private、`tenant_id`），敏感字段加密/掩码存储；验证：迁移可正向/回滚执行且建表字段与 §16.4 公网/私有化两套字段对齐
- [ ] 1.2 新增迁移：创建 `model_routes` 表，记录 (capability, provider_id, priority, enabled, tenant_id) 多对多绑定；validate：同一 capability 可绑定多 provider 并按 priority 排序
- [ ] 1.3 新增迁移：创建 `provider_health_checks` 表，含 provider_id、status、latency、error、检测时间、TTL 相关字段；验证：可写入主动测试与被动标记两类记录
- [ ] 1.4 新增迁移：创建 `visual_parse_providers` 表（含 `backend_kind` ∈ {ocr, multimodal_llm, layout, table, third_party_api, private_service}、`deployment_kind`、连接字段、`tenant_id`）；验证：可分别持久化公网与私有化两条视觉解析配置
- [ ] 1.5 新增迁移：创建 `document_parse_jobs` 表，含 `document_id`、`document_version`、状态字段（pending/parsing/succeeded/failed 及内部子状态）、`failure_reason`、时间戳、操作者；验证：可创建并按 (document_id, version) 查询作业
- [ ] 1.6 新增迁移：创建 `document_visual_parse_results` 表，落 §16.5 结构化契约字段（文本、页码、段落、坐标、标题层级、表格结构、图片位置、页眉页脚、置信度、失败原因、chunk 定位信息、`tenant_id`）；其访问控制**继承来源文档级访问权限（由 c01 `document_permissions` 派生执行）**，**不引入独立 `chunk_acl` 物理列、亦不作为 §11.9 六维 RAG 检索过滤维**（与 `document_chunks` 的 `document_acl`(派生)/`chunk_acl`(物理列) 二维口径区隔，消除命名漂移）；验证：单文档解析结果可完整写入并按租户隔离读取，受限文档解析结果由文档级权限派生可见性、表无独立 `chunk_acl` 列
- [ ] 1.7 新增迁移：创建/补全 `document_chunks` 与 `embeddings` 表，`document_chunks` 含 §16.3 全部字段（`document_id`、`source_type`、`source_title`、`source_url`、`pubmed_id`、`doi`、`journal`、`year`、`section`、`page`、`paragraph_index`、`chunk_text`、`embedding`、`chunk_acl`），其中 §16.3 单一 `acl` 字段正名为 `chunk_acl`（chunk 级 ACL 物理列，默认继承来源文档、允许写入严于文档级的范围），并支持 `superseded` 标记；`document_acl` 维不落 chunk 物理列、由 c01 `document_permissions` 经 join 派生执行，与 c04 rag-retrieval、c06 D6/8.4 两维口径一致；`embeddings` 表 MUST 携带 `chunk_id` 外键引用 `document_chunks(id)`，向量行经该外键回连 chunk 继承 `tenant_id`/`chunk_acl`（与 §16.3「`embedding` 属 chunk 元数据」一致），MUST NOT 在 `embeddings` 上重复物化 `tenant_id` 列以免与 chunk 维双源；验证：建表后 `chunk_acl` 列存在且可写入严于文档级的值、缺失字段可显式置空、可按 (document_id, version) 标记旧 chunk superseded、`embeddings.chunk_id` 外键约束指向 `document_chunks(id)` 且 `embeddings` 表无独立 `tenant_id` 列（租户维经 chunk 回连派生）

## 2. 模型 Provider 抽象层（四协议 Adapter + 9 类 capability 接口）

- [ ] 2.1 定义 9 个 capability 内部接口（`ChatCompletion`、`Summarize`、`Translate`、`Embed`、`Rerank`、`VisualParse`、`TermExtract`、`Proofread`、`OutlineGen`），上层仅依赖统一接口；验证：上层调用方代码不出现任何协议/厂商 SDK 直接引用
- [ ] 2.2 实现 `OpenAICompatAdapter`，按 `base_url`/`api_key`/`model` 接入并适配到 capability 接口；验证：新增一个 OpenAI-compatible provider 后上层经统一接口可成功调用（对应「通过 OpenAI-compatible 协议接入 provider」场景）
- [ ] 2.3 实现 `AnthropicMessagesAdapter`，仅注册生成类 capability（Chat/Summarize/Translate/TermExtract/Proofread/OutlineGen）；验证：将 Anthropic provider 绑定到 Embedding/Rerank/视觉解析时配置层拒绝并提示需单独配置（对应「Anthropic 协议不可绑定 Embedding/Rerank/视觉解析」场景）
- [ ] 2.4 实现 `LocalGatewayAdapter` 与 `ThirdPartyAdapter` 两类协议适配；验证：本地网关与第三方 provider 各自经统一接口可被同一上层调用方无差别调用
- [ ] 2.5 实现公网/私有化双入口配置：同一 `model_providers` 表以 `deployment_kind` 区分两套字段语义，二者独立配置、独立启用；验证：为「医学翻译」同时配置公网与私有化 provider，公网含超时/重试、私有化含 `network_policy`，启用状态互不影响（对应「公网与私有化入口独立配置」场景）
- [ ] 2.6 实现私有化部署零公网 provider 启动：缺少任何公网 provider 时系统正常启动；验证：仅配置各用途私有化 provider 时系统不报错且各用途可经私有化路径调用（对应「私有化部署完全不配置公网模型」场景）

## 3. 用途路由、优先级与 fallback 容错

- [ ] 3.1 实现路由器按 capability 从 `model_routes` 取 enabled provider 并按 `priority` 排成 fallback 链；验证：「LLM 聊天/AIMed」走 provider A、「Embedding」走 provider B 互不干扰（对应「不同用途绑定不同 provider」场景）
- [ ] 3.2 实现用途未绑定 provider 时拒绝调用且不静默回退到其它用途；验证：Rerank 无启用 provider 时返回「该用途未配置可用模型」错误（对应「用途未绑定任何 provider 时拒绝调用」场景）
- [ ] 3.3 实现 fallback 错误分类（可 fallback：超时/5xx/429/健康 down/缺 key；不 fallback：脱敏未过/超上限/401 鉴权错/内容安全拒绝）与按序切换；验证：provider A 超时后自动切 provider B 并在 `audit_logs` 写入含原 provider、失败原因、切换目标、时间戳的记录（对应「高优先级失败时按优先级 fallback」场景）
- [ ] 3.4 实现公网失败时切换同用途私有化 provider；验证：公网 provider 失败且存在私有化 provider 时切私有化完成调用并落审计（对应「公网不可用时切换私有化」场景）
- [ ] 3.5 实现全部 provider 失败时终止重试并返回明确不可用错误；验证：所有 provider 依次失败后逐条失败原因写入 `audit_logs` 且不静默成功（对应「全部 provider 均失败时返回明确错误」场景）

## 4. 公网调用前置脱敏门控（医疗红线）

- [ ] 4.1 在路由器入口对 `deployment_kind`=public 的 provider 强制前置消费 **c09 `redaction-gateway`** 的 PHI/PII 判定（命中规则 + 脱敏文本 + 置信度）。PHI/PII 识别+脱敏引擎唯一 owner=c09（c01 不实现），本期仅落「门禁接缝 + 默认拒绝公网」的保守降级；验证：本期验收「接缝存在且公网默认拒绝/降级」（对应「脱敏门禁未接入时公网默认拒绝（本期保守降级）」场景）；脱敏成功且置信度达标时以脱敏后内容调用公网并落「脱敏已通过」审计（对应「脱敏通过后允许调用公网模型」场景）——**真实判定接入与公网放开的端到端验收随 c09（phase 9）落地完成，不在 phase 3 验收**
- [ ] 4.2 实现识别失败/置信度不足时禁公网并切私有化；验证：识别失败且存在私有化 provider 时禁用公网、改走私有化并记录门禁触发与切换原因（对应「识别失败时禁止公网调用并切私有化」场景）——真实判定接入随 c09 落地完成
- [ ] 4.3 实现识别服务不可用且无私有化路径时拒绝调用；验证：无任何明文外发，返回明确错误并落拒绝审计（对应「识别服务不可用且无私有化路径时拒绝调用」场景）——真实判定接入随 c09 落地完成
- [ ] 4.4 实现 private provider 出网网关执行 `network_policy`：标记「禁止出网」的私有化 provider 即便误配公网域名也被传输层拦截；验证：私有化 provider 误配公网域名时请求被拦截并告警

## 5. 连通性测试与健康检查

- [ ] 5.1 实现每个 provider 的主动连通性测试入口：对生成类发最小 chat、对 Embedding/Rerank/视觉解析发对应轻量探针，结果（成功/失败、延迟、原因、时间戳）写 `provider_health_checks` 并回显；验证：成功时写成功记录（对应「触发连通性测试成功」场景）
- [ ] 5.2 实现连通性测试失败记录：`base_url` 不可达或鉴权失败时返回具体原因且不标记为可用；验证：失败时写失败记录且该 provider 不进入可用路由（对应「连通性测试失败记录原因」场景）
- [ ] 5.3 实现 Embedding 与 Rerank provider 的独立连通性验收；验证：分别对 Embedding 与 Rerank 触发测试，各自独立发探针并分别记录结果互不影响（对应「Embedding/Rerank 独立连通性验收」场景，覆盖 §24.9）
- [ ] 5.4 实现运行期健康检查（周期探测 + 调用失败被动标记 + TTL + 多次失败才判 down），路由器跳过 down 的 provider；验证：down provider 被路由跳过、TTL 过期后可恢复参与路由

## 6. 文档视觉解析服务（可插拔后端 + 统一契约 + 双路径）

- [ ] 6.1 实现 `VisualParse` capability 复用统一 provider/route/health 机制，`backend_kind` 对上层透明；验证：底层由 OCR 切换为多模态/第三方时上层不改动仍按统一契约消费（对应「上层不感知底层实现」场景）
- [ ] 6.2 实现各后端归一化为 §16.5 统一结构化输出契约（文本/页码/段落/坐标/标题层级/表格结构/图片位置/页眉页脚/置信度/失败原因/chunk 定位）并落 `document_visual_parse_results`；验证：复杂 PDF 解析后结果含全部字段（对应「输出完整结构化字段」场景）
- [ ] 6.3 实现非 OCR 场景支持：含图表/表格/复杂版式 PDF 返回表格结构、图片位置与版式信息而非仅纯文本；验证：复杂 PDF 输出表格结构与图片位置（对应「非 OCR 场景被支持」场景）
- [ ] 6.4 实现公网视觉解析 provider 真实接入与连通性测试；**本期可验证**：配置公网视觉解析 provider 后可触发连通性测试（不发送 PHI），公网解析 provider 配置 + 连通性测试可触发即通过；**公网解析经脱敏放行后的真实解析验收随 c09（phase 9）落地完成，不在 phase 3 验收**（对应「公网与私有化解析独立配置」公网侧 + 「脱敏通过后调用公网解析」场景，覆盖 §24.9 公网解析验收的本期可验证半段）
- [ ] 6.5 实现私有化视觉解析 provider 接入与仅私有化离线降级演示；验证：无公网环境仅配私有化 provider 时扫描/复杂文档全部走私有化路径不失败（对应「仅私有化解析的离线部署」场景，覆盖 §24.9 私有化解析验收）
- [ ] 6.6 实现公网视觉解析前置脱敏门控接缝：消费 **c09 `redaction-gateway`** 判定（c01 不实现，识别脱敏引擎唯一 owner=c09），识别失败/不足时禁公网、切私有化、无私有化则拒绝并落审计；本期 `redaction-gateway` 未接入前公网解析默认拒绝/降级；验证：识别失败且有私有化 provider 时改走私有化、门禁未接入时公网解析默认拒绝（对应「识别失败时禁止公网解析并切私有化」「识别服务不可用且无私有化解析时拒绝」「脱敏门禁未接入时公网解析默认拒绝（本期保守降级）」场景）——真实判定接入与公网解析放开的端到端验收随 c09（phase 9）落地完成
- [ ] 6.7 实现解析失败返回失败原因：图片清晰度过低时返回 `failure_reason` 且不输出空置信度伪结果，并使上游作业转失败；验证：低质量图片解析返回失败标志与原因（对应「解析失败返回失败原因」场景）
- [ ] 6.8 实现表格与页码质量指标：对内置视觉解析测试集统计页码定位成功率 ≥ 90%、表格结构识别成功率 ≥ 85%、引用源页码误差 ≤ 1 页，低于阈值结果被标记（**该「内置文档视觉解析测试集」由 c09 提供——§20.4 内置验收测试集 / Demo 数据集，`eval_cases` owner；c03 本期仅在内置子集上自验指标计算与低置信度标记逻辑，§20.3 三项数值指标的最终达标判定随 c09 Evals 跑批 tasks 10.2 / phase 9 完成，不在 phase 3 单独终判**）；验证：在 c09 提供的内置子集上跑出三项指标的计算结果且低置信度结果带标记，三项数值阈值的最终达标判定随 c09 phase 9 落地（对应「页码定位达标」「低置信度结果被标记」场景）
- [ ] 6.9 实现视觉解析结果租户隔离与审计：`document_visual_parse_results` 继承来源文档 `tenant_id` 与文档级访问权限（由 c01 `document_permissions` 派生执行，不引入独立 `chunk_acl` 物理列、不作为 §11.9 六维 RAG 检索过滤维），解析路径/provider/成功失败/置信度/原因写 `audit_logs`；验证：受限文档解析结果不对未授权放开且解析过程可溯源（对应「解析结果继承文档权限」「解析调用可审计」场景）

## 7. 文本/扫描文档解析入库流水线

- [ ] 7.1 实现上传成功后异步创建 `document_parse_jobs`：上传立即返回成功，异步创建待处理作业关联 document_id 与当前版本；验证：上传不被阻塞且作业记录生成（对应「上传后异步创建解析作业」场景）
- [ ] 7.2 实现 ONLYOFFICE 保存回调（§9.8）触发新版本异步解析：不阻塞保存回调返回（成功率 ≥ 99%），针对新 `document_version` 创建作业；验证：保存回调返回不被解析阻塞且新版本作业生成（对应「保存回调触发新版本解析」场景）
- [ ] 7.3 实现解析状态机（pending → detecting →（visual_parsing?）→ chunking → embedding → indexing_handoff → succeeded|failed，其中 detecting/visual_parsing/chunking/embedding/indexing_handoff 内部子状态对外统一归并为 parsing、成功终态统一为 succeeded）与成功流转；验证：pending 作业被取走后流转至 succeeded 并记录完成时间（图与断言用同一终态名 succeeded，对应「解析成功状态流转」场景）
- [ ] 7.4 实现失败处理：失败置 `failed` 并记 `failure_reason`，回滚已产生的半成品 chunk 不写索引；验证：文档损坏时不残留 chunk（对应「解析失败记录原因且不写半成品」场景）
- [ ] 7.5 实现失败作业重试：管理员触发后重置 pending 重新执行并写审计；验证：failed 作业可重试且重试动作落审计（对应「失败作业可重试」场景）
- [ ] 7.6 实现文本型文档直接 chunk 切分：可抽取文本的 DOCX/PDF 按段落/章节切分写 `document_chunks`，经 `Embed` 写 `embeddings` 并发出「索引就绪」事件（BM25/向量索引构建由 c04/c06 消费该事件完成，c03 不构建检索索引）；验证：DOCX 切分入库、embedding 写入并发出「索引就绪」事件、本期不含任何 BM25/向量索引构建（对应「文本型文档直接切分入库」场景，与 7.10 对齐）
- [ ] 7.7 实现扫描/复杂文档以视觉解析结构化输出为切分输入：先调 `VisualParse` 再以其页码/段落/标题层级/chunk 定位切分而非直接抽文本；验证：扫描 PDF 经视觉解析结果切分且页码段落可溯源（对应「扫描文档以视觉解析输出为切分输入」「结构化输出作为切分输入」场景）
- [ ] 7.8 实现 chunk 元数据完整性：每个 chunk 写 §16.3 全部字段，无值字段显式置空，`page` 与 `paragraph_index` 可用于引用定位（页码误差 ≤ 1 页）；验证：带页码 PDF 切分后每 chunk 携带 page/paragraph_index（对应「chunk 写入完整元数据」「chunk 携带页码与段落定位」场景）
- [ ] 7.9 实现 chunk 的 ACL 与租户隔离继承：chunk 继承来源文档 `tenant_id`，chunk 级 ACL 写入 `chunk_acl` 列，默认继承文档级 ACL 不放宽，允许 c06 写入严于文档级的 `chunk_acl`；文档权限收紧后重解析 chunk 反映收紧范围；后续检索按 §11.9 六维 `tenant_id`/`kb_id`/`user_id`/`role`/`document_acl`/`chunk_acl` 过滤（`document_acl` 由 `document_permissions` 派生、不落 chunk 列）；验证：受限文档 chunk 的 `chunk_acl` 与来源一致、收紧后不暴露超权内容、`chunk_acl` 列可独立物化并供 c04 chunk_acl 维过滤与 c06 8.4 写入消费（引用 §11.9 六维）（对应「chunk 继承来源文档 ACL」「来源文档权限收紧后 chunk 同步约束」「chunk_acl 列可独立物化且可严于文档级」场景）
- [ ] 7.10 实现 Embedding 写入经 `Embed` capability 走路由/降级生成向量写 `embeddings`，每条向量行 MUST 经 `chunk_id` 外键回连 `document_chunks`，租户维与 `chunk_acl` 一律经该外键从 chunk 派生、不在 `embeddings` 上独立物化 `tenant_id`；并在 `indexing_handoff` 仅发「索引就绪」事件、不做向量库检索装载（划清与 c04/c06 边界）；验证：embedding 写库且 `chunk_id` 外键有效回连 chunk、本期不含检索/召回逻辑
- [ ] 7.11 实现重解析幂等：同 (document_id, version) 以 version 去重，重解析生成新作业并将旧 chunk 标记 superseded 保留版本可溯源；验证：同版本重复触发不产生重复 chunk、重解析后旧 chunk 被 superseded
- [ ] 7.12 实现解析作业可审计：作业创建/状态流转/失败原因/重试等作业生命周期审计**仅写 `audit_logs`**（复用 c01 既有列 操作者/role/tenant_id/操作类型/对象/时间/result/failure_reason），记录 document_id、版本、状态、failure_reason、操作者；作业生命周期 MUST NOT 写 `document_events`（§10.6 仅承载 6 类触发源，作业 create/parsing/failed/retry 不在该枚举内）；验证：可返回单文档解析全过程时间序列审计且失败原因可查、且作业生命周期无任何 `document_events` 写入（对应「解析全过程可溯源」「解析失败原因落审计」场景）
- [ ] 7.13 实现 c03 作为 `document_events` **纯消费方**（不产生任何 `event_type`）消费 §10.6 全部 6 类触发源发起重新解析与索引：消费 `upload_success`(c01产生)/`save_new_version`(c02产生)/`ai_writeback`(c02产生)/`translation_done`(c07产生)/`template_created`(c08产生)/`manual_reindex`(c06产生)，逐类为对应 `document_id`/`version_id` 异步创建解析/重解析作业（重解析生成新作业、旧 chunk 标记 superseded）；`manual_reindex` 由 c06 产生、c03 仅消费触发重解析，c03 MUST NOT 自行产生 `manual_reindex` 或任何其它 `document_events`；验证：6 类事件均能驱动 c03 创建对应作业、且 c03 在 `document_events` 上无任何写入/生产（对应「消费 upload_success / save_new_version / ai_writeback 发起解析」「消费 translation_done / template_created 发起解析」「消费 c06 产生的 manual_reindex 发起重解析」场景）

## 8. 管理后台配置入口与权限（§17.7 配置部分）

- [ ] 8.1 实现管理后台「模型与评测管理」的模型配置入口（§17.7 配置部分：公网/私有化模型配置、OpenAI-compatible/Anthropic provider、Embedding 配置、Rerank 配置、文档视觉解析配置），不含 Agent Evals/RAG 评测/引用准确率等评测项；验证：各配置入口可增删改查 provider 与路由绑定
- [ ] 8.2 实现模型配置租户隔离与 RBAC：provider/route/健康检查按 `tenant_id` 隔离，仅「模型与评测管理」权限角色可读写；验证：非授权角色访问/修改被拒绝并落审计、配置不被改动（对应「非授权角色无法修改模型配置」场景）
- [ ] 8.3 实现敏感凭据掩码返回：`api_key`/`token` 不以明文返回前端；验证：授权管理员查看配置时凭据被掩码（对应「敏感凭据不明文返回」场景）
- [ ] 8.4 实现配置层校验：每个被上层依赖的 capability 至少绑定一个 enabled provider，否则后台显式报缺并阻止该能力上线；主闭环涉及能力强制至少一条私有化/离线路径；验证：缺绑定能力被标记不可上线、私有化部署主闭环能力均有私有化路径

## 9. 端到端验收对齐 §24.9

- [ ] 9.1 验收 AIMed 用途**私有化**模型路径（本期可验证）：配置并经私有化 provider 完成一次 ChatCompletion 调用；验证：私有化路径成功且落审计（覆盖 §24.9「AIMed 私有化路径验收」本期可验证半段）
- [ ] 9.1b 验收 AIMed 用途**公网**模型路径（随 c09 落地）：公网 provider 经脱敏放行后完成一次 ChatCompletion 调用并落审计——**真实判定接入与公网放开的端到端验收随 c09（phase 9）落地完成，不在 phase 3 验收**（覆盖 §24.9「AIMed 公网路径验收」公网半段）
- [ ] 9.2 验收医学翻译用途**私有化**模型路径（本期可验证）：经私有化 provider 完成一次 `Translate` 调用；验证：私有化路径成功且落审计（覆盖 §24.9「医学翻译私有化路径验收」本期可验证半段）
- [ ] 9.2b 验收医学翻译用途**公网**模型路径（随 c09 落地）：公网 provider 经脱敏放行后完成一次 `Translate` 调用并落审计——**真实判定接入与公网放开的端到端验收随 c09（phase 9）落地完成，不在 phase 3 验收**（覆盖 §24.9「医学翻译公网路径验收」公网半段）
- [ ] 9.3 验收禁用公网模型时主闭环经私有化/离线完成：关闭全部公网 provider，跑「上传/扫描件 → 视觉/文本解析 → Embedding 写入」链路；验证：全链路经私有化路径成功完成不报错（覆盖 §24.9「禁用公网时主闭环可经私有化或离线完成」）
- [ ] 9.4 验收 fallback 审计四要素齐全：构造公网失败触发切私有化，核对 `audit_logs` 含 provider、失败原因、切换目标、时间戳；验证：四要素均落库可溯源（覆盖 §24.9「fallback 必须记录 provider、失败原因、切换目标和审计日志」）
