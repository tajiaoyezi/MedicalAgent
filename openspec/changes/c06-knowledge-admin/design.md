## Context

医疗知识库（PRD §11）是 MedOffice AI 的院内权威知识入口与 RAG 数据源之一，聚合医学文献、医院制度、临床指南、SOP、药品说明书、科研资料，目标是高准确率、可溯源的检索与问答（§11.1）。本 phase（c06-knowledge-admin，9 阶段中的第 6 阶段）在 c03（model-and-parse：文档解析、分块、向量化、模型路由）与 c04（aimed-rag-citation：RAG 检索 + rerank + 引用溯源 + RAG 权限过滤，§16.2 检索流程）就绪后落地。

当前状态与约束：
- c03 已沉淀文档解析（含文档视觉解析）、chunk 分块、向量化（embeddings）与模型 Provider 抽象层（公网/私有化双入口 + fallback）。
- c04 已沉淀 §16.2 检索流程（Query Rewrite → 数据源选择 → 权限过滤 → BM25 + 向量 → 合并去重 → rerank → source compression → 生成带引用答案）与 §16.3 chunk 元数据、citations 溯源结构。
- 本 phase 不重复定义解析服务、向量化、模型路由与 RAG 检索内核，而是把这些能力组织成「13 库管理 + 受控导入 + 检索问答」的闭环，并补齐知识库特有的卡片/排序/置顶模型、导入管线与授权状态机、知识库级权限过滤落点。
- 范围纪律：仅覆盖 PRD §22.1 P0（医疗知识库 13 个卡片 / 知识库搜索与问答 / 知识库上传·URL 导入·PubMed 导入）；不纳入 §22.2/§22.3 与数字员工。验收口径以 §24.3 为准。
- 部署环境可能无公网，所有核心闭环必须有离线/私有化降级路径。

关键干系人：平台管理员（可上传到任意库）、知识库管理员（可上传到自己管理的库）、终端用户（只读检索问答，默认隐藏管理类入口）、合规/审计角色（查看导入授权与问答日志）。

## Goals / Non-Goals

**Goals:**
- 预置 13 个核心医疗知识库（§11.2），每库以「预置空库 + 演示资料库」形态存在，但全部具备上传、导入、检索、问答、溯源、权限过滤能力。
- 提供知识库首页卡片（9 字段）与确定性排序规则（置顶 → 手动权重降序 → 同权重更新时间倒序 → 无配置创建时间倒序，§11.3）。
- 终端用户功能隔离（§11.4）：默认隐藏导入/新建/公开知识/我管理的/我加入的/历史会话侧边栏，普通用户只见预设库与被授权私有库。
- 受控导入管线（§11.5/§11.5.1）：批量上传、URL 导入、PubMed/PMC 导入、白名单官网来源导入；入库前强制预览确认；入库元数据 10 字段全程留痕。
- 导入授权状态机：未授权商业库默认抓取/批量导入禁止，授权状态不明确仅进临时预览、不写入正式公共库；普通用户上传只进个人资料区/会话上下文/私有库。
- 全局搜索（关键词/语义/混合 + 多维筛选，§11.6）与多库选择的知识库问答（§11.7），答案溯源到 知识库/源文档/章节/页码/段落/chunk/原文片段（§11.8）。
- RAG 检索强制权限过滤维度落到知识库链路：tenant_id / kb_id / user_id / role / document_acl / chunk_acl（§11.9）。
- 复用 c03 解析/向量化与 c04 RAG + Citation 管线，不重复造轮子；为 c07（术语/语料来源）、c09（知识库级 ACL 与审计取证）提供基线。

**Non-Goals:**
- 不实现 AIMed 模块本体与 PubMed 检索内核（属 c04），本 phase 仅复用其检索/溯源接口并把知识库作为新数据源接入。
- 不实现文档解析、视觉解析、分块、向量化与模型 Provider 路由本体（属 c03）。
- 不实现 PHI/PII 识别与脱敏引擎本体（redaction-gateway 唯一 owner=c09 security-evidence；c01 不实现、c03 仅在公网出口预留门禁接缝）。本 phase 仅在导入入向量化/公网调用前消费 c09 redaction-gateway 的判定接缝并按结果阻断/降级；本期默认公网关闭、私有化优先，redaction-gateway 未接入前不启用公网，仅私有化/离线路径跑通闭环（§16.4/§24.9）。
- 不含数字员工的创建/运行/编排/执行历史，不含 §22.2/§22.3 能力（如知识库自动更新、知识图谱、跨库知识融合等）。
- 普通用户的「新建知识库」入口属隐藏项（§11.4），本 phase 不向普通用户开放新建；但 §11.5/§17.3 明确「创建知识库」是后台管理能力，本 phase 保留「创建知识库」入口（RBAC 管控，仅创建空库并落 `knowledge_bases`，13 库 seed 已覆盖即开即用，成员管理完整后台留待 V1.1）。**创建授权谓词锚定**：创建一个尚不存在的新库无法以 per-kb 知识库 ACL 授权（待创建对象在创建时不存在、无从持有 per-kb 管理级授予），故创建授权 MUST 以「持有 c01 `permissions` 表登记的租户级 `kb:create` 权限点」判定——`kb:create` 由 c01-foundation auth-rbac 作为 `permissions` 唯一真值登记（默认授予 `admin`），c06 仅引用该权限点，MUST NOT 自造 `kb:*` 平台级权限点、MUST NOT 新增全局角色；「平台管理员」=持 `kb:create` 的 `admin` 角色，若需非 admin 创建则由 c01 把 `kb:create` 授予对应角色。被 Non-Goal 排除的仅为「普通用户新建」与「成员管理完整后台」，不排除「持 `kb:create` 用户创建知识库」这一应做能力。私有库仅作为权限可见性的目标对象存在，其成员关系完整管理后台归 V1.1。
- 不实现 §11.5 管理员能力清单中的「查看检索效果」管理员视图（检索命中/召回质量概览）：该项不在 §17.8 V1.0 POC 后台必做清单、§24.3 知识库验收亦未要求，属 V1.1 留待项；§11.5 同列的「查看问答日志」「重建索引」已在本期覆盖（kb-search-qa「检索与问答行为审计」与 kb-import「解析与索引状态可追踪」）。

## Decisions

### D1. 13 库预置：空库 + 演示库，配置驱动而非硬编码

**决策**：13 个知识库作为「平台级种子数据」在初始化（migration/seed）时写入 `knowledge_bases`，每库带固定 `kb_id`、名称、用途简介、`is_seed=true`、`data_source` 标签。每库挂接两类内容：演示资料库（随 Demo 数据集导入的少量真实可用资料，满足 §24.3「跨库检索/问答/溯源可演示」）与空库（无演示资料但导入管线就绪）。13 库清单与简介放在版本化 seed 配置文件中，不在代码里硬编码字符串。

**备选与取舍**：
- 备选 A：13 库纯硬编码在初始化代码里。取舍：改名/调简介需改代码、不可审计，放弃。
- 备选 B：13 库由管理员运行时手工创建。取舍：违反 §22.1「13 个卡片」即开即用、§24.3「13 个默认知识库清单符合 PRD」验收要求，无法保证清单一致，放弃。
- 选 seed 配置：清单可校验、可对照 PRD §11.2 验收、改动留痕，且演示库与空库共用同一套管线，符合「预置空库 + 演示资料库」原文。

### D2. 卡片字段与排序：物化卡片字段 + 确定性多级排序键

**决策**：卡片 9 字段（名称/ID/创建人/简介/成员人数/文档数量/更新时间/数据源/置顶状态）中，成员人数、文档数量为聚合值（成员人数 = 该知识库 ACL/`document_permissions` 中被授予读取/问答及以上权限的、当前租户内可见用户的去重计数，本期不依赖独立 `kb_members` 表；文档数量来自 `kb_documents` 中 `index_status=indexed` 的计数），随导入/成员变更增量更新并缓存到 `knowledge_bases` 上以避免列表页 N 次聚合查询；更新时间取该库最近一次「内容/配置变更」时间。排序在 SQL 层用确定性多级 ORDER BY 实现：`is_pinned DESC, manual_weight DESC NULLS LAST, updated_at DESC, created_at DESC`，保证 §11.3 四级规则与「无配置」兜底一字不差，且分页稳定。

**备选与取舍**：
- 备选 A：列表页实时 COUNT 成员/文档。取舍：13 库 × 高频列表访问下聚合开销大且排序不稳定，放弃；改为物化计数。
- 备选 B：排序在应用层内存排序。取舍：与分页冲突、规则散落易错，放弃；下沉到 SQL ORDER BY 表达式，规则即代码即文档。
- `manual_weight` 用 NULLS LAST 精确表达「无配置时」回退到时间排序，避免把空权重当 0 与显式 0 混淆。

### D3. 导入管线：统一「来源适配器 → 暂存预览 → 授权闸门 → 入库」四段式

**决策**：所有来源（批量上传 / URL / PubMed / PMC / 白名单官网）走同一条入库流水线，差异收敛在「来源适配器」层：
1. **来源适配器**：上传适配器取文件流；URL/白名单适配器抓取页面或文件；PubMed/PMC 适配器走 c04 已有的 PubMed 接入（公网或离线缓存）取结构化文献。适配器统一产出「原始资料 + 来源元数据（source_type/source_url/pubmed_id/doi 等，对齐 §16.3）」。
2. **暂存预览**：原始资料先落临时预览区（staging），调用 c03 解析/分块得到预览 chunk，调用脱敏判定（PHI/PII），把「来源/授权状态/解析状态/预览内容/脱敏结果」呈现给导入人。**入库前必须预览确认**（§11.5.1）。
3. **授权闸门**（见 D4 状态机）：根据来源类型 + 白名单规则 + 管理员授权决定能否写入正式公共库。
4. **入库**：通过闸门后，复用 c03 向量化写 `document_chunks`/`embeddings`，在 `kb_documents` 落 10 字段元数据，写 `audit_logs`；`index_status→indexed` 的推进与 D2 物化计数（`document_count`/`updated_at`）刷新不由本步直接置位，而是由 c06 消费 c03「索引就绪」事件触发（c03 是该事件唯一产生方、c04 与 c06 共同消费，c06 据此完成知识库入库与重建索引收尾）：c03 解析/分块/向量化完成发出「索引就绪」事件后，c06 把对应 `kb_documents.index_status` 置 `indexed` 并在同一事务内刷新该库 `document_count`（仅计 `index_status=indexed`）与 `updated_at`，该事件为 `index_status=indexed` 与卡片计数刷新的唯一触发源；重建索引（manual_reindex→c03 重解析→再发索引就绪）收尾走同一消费路径。

**备选与取舍**：
- 备选 A：每种来源各写一条独立导入逻辑。取舍：预览确认、授权闸门、脱敏、留痕四类横切逻辑被复制 5 份，易漏红线校验，放弃。
- 选四段式：来源差异隔离在适配器，红线（预览/授权/脱敏/留痕）只实现一次且对所有来源强制生效，符合「surgical / 简单优先」。
- staging 与正式库物理隔离（临时预览区不进正式检索索引），确保「授权不明只进临时预览、不写入正式公共库」从存储层即成立，而非仅靠状态字段约束。

### D4. 授权状态机 + 白名单规则：来源类型决定默认门，白名单/管理员授权放行

**决策**：在 `kb_documents` 上承载导入授权状态字段（`copyright_status` / `authorization_status`），状态机如下：

```
来源进入 → [staging:pending_preview]
  └─ 已预览确认 → 判定来源类型：
       ├─ 上传(管理员/库管理员到授权库)         → authorized      → 可入正式库
       ├─ PubMed/PMC（合规开放来源）            → authorized      → 可入正式库
       ├─ URL/官网 命中白名单规则               → authorized(白名单规则ID留痕) → 可入正式库
       ├─ URL/官网 未命中白名单但管理员显式授权  → authorized(授权确认人留痕)   → 可入正式库
       ├─ URL/官网 授权状态不明确               → preview_only    → 仅临时预览，禁止写正式公共库
       └─ 未授权商业库 / 镜像站 / 下载链接       → rejected        → 阻断，禁止抓取/批量导入
```

白名单规则建模为平台级「来源白名单」配置（域名/来源标识 → `whitelist_rule_id` + 是否允许 + 授权说明），URL/官网导入时按域名/来源匹配；命中即 `authorized` 并把 `whitelist_rule_id` 写入 `kb_documents`；未命中进入「管理员显式授权」或「preview_only」分支。授权确认人（`authorized_by`）与白名单规则 ID 全程留痕（§11.5.1 必录字段）。状态机的「写入正式公共库」动作只对 `authorized` 开放，`preview_only`/`rejected` 在存储层即被 staging 隔离（D3）。

**消费 c04 取数授权标记、不独立重建三态（与 c04 pubmed-data-service 对齐）**：URL/PMC/PubMed 取数路径进入本授权闸门时，其初始授权标记 MUST 取自 c04 pubmed-data-service 对每个 `RetrievedSource` 已返回的 `authorized` / `preview_only` / `rejected` 标记作为闸门输入，本 phase MUST NOT 从零重新判定该三态。c06 仅在 c04 标记之上叠加白名单规则匹配、管理员显式授权裁决与 staging 隔离，最终落库裁决以 c06 kb-import 契约为唯一真值（c04 已声明 `RetrievedSource` 标记供 c06 消费、裁决以 c06 为唯一真值）。本约束消除「c04 判 authorized、c06 闸门判 preview_only」的双算漂移面：上传适配器路径无 c04 取数标记，直接由本闸门依来源类型与授权裁决定状态。

**备选与取舍**：
- 备选 A：只用布尔 `is_authorized`。取舍：无法区分「授权不明（preview_only，可补授权）」与「明确拒绝（rejected，红线禁止）」两种负向路径，§24.3 要求「被阻止 或 仅进临时预览」二者并存，放弃。
- 备选 B：白名单写死代码常量。取舍：新增可信官网来源需改代码、不可审计、不能按租户配置，放弃；改为可配置规则表 + 规则 ID 留痕。
- 选「来源类型默认门 + 白名单/管理员授权放行 + 三态状态机」：默认安全（未知来源不会误入公共库），放行有据可查（规则 ID/授权人），负向路径与 §24.3 一一对应。

### D5. 检索与问答：复用 c03 解析/向量与 c04 RAG 管线，知识库作为数据源接入

**决策**：本 phase 不新建检索/问答内核，而是把「知识库」注册为 c04 §16.2 检索流程的一个数据源类别，并补齐知识库特有的入口与筛选：
- 全局搜索（§11.6）= 在 c04 检索流程前置「数据源选择 = 选定的一个或多个 kb_id」+ 多维筛选（文档类型/更新时间/来源/权限），检索方式（关键词 = BM25、语义 = 向量、混合 = 二者合并去重）直接复用 c04 的 BM25 + 向量 + 合并去重 + rerank。
- 知识库问答（§11.7）= 用户选 1..N 库 → 复用 c04：检索 chunk → rerank → source compression → 生成答案 → 标注引用（写/读 `citations`）→ 展示来源文档。
- 溯源（§11.8）= 复用 c04 citations + §16.3 chunk 元数据，定位到 知识库/源文档/章节(section)/页码(page)/段落(paragraph_index)/chunk/原文片段(chunk_text)；知识库链路只需保证导入时把这些元数据正确写入 chunk。

**备选与取舍**：
- 备选 A：知识库自建一套独立检索/问答。取舍：与 c04 §16.2 重复、溯源/引用结构分裂、维护两套 rerank，违反依赖纪律与简单优先，放弃。
- 选「数据源接入」：知识库只负责「把对的 chunk 带对的元数据和 ACL 喂进 c04 检索」，检索质量、引用、rerank 由 c04 统一保证，本 phase 聚焦管理/导入/权限。

### D6. 权限过滤落点：导入时写 ACL，检索时由 c04 强制过滤（不在应用层后置过滤）

**决策**：六维过滤（tenant_id/kb_id/user_id/role/document_acl/chunk_acl，§11.9）的落点分两处。其中 `document_acl` 与 `chunk_acl` 是两个**独立维度**（§11.9 并列），分别对应「文档级 ACL（落 `document_permissions`）」与「chunk 级 ACL（落 `document_chunks` 的 `chunk_acl` 列）」，本 phase 不在应用层各自维护这两维过滤，统一下推由 c04 rag-retrieval 执行。`chunk_acl` 的唯一物理列名与建表 owner 为 c03（`document_chunks.chunk_acl`，语义：chunk 级 ACL，默认继承来源文档 ACL，可写入比文档级更严的范围），c06 仅向该列**写值**、不改结构（与 c03 document-parsing、c04 rag-retrieval 同名同维，禁止再出现「单一 acl」与「chunk_acl」并存指同一列）：
- **写入侧**：导入入库时，把 `tenant_id`、`kb_id` 写到 `kb_documents`，把 `document_acl` 落 `document_permissions`，把 `chunk_acl` 覆盖值写入 c03 所建 `document_chunks.chunk_acl` 列（默认继承来源文档 ACL，可写严于文档级）作为检索过滤的物化条件。两维分别物化，不混为单一 acl。
- **读取侧（与 c04 的两维契约）**：检索时把当前 `user_id`/`role`/可见 `kb_id` 集合，以及**基于 `document_permissions` 预计算出的可见 `document_id` 集合**，作为过滤条件下推到 c04 §16.2 的「权限过滤」步骤。c04 rag-retrieval 已将单一 `acl` 扩展为可区分 `document_acl` 与 `chunk_acl` 两维过滤（对齐 §11.9 六维）：`document_acl` 维由 c06 注入的可见 document 集合或 document_permissions 条件执行、`chunk_acl` 维由 chunk 元数据条件执行。两维均在 BM25/向量召回阶段即过滤，而非召回后在应用层裁剪，保证未授权 document 与未授权 chunk 都不进入候选集、不被 rerank、不出现在引用里。终端用户可见 kb_id 集合 = 预设公共库 ∪ 被授权私有库（§11.4）。本 phase 知识库可见性只负责映射到这两维（或注入预计算的可见 document 集合），过滤执行归 c04。

**备选与取舍**：
- 备选 A：召回后在应用层按 ACL 后置过滤。取舍：未授权 chunk 仍进候选/参与 rerank/可能进 source compression 上下文，存在越权泄漏与配额浪费，违反红线「RAG 检索必须按 tenant/kb/user/role/acl 过滤」，放弃。
- 选「写入物化 + 召回前下推过滤」：过滤在最早阶段生效，复用 c04 已有权限过滤步骤，知识库只负责把 ACL 正确物化到 chunk，落点单一可审计。

### D7. 离线 / 私有化降级路径（公网不可用时）

**决策**：
- **PubMed/PMC 导入**：公网不可用时走 c03/c04 已有的「离线 PubMed 缓存」作为来源适配器后端，从离线缓存取结构化文献入库；公网恢复后可补全。
- **URL/白名单导入**：依赖公网抓取，公网不可用时该来源不可用，导入人改用「批量上传已下载的授权文件」路径完成入库（同样走 D3/D4 预览+授权闸门），保证闭环不中断。
- **解析/向量化/脱敏判定**：复用 c03 的私有化解析服务、私有化向量模型；脱敏判定消费 c09 redaction-gateway 接缝（c09 为 PHI/PII 识别脱敏唯一 owner，c01 不实现），redaction-gateway 未接入前公网保持关闭、仅走私有化/离线路径；按红线，脱敏识别失败或置信度不足时阻断公网模型调用并可切私有化模型，本 phase 导入侧遵循同一策略（识别不可用则降级到私有化或阻断入库公共库）。
- **检索/问答生成**：复用 c04/c03 的模型 Provider fallback（公网 → 私有化），知识库问答在无公网时由私有化模型生成带引用答案。问答生成调用公网模型前同样消费 c09 redaction-gateway 脱敏门禁（与 kb-import 导入侧门禁为两个独立执行点：导入入向量化前 vs 问答生成调公网模型前）：用户问题/检索注入上下文可能含 PHI/PII，公网模型调用前 MUST 先经 c09 识别脱敏，识别失败/置信度不足/服务不可用/门禁未接入时 MUST NOT 调用公网模型并切 c03 私有化模型，脱敏事件由 c09 在 c03 公网出口写 `privacy_redaction_events`，与 c09 security-compliance 把「c06 知识库问答」列为门禁前置消费方的口径一致。

**备选与取舍**：
- 备选：无公网即整条导入/问答不可用。取舍：违反「离线优先、任何核心闭环必须有离线降级」，放弃。
- 选「来源级 + 模型级双重降级」：来源不可用时切上传路径，模型/解析不可用时切私有化，确保 POC 在内网无公网环境仍可演示 13 库检索问答。

### D8. 数据模型：c06 新建 §18 命名的 knowledge_bases/kb_documents 基表，导入元数据承载在 kb_documents

**建表归属（跨 change 唯一 owner）**：`knowledge_bases` 与 `kb_documents` 是 PRD §18 命名但前序 phase（c01/c02/c03）均未建的基表；按数据表唯一建表 owner 约定，**这两张基表由本 phase（c06，知识库归属 phase、§24.3 验收方）新建**。c01 建 tenants/users/roles/permissions/documents/document_versions/document_permissions/document_events/recent_tasks/audit_logs；c03 建 document_chunks/embeddings 等解析向量表；c04 建 conversations/messages/citations 等会话引用表；c06 新建 knowledge_bases/kb_documents。下游若提及这两表一律为「消费/写入由 c06 所建表」。

**决策**（c06 新建基表，对齐 §18 表名，不新造同义表）：
- `knowledge_bases`（**c06 新建基表**）：13 库主表，字段含 `kb_id`（主键）、`tenant_id`、`name`、`description`、`created_by`、`is_seed`、`is_pinned`、`manual_weight`（可空）、`data_source`、`member_count`、`document_count`、`created_at`、`updated_at`。
- `kb_documents`（**c06 新建基表**）：知识库内文档 + §11.5.1 导入 10 必录字段（来源URL/文件来源、来源类型、导入人、导入时间、版权·授权状态、版本、解析状态、索引状态、`whitelist_rule_id`、`authorized_by`），并含 `tenant_id`/`kb_id` 外键与 `document_id` 关联到 c01 documents。
- `document_chunks` / `embeddings`：分块与向量（**c03 所建表**，c06 仅向 c03 所建 `document_chunks.chunk_acl` 列写入 chunk 级 ACL 覆盖值与 §16.3 元数据、不改结构；`chunk_acl` 物理列 owner=c03，c06 仅写值）。
- `citations`：问答引用溯源（**c04 所建表**，c06 仅写入/读取，不建表）。
- `conversations` / `messages`：知识库问答会话与消息（**c04 所建表**，c06 通过 c04 在 `conversations` 上提供的 `module`/`source` 两个独立维写入知识库问答会话以区分 AIMed：`module=kb_qa`（机器枚举值，c04 owner 唯一定义、取值域 {aimed, kb_qa}）、`source=「医疗知识库问答」`（§6.4 中文规范值），二者为不同字段不可混用同一字面量；c06 仅写入不建表；c06 据此为 §18「问答历史」中知识库会话语义的唯一写入方，c05 恢复编排按 `module=kb_qa` 区分并提供「继续追问」）。
- 权限与审计消费 `document_permissions`、`audit_logs`（**c01 所建表**）；脱敏判定关联 `privacy_detection_rules`、`privacy_redaction_events`（**c09 所建表**，c06 仅消费其判定接缝，详见 D6/D7）。
- 白名单规则：作为本 phase 新增配置表（来源白名单表/配置），其规则 ID 被 `kb_documents.whitelist_rule_id` 引用；属导入治理所需的最小新建结构。

**备选与取舍**：
- 备选：把 knowledge_bases/kb_documents 当作「上游 phase 已建表」只做补字段。取舍：c01/c02/c03 建表清单均不含这两张表，若 c06 仅补字段则二者成无 owner 的孤儿表、13 库 seed 与导入管线无基表可落，放弃；改为 c06 显式新建。
- 备选：新建 `kb_import_records` 独立导入记录表。取舍：与 `kb_documents` 一对一冗余、§18 未命名此表、检索需 join，放弃；把导入元数据直接挂 `kb_documents`，与「入库资料必须记录」语义一致。
- 白名单单独建表而非塞进既有表：白名单是来源治理规则（与具体文档无关的复用配置），独立成表才能被多次导入引用并按租户配置，属最小必要新增。

## Risks / Trade-offs

- **未授权商业数据库误入公共库（红线）** → D3 staging 物理隔离 + D4 三态状态机，未授权/不明来源在存储层即无法进入正式检索索引；`rejected` 来源直接阻断抓取，`preview_only` 仅临时预览；白名单规则 ID 与授权确认人全程留痕，§24.3 负向路径（被阻止 / 仅临时预览 / 普通用户不能写公共库）逐条可验。
- **物化计数（成员/文档数）与真实状态不一致** → 计数随导入/索引完成/成员变更的事务内增量更新；提供「重建索引」时一并校正计数（§11.5 管理员能力含重建索引），列表页只读缓存值不实时聚合。
- **越权检索泄漏** → D6 在召回前下推权限过滤（非应用层后置裁剪），未授权 chunk 不进候选/不参与 rerank/不进引用；终端用户可见 kb 集合严格 = 预设库 ∪ 授权私有库。
- **脱敏识别服务不可用导致入库带 PHI/PII** → 复用 c03/c09 脱敏判定，识别失败或置信度不足时按平台策略阻断入正式公共库或降级私有化处理；判定结果写 `privacy_redaction_events` 留痕。
- **依赖 c03/c04 接口未冻结** → 本 phase 以「数据源接入 + 元数据契约」方式依赖（chunk 元数据对齐 §16.3、检索对齐 §16.2、引用对齐 citations），契约字段稳定即可独立推进；接口细节差异在 Open Questions 跟踪。
- **演示库资料版权** → Demo 数据集仅用授权/开放来源（开放获取文献、自有制度模板等），演示库资料同样过 D4 授权状态机并标 `authorized` 来源，避免 POC 演示自身违反红线。
- **离线环境 URL/白名单导入不可用** → D7 提供「上传已下载授权文件」替代路径，闭环不中断；接受「无公网时实时 URL 抓取能力降级」这一权衡。

## Migration Plan

1. **建表**：本 phase 新建 §18 命名的 `knowledge_bases` 基表（含卡片物化字段）与 `kb_documents` 基表（含导入 10 必录字段 + tenant_id/kb_id 外键）；新增最小「来源白名单」配置结构。c06 是这两张基表的唯一建表 owner（c01/c02/c03 不建）。→ 验证：迁移可正向执行，knowledge_bases/kb_documents 表创建成功且字段齐备，不与 c01/c03/c04 所建表重名冲突。
2. **种子 13 库**：执行 seed 写入 13 个 `is_seed` 知识库（空库就绪 + 演示库资料经 D3/D4 管线入库）。→ 验证：首页展示 13 卡片、清单与 §11.2 逐项一致、可跨库检索/问答/溯源（§24.3）。
3. **导入管线接入**：上线四段式管线与三态授权闸门，复用 c03 解析/向量化、脱敏判定接口。→ 验证：未授权 URL 被阻止或仅进临时预览、普通用户上传不入公共库（§24.3 负向用例通过）。
4. **检索问答接入 c04**：把知识库注册为 c04 数据源，下推六维权限过滤。→ 验证：答案可溯源到段落、问答历史进入最近任务、越权用例无未授权内容泄漏。
5. **回滚策略**：迁移按可逆顺序拆分；新增列/白名单表可独立回退而不影响 §18 既有结构；种子库与演示资料可按 `is_seed`/导入批次清理；管线为新增路径，回退即停用导入入口，不影响 c03/c04 既有能力。

## Open Questions

- 知识库「成员人数」的成员关系建模归属：**已收口**。POC 本期 `member_count` 数据源 = 该知识库 ACL/`document_permissions` 授权用户去重计数（取该知识库范围内被授予读取/问答及以上权限的租户内可见用户数），不在 §18 新增 `kb_members` 表；独立的知识库成员关系表归 V1.1。该口径写入 knowledge-base spec「知识库卡片字段」Requirement 与 tasks 3.2，使 `member_count` 的 MUST 可被确定性验证。
- 私有库的可见性来源：**已收口**。终端用户「被授权私有库」集合 = 该用户在各知识库上持有读取/问答及以上 per-kb 知识库 ACL 授予记录（落 c01 `document_permissions` / 知识库 ACL 授予，与 D2 `member_count` 取知识库授权用户去重计数同源）的库集合 ∪ 预设公共库，本期不依赖独立 `kb_members` 表。「知识库管理员」身份口径同此 ACL 模型收口（见 knowledge-base / kb-import spec 角色锚定声明）：不是 c01 `roles` 表的新全局角色，而是在某具体知识库上持有管理级 per-kb ACL 授予的用户；其租户内全局角色仍取 c01 auth-rbac `roles` 表已登记角色（`admin`/`user`/`dept`/`doctor`/`reviewer`），「平台管理员」=`admin`；「读取/问答/上传导入/管理」四类为 PRD §19.1 per-kb 资源级 ACL 能力，区别于 c01 `permissions` 平台权限点，c06 MUST NOT 在 c01 `permissions` 表自造 `kb:*` 权限点。私有库成员关系完整管理后台与独立 `kb_members` 表归 V1.1（与 c03/c09 的 ACL 模型保持一致：本期统一以 `document_permissions` per-kb 授予表达，不引入平行 ACL 维度）。**注**：上述 per-kb 管理级 ACL 仅表达「对某已存在知识库的管理（排序/置顶/权限配置/重建索引/上传导入）」授权；「创建一个尚不存在的新知识库」因目标对象在创建时不存在、无从持有 per-kb 授予，其授权谓词另由 c01 auth-rbac 唯一登记的租户级 `kb:create` 权限点表达（默认授予 `admin`，c06 仅引用），两者为不同授权面，不可互相替代。
- 白名单规则的作用域：平台级全局 vs 按租户配置（POC 默认平台级，是否需要租户级覆盖待定）。
- 演示库初始资料的具体清单与体量（每库几篇）由 Demo 数据集 / 内置验收测试集（§22.1）统一规划，本设计只约束其必须经 D3/D4 管线且标 `authorized`。
- c04 检索接口接收「多 kb_id + 多维筛选 + 六维权限上下文」的具体参数契约，需在 c04 接口冻结后对齐（当前以 §16.2/§16.3 字段为契约假设）。其中 `document_acl` 与 `chunk_acl` 两维的读取侧执行方已明确为 c04 rag-retrieval（c04 已把单一 acl 扩展为两维过滤）：`document_acl` 维接受 c06 基于 `document_permissions` 预计算的可见 `document_id` 集合或等价条件、`chunk_acl` 维由 c04 按 c03 所建 `document_chunks.chunk_acl` 列执行；待 c04 冻结的仅为两维参数的字段名与传参形态（document 可见集合 vs 直接 document_permissions 条件），本 phase 8.6 的 document/chunk 级越权用例依赖该两维过滤已可成立。`chunk_acl` 物理列 owner 为 c03，c06 仅写值不建列。
