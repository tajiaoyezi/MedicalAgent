## Context

c01（foundation）已落地租户/RBAC、文档与对象存储（MinIO/S3 + PostgreSQL）、审计骨架。**c01 不实现 PHI/PII 识别与脱敏**（已声明属 c09 security-evidence）；PHI/PII 识别+脱敏引擎 `redaction-gateway` 及 `privacy_detection_rules` / `privacy_redaction_events` 表的唯一 owner 是 c09。c03 仅在公网出口**预留脱敏门禁接缝**（消费 c09 `redaction-gateway` 的契约），真实识别/脱敏判定由 c09 落地。但平台尚无任何模型接入与文档解析能力：上传的论文/扫描件还停留在原始文件，既不能被 AIMed 消费，也无法进入 RAG 检索。后续 c04（aimed-rag-citation）、c05（ai-panel-recent-tasks）、c06（knowledge-admin）、c07（medical-translation）全部阻塞在两块公共底座上——「模型怎么接」与「文档怎么变成可检索的 chunk」。

本 phase 是 9 阶段中的第三阶段，依赖 c01，目标是在**离线/私有化优先**与**医疗安全红线**约束下，把这两块底座做实，对外暴露唯一、可降级、可审计的入口。

约束来自 PRD：
- §16.4 模型 Provider 抽象层——四类协议（OpenAI 兼容 / Anthropic Messages / 本地网关 / 第三方）、每种模型能力同时具备公网与私有化两个配置入口、按用途绑定、优先级与 fallback、私有化部署可不配公网。
- §16.5 文档视觉解析服务——底层可由 OCR / 多模态 / 版面分析 / 表格识别 / 第三方 / 私有化实现，对上层透明，统一结构化输出契约。
- §16.3 chunk 元数据 / §9.8 保存后异步解析 / §16.1 数据源分级与离线。
- §24.9 验收红线——禁用公网模型时主闭环仍可经私有化或离线完成；fallback 必须记录 provider / 失败原因 / 切换目标 / 审计日志。
- 公网调用前置脱敏（医疗安全红线）：识别失败或置信度不足时禁止走公网。

数据模型——本 phase 唯一 owner 建表（PRD §18 命名）：`model_providers`、`model_routes`、`provider_health_checks`、`visual_parse_providers`、`document_parse_jobs`、`document_visual_parse_results`、`document_chunks`、`embeddings`。消费 c01 所建表：读取 `documents` / `document_versions`、写入 `audit_logs`。脱敏门禁相关 `privacy_detection_rules` / `privacy_redaction_events` 由 **c09**（`redaction-gateway`）建表与落地，c03 仅在公网出口消费其判定结果，不建表、不实现识别算法。

## Goals / Non-Goals

**Goals:**

- 提供统一**模型 Provider 抽象层**：四类协议适配到统一内部接口，公网/私有化双入口，可独立配置 9 类模型能力（LLM/AIMed、长文总结、医学翻译、Embedding、Rerank、文档视觉解析、术语抽取、校对/润色/排版、PPT 大纲生成）。
- 提供**用途路由与容错**：同一用途（capability）可绑定多个 provider，按优先级选用，失败/不可用时按序 fallback；私有化部署允许零公网 provider。
- 提供**连通性测试与健康检查**：配置后可主动测试连通性，运行期周期性/失败触发健康检查，结果落库并影响路由决策。
- 提供**文本型文档解析入库流水线**：保存/上传后异步发起解析作业（`document_parse_jobs`），完成 chunk 切分并写入符合 §16.3 的 chunk 元数据，管理作业状态。
- 提供**文档视觉解析服务**：可插拔后端、公网/私有化双配置，输出 §16.5 统一结构化契约，作为复杂/扫描文档的结构化输入喂给文本解析继续切分。
- 落实**离线/私有化降级**与**公网调用前置脱敏**两条红线，并把 fallback、切换、健康检查、解析状态写入 `audit_logs`。
- 划清**解析 → chunk → 索引**的边界：本期产出 chunk 与 embedding 写入，把向量库检索/RAG/问答交给 c04、知识库导入交给 c06。

**Non-Goals:**

- 不实现向量检索、BM25、rerank 召回、合并去重、source compression、带引用问答（§16.2 检索流程归 c04）。
- 不实现知识库 URL 导入、PubMed 导入、知识库重建索引（归 c06）；本期只提供被它们复用的解析与 embedding 写入能力。
- 不实现翻译任务编排（`translation_jobs`/分段/术语，归 c07）；本期只提供翻译/视觉解析所需的 provider 与路由。
- 不实现 PHI/PII 识别与脱敏算法本体（PHI/PII 识别+脱敏引擎 `redaction-gateway` 及 `privacy_detection_rules`/`privacy_redaction_events` 唯一 owner=**c09**）；本期仅在公网出口预留门禁接缝、作为公网调用前置依赖**消费** c09 的判定结果。c01 不实现该能力。
- 不实现 Agent Evals / RAG 评测 / 引用准确率 / 模型评测后台（§17.7 中评测条目被 §22.2/§22.3 划入 V1.1/V1.2，非本期目标）。本期只覆盖 §17.7 中的模型/Embedding/Rerank/视觉解析配置入口。
- 不实现数字员工创建/运行/编排/执行历史。
- 不替上层产出诊疗/用药/医嘱结论；本期产物均为草稿/辅助素材，高风险确认链路由 c04/c05 落实。

## Decisions

### D1. Provider 抽象：四协议适配到统一内部接口（capability-oriented）

**决策**：定义一组按能力划分的内部接口（`ChatCompletion`、`Summarize`、`Translate`、`Embed`、`Rerank`、`VisualParse`、`TermExtract`、`Proofread`、`OutlineGen`），每类协议实现一个 Adapter（`OpenAICompatAdapter` / `AnthropicMessagesAdapter` / `LocalGatewayAdapter` / `ThirdPartyAdapter`）。`model_providers` 存连接级配置（§16.4 字段：`provider` / `base_url`(公网) 或内网 `base_url` / `api_key`|`token` / `model` / 超时 / 重试 / `network_policy`(私有化) / `enabled` / `default_priority` / `deployment_kind`=public|private）。上层只面向 capability 接口调用，不感知底层是哪家协议、哪个 model、公网还是私有化。

**理由**：§16.4 要求四类协议、9 类能力、双入口都可独立配置；只有把「协议适配」与「能力路由」解耦，才能让任意能力自由绑定任意 provider，并满足「Anthropic 主要用于生成类，embedding/rerank/视觉解析单独配服务」。

**备选与取舍**：
- (A) 每个 capability 写死一个 SDK 客户端 —— 简单，但无法支持同一能力多 provider/fallback、无法统一公网/私有化切换，违反 §16.4，否决。
- (B) 仅做一个「OpenAI 兼容」万能适配，要求私有化网关也兼容 OpenAI 协议 —— 实现最省，但 Anthropic Messages 协议形态不同、第三方视觉解析多为自定义 HTTP，硬套会丢能力，否决。
- (C, 选中) 协议 Adapter + capability 接口两层。多写 4 个 Adapter 的成本换取协议无关的路由与可降级性，符合 PRD 的「抽象层」定位。

### D2. 用途绑定与路由表 `model_routes`，优先级 + 顺序 fallback

**决策**：`model_routes` 记录 (capability, provider_id, priority, enabled) 的多对多绑定（一个能力可绑多 provider，一个 provider 可服务多能力）。路由器按 capability 取出 enabled 的 provider，按 `priority` 升序排成 fallback 链；调用时逐个尝试，遇可重试错误先按该 provider 的重试策略重试，仍失败或被健康检查标记不可用则切换下一个。每次切换写一条 `audit_logs`（含原 provider、失败原因 error_class、切换目标、capability、request_id）。私有化部署可使整条链全为 private provider。

**理由**：§16.4「同一功能可配置多个 provider 并设置优先级和 fallback」、§24.9「fallback 必须记录 provider、失败原因、切换目标和审计日志」直接要求。把绑定独立成 `model_routes`（§18 既有表）而非塞进 `model_providers`，是因为绑定是多对多且按能力差异化优先级。

**备选与取舍**：
- (A) 在 `model_providers` 上加 `capabilities` 数组字段 —— 省一张表，但无法给「同一 provider 在能力 X 优先、能力 Y 次选」设独立优先级，且违背复用 §18 `model_routes` 命名的约束，否决。
- (B) 权重/负载均衡式路由 —— POC 不需要分流，徒增复杂度，否决；保留为 Open Question。
- (C, 选中) 顺序优先级链 + 顺序 fallback。确定性、可审计、易演示「公网失败 → 私有化兜底」。

**fallback 触发分类**（决定是否切换 provider，避免无意义重试）：
- 可 fallback：连接超时、5xx、限流(429)、健康检查 down、provider 未配置 key。
- 不 fallback、直接失败上抛：脱敏前置未通过（见 D6）、输入超模型上限、鉴权配置错误(401，应告警而非静默切换)、内容安全拒绝。

### D3. 连通性测试 + 健康检查 `provider_health_checks`

**决策**：
- **连通性测试（主动）**：管理后台配置 provider 后可手动「测试连通」，对生成类发最小 chat、对 embedding/rerank/视觉解析发对应轻量探针；结果（latency、status、error）写 `provider_health_checks` 并回显。
- **健康检查（运行期）**：周期探测 + 调用失败被动标记。路由器读最近一次健康状态，跳过 down 的 provider。健康状态有 TTL，避免一次抖动长期拉黑。

**理由**：§24.9「Embedding / Rerank 支持独立配置和连通性验收」「文档视觉解析支持公网/私有化解析服务验收」；离线环境必须能在不发起真实业务请求的前提下验证私有化链路是否通。

**备选与取舍**：
- (A) 只在真实调用失败时被动 fallback，不做主动探测 —— 省事，但验收无法「配置后立即验证连通性」，且首个真实请求才暴露错配，演示体验差，否决。
- (B, 选中) 主动测试 + 被动标记 + TTL。

### D4. 公网 / 私有化配置字段差异与网络访问策略

**决策**：用同一张 `model_providers`，以 `deployment_kind`(public|private) 区分两套字段语义：
- public：`base_url`、`api_key`、超时、重试（§16.4 公网字段）。
- private：内网 `base_url`、`api_key`|`token`、`network_policy`（如：仅内网、禁止出网、允许指定网段）（§16.4 私有化字段）。

`network_policy` 是一条**强约束**：标记为 private 的 provider，其出站请求经统一出网网关校验，命中「禁止出网」时即便误配公网域名也会被拦截，防止私有化部署意外把 PHI 发到公网。公网 provider 反之要求显式启用且默认受脱敏前置（D6）门控。

**理由**：§16.4 明确给出两套字段且私有化多出「网络访问策略」；医疗红线要求私有化部署可证明「数据不出网」。

**备选与取舍**：
- (A) public/private 拆两张表 —— 字段重叠 80%，路由器要 union 查询，复用 §18 命名也更别扭，否决。
- (B, 选中) 单表 + `deployment_kind` 判别 + 出网网关执行 `network_policy`。

### D5. 视觉解析可插拔后端 + 统一结构化输出契约

**决策**：`visual_parse_providers` 与 `VisualParse` capability 复用 D1/D2/D3 的同一套 provider/route/health 机制（视觉解析就是一种 capability），底层 backend_kind ∈ {ocr, multimodal_llm, layout, table, third_party_api, private_service} 对上层透明。所有后端必须归一化为 §16.5 统一输出契约：文本内容、页码、段落、坐标(bbox)、标题层级、表格结构、图片位置、页眉页脚、置信度、失败原因、chunk 定位信息。该结果落 `document_visual_parse_results`，作为文本解析的结构化输入。

**理由**：§16.5「不是单一 OCR，底层可由多种实现」「对上层透明」「统一结构化输出」。归一化契约是后续 AIMed 引用定位、版式还原翻译、RAG chunk 切分共用的溯源基础。

**备选与取舍**：
- (A) 把视觉解析当成独立子系统、自带一套 provider 管理 —— 与模型 provider 重复造轮子，配置/健康检查/审计要写两遍，否决。
- (B, 选中) 视觉解析 = 一种 capability，复用统一 provider 抽象；仅在 backend 内部处理 OCR/多模态差异，对外是统一契约。
- 结构化契约 vs 仅返回纯文本：纯文本会丢坐标/表格/页码，无法支撑引用定位与版式还原（§16.5 使用场景），故必须保留完整结构化字段。

### D6. 公网调用前置脱敏门控（医疗红线，编排在路由器入口）

**决策**：路由器在选出 provider 后、真正发请求前判断 `deployment_kind`：
- private provider：直接调用（数据不出内网）。
- public provider：必须先消费 **c09 `redaction-gateway`** 的 PHI/PII 判定结果（`privacy_detection_rules` 命中 + 脱敏后文本 + 置信度）。识别失败、置信度不足或识别服务不可用 → **禁止走公网**，按 fallback 链尝试 private provider；若链上无可用 private provider，则整体失败并落审计，绝不降级为「明文发公网」。视觉解析的公网后端同样受此门控。

**phase 顺序与本期口径（关键）**：`redaction-gateway` 是被 c03–c07 前置消费的横切能力，唯一 owner=c09（phase 9，收尾环）。c03 处于 phase 3，**不依赖 c09 先就绪**：本期 c03 只落「预留门禁接缝 + public provider 默认拒绝」的保守降级——`redaction-gateway` 未接入前公网 provider 一律不启用，主闭环经私有化/离线路径跑通（符合 §16.4/§24.9 禁用公网仍经私有化/离线闭环）。真实判定接入与公网放开的端到端验收**随 c09 落地完成**，不在 phase 3 验收。即 c03 本期可验收的是「接缝存在 + 默认拒绝公网」，而非「公网经脱敏放行」。

**理由**：医疗安全红线 + proposal 公网前置脱敏条款 + §24.9 离线兜底。脱敏必须是公网调用的**前置硬门**而非旁路日志；门禁能力归 c09，c03 不得自行实现 PHI/PII 识别脱敏，也不得谎称由 c01 提供。

**备选与取舍**：
- (A) 脱敏作为调用方各自负责 —— 容易遗漏，单点违规即泄露 PHI，否决。
- (B, 选中) 在统一路由器入口集中门控，调用方无法绕过。把门控放在抽象层是「唯一入口」价值的核心体现。

### D7. 文本解析流水线与 `document_parse_jobs` 状态机

**决策**：c02 的 ONLYOFFICE 保存回调（§9.8：写对象存储 → 建 `document_version` → 更新元数据 → 异步解析）与文件上传，均通过**事件**触发创建 `document_parse_jobs`（绑定 document_id + version）。作业状态机：`pending → detecting → (visual_parsing?) → chunking → embedding → indexing_handoff → succeeded | failed`。其中 `detecting / visual_parsing / chunking / embedding / indexing_handoff` 为内部子状态，对外（验收/审计/上层查询）统一归并为 `parsing`，成功终态统一为 `succeeded`（与建表 task 1.5 / `document-parsing` spec「解析状态机与失败处理」Requirement 的 `pending/parsing/succeeded/failed` 对外四态逐字一致；本 design 不再出现 `done` 别名）。
- detecting：判定文档类型——文本型（DOCX/可抽取文本 PDF/Markdown）直接进入 chunking；扫描/复杂/图片型先调 `VisualParse`（D5），用其结构化结果再进入 chunking。
- chunking：基于段落/标题层级切分，每个 chunk 落 §16.3 全部元数据（document_id、source_type、source_title、source_url、pubmed_id、doi、journal、year、section、page、paragraph_index、chunk_text、embedding、chunk_acl），其中 §16.3 单一 acl 正名为 `chunk_acl` 物理列（chunk 级 ACL），默认继承 c01 文档级 ACL、允许 c06 写入严于文档级的范围；`document_acl` 维不落 chunk 列，由 c01 `document_permissions` 派生执行（§11.9 六维：tenant_id/kb_id/user_id/role/document_acl/chunk_acl）。
- embedding：经 `Embed` capability（同样走路由/降级）生成向量写 `embeddings`。
- indexing_handoff：本期边界终点——只把 chunk+embedding 写库并发出「索引就绪」事件，**不**做向量库检索装载（交 c04/c06）。

幂等：同一 (document_id, version) 重复触发以 version 去重；重解析生成新作业、旧 chunk 标记 superseded，保证版本可溯源。失败可重试，状态与失败原因落 `audit_logs`。

**`document_events` 口径（对齐 c01 §10.6 闭合 6 类契约）**：解析作业的生命周期（create/parsing/failed/retry）属审计动作，**仅写 `audit_logs`**（c01 既有列 操作者/role/tenant_id/操作类型/对象/时间/result/failure_reason），绝不写 `document_events`（§10.6 仅承载 6 类触发源，作业生命周期不在该枚举内）。c03 在 `document_events` 上是**纯消费方、不产生任何 `event_type`**：c03 是 §10.6 全部 6 类触发源的唯一重解析/索引消费方，逐类产生方为 `upload_success`=c01、`save_new_version`/`ai_writeback`=c02、`translation_done`=c07、`template_created`=c08、`manual_reindex`=c06。c03 消费其中任一类即为对应 `document_id`/`version_id` 异步创建解析/重解析作业；其中 `manual_reindex` 由 c06（管理员重建索引）产生、c03 仅消费触发重解析，c03 **不再**自行产生 `manual_reindex`（撤回旧版「manual_reindex 是 c03 唯一生产类型」表述，对齐 c01/c06「c06 产生、c03 消费」owner 口径）。

**理由**：§9.8 异步解析、§16.3 chunk 元数据、proposal「只产出 chunk 与 embedding 写入，不含检索」的边界。

**备选与取舍**：
- (A) 保存回调里同步解析 —— 阻塞保存、大文件超时、违反 §9.8「异步」，否决。
- (B) 解析与 embedding 合成单步不可观测 —— 失败难定位、不利审计，否决。
- (C, 选中) 显式状态机异步作业，各步可观测、可重试、可审计，并在 `indexing_handoff` 划清与 c04/c06 的边界。

### D8. 解析 → chunk → 索引的边界（与 c04/c06 的契约）

**决策**：本期对外契约 = `document_chunks`（含 §16.3 元数据与 `chunk_acl` 物理列）+ `embeddings` + 「索引就绪」事件 + 视觉解析结构化结果。`chunk_acl` 是 chunk 级 ACL 唯一物理列，owner=c03（document_chunks 建表 owner），默认继承文档级 ACL、允许 c06 写入严于文档级的范围、可被 c04 按 `chunk_acl` 维过滤；`document_acl` 维不落 chunk 列，由 c01 `document_permissions` 派生（经 join / c06 注入预计算可见 document 集合执行），与 §11.9 六维 tenant_id/kb_id/user_id/role/document_acl/chunk_acl 一致。c04 消费 chunk/embedding 做向量+BM25 检索与带引用回答；c06 复用同一解析+embedding 写入做知识库入库与重建索引、并向 c03 所建 `chunk_acl` 列写覆盖值（只写值不改结构）；c07 复用翻译/视觉解析 provider 与版式结构。本期不持有任何检索/召回/问答逻辑。

**写侧归属（消除 BM25 口径冲突）**：**BM25 全文索引与向量倒排索引的构建装载在 c04（检索侧）完成（§16.2 检索流程），c03 不构建任何检索索引**。c03 的索引相关职责终于 `indexing_handoff`：仅把 chunk+embedding 写库并发出「索引就绪」事件；c04 消费该事件后构建/刷新 BM25 全文索引与向量倒排索引。**「索引就绪」事件的下游消费方=c04（唯一检索索引构建方）——此为硬契约而非建议**：c04 design/tasks MUST 显式新增一条「消费 c03 索引就绪事件构建/刷新 BM25 全文索引与向量倒排索引」的 Requirement/Scenario，使该 handoff 事件有唯一订阅者、消除孤儿事件（§16.2 写侧→检索侧闭合，对齐 §24.9 主验收闭环端到端可验证性）。c06（知识库侧）消费同一事件完成知识库入库与重建索引收尾。spec、tasks 7.6/7.10 与本契约一致。

**理由**：避免 c03 越界做 c04 的检索；让「写入」与「检索」解耦，knowledge-admin 与 aimed 能各自演进。

## Risks / Trade-offs

- [私有化 provider 误配公网域名导致 PHI 出网] → D4 `network_policy` 出网网关在传输层拦截；D6 在路由器入口对 public provider 强制脱敏前置门控，双层防护。
- [脱敏服务不可用时若仍放行公网，泄露 PHI] → D6 规定识别失败/不可用一律禁公网、仅走私有化，无私有化则整体失败并审计，绝不明文外发。
- [fallback 链被异常拖慢（逐个超时重试，演示卡顿）] → D2 区分可/不可 fallback 错误类型 + D3 健康检查预先剔除 down provider + 每 provider 设超时上限，控制最坏耗时。
- [健康检查抖动误杀可用 provider] → D3 健康状态设 TTL + 多次失败才标记 down，避免一次网络抖动长期拉黑。
- [视觉解析后端众多、输出契约难统一，坐标/表格质量参差] → D5 在 Adapter 内归一化为统一契约，缺失字段显式置空并带 `confidence`/`failure_reason`，下游据置信度决定是否人工复核；POC 先保证文本+页码+段落+表格结构可用，坐标尽力而为。
- [大文件/扫描件解析耗时长，作业堆积] → D7 异步作业 + 状态机可观测，按 tenant 排队；POC 不做分布式调度但预留队列接口。
- [chunk 切分粒度影响后续召回质量] → 本期产出元数据完整、可重切；切分策略参数化，c04 调优召回时可触发重解析（旧 chunk superseded）。
- [Anthropic 协议不支持 embedding/rerank，绑定到生成类以外能力会失败] → D1 capability 接口 + §16.4 规则约束：embedding/rerank/视觉解析必须绑定到具备该能力的 provider，配置层校验 + 连通性测试拦截错配。
- [私有化部署零公网 provider 时，某能力无任何可用 provider] → 配置层校验：每个被上层依赖的 capability 至少绑定一个 enabled provider，否则在后台显式报缺并阻止该能力上线；主闭环涉及能力强制要求至少一条私有化/离线路径（§24.9）。

## Migration Plan

1. **建表/迁移**（复用 §18 命名）：`model_providers`、`model_routes`、`provider_health_checks`、`visual_parse_providers`、`document_parse_jobs`、`document_visual_parse_results`、`document_chunks`、`embeddings`。仅新增表，无破坏性变更（specs 当前为空，本期全为新增能力）。
2. **抽象层落地**：实现 4 个协议 Adapter + 9 个 capability 接口 + 路由器（D1/D2）+ 脱敏前置门控**接缝**（D6，消费 c09 `redaction-gateway`，本期落「接缝 + 公网默认拒绝」保守降级，真实判定接入随 c09 落地）+ 出网网关（D4）。本期默认公网关闭、私有化优先；公网脱敏门禁的端到端验收依赖 c09（phase 9）。
3. **健康检查/连通性测试**（D3）与管理后台「模型与评测管理」配置入口（§17.7，仅配置部分，不含评测）。
4. **视觉解析服务**（D5）：先接 1 个私有化后端 + 1 个公网后端打通双路径验收。
5. **解析流水线**（D7）：挂接 c02 保存回调与上传事件，跑通 文本型 与 扫描型 两条解析路径到 `indexing_handoff`。
6. **验收对齐 §24.9**：分别验证公网/私有化模型路径、Embedding/Rerank 连通性、公网/私有化视觉解析、禁用公网时主闭环经私有化/离线完成、fallback 审计四要素齐全。

**Rollback**：本期均为新增表与新增服务，无既有契约改动。回滚 = 停用解析作业触发器 + 下线 provider 配置入口 + 保留已写 chunk/embedding（对上层只读无害）。保存回调（c02）在解析订阅者缺失时退化为「只存版本不解析」，不影响 c02 既有保存闭环。

## Open Questions

- chunk 切分的默认粒度（按 token 数 / 按段落 / 按标题层级混合）与默认 `chunk_size`/overlap 取值，需结合 c04 召回评测确定；本期先参数化、给保守默认。
- 视觉解析「坐标(bbox)」精度要求：POC 是否强制所有后端返回精确 bbox，还是允许低置信度近似？倾向允许近似 + 置信度标注，待 c04 引用定位明确需求后定。
- 健康检查周期与 down 判定阈值（连续失败次数 / TTL 时长）的具体取值，待部署环境网络稳定性确认。
- 路由是否需要「同优先级多 provider」的并行/分流策略 —— 本期按确定性顺序链实现，分流留待 V1.1。
- 重解析触发权限：谁可发起文档/知识库重解析（管理员 / 文档所有者）的细粒度 RBAC 待定；但「重建索引」入口归属已定死——`manual_reindex` 由 **c06** 作为唯一产生方（管理员重建索引时产生），c03 仅作消费方消费该事件触发重解析，不再自行产生任何 `document_events`（已对齐 c01/c06 owner 口径，本项不再开放）。
