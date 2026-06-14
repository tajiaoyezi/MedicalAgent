## Context

MedOffice AI V1.0 POC 处理医学论文、患者个体信息与院内知识资产。前 8 个 phase（c01–c08）已分别交付门户与最近任务（c01）、ONLYOFFICE 集成与保存回调（c02）、模型 Provider 与文档解析（c03）、AIMed RAG 与引用溯源（c04）、AI 右侧面板与写回（c05）、知识库管理（c06）、医学翻译（c07）、模板中心（c08）。这些能力各自完成了功能闭环，但都各自触达「公网模型调用」与「操作文档」两类高敏感动作，而 PRD §19（安全与合规）、§20（可观测性与评测）、§21（性能）所要求的安全红线与可验收证据尚未被统一收口。

本 phase 是 9 阶段的收尾环，依赖 c01–c08 全部就绪，目标是把分散在各闭环里的安全约束与证据采集集中到一处横切层落地：

- **医疗安全边界**（PRD §19.2）：系统只能定位为科研 / 检索 / 知识查询 / 翻译 / 文书草稿 / 办公辅助，禁止自动诊断 / 处方 / 医嘱 / 替代医生决策；高风险内容必须进入医生（或授权审核角色）确认链路。
- **公网模型前置门禁**（PRD §19.4）：任何可能含 PHI / PII 的内容在调用公网模型前必须先经识别与脱敏；识别失败 / 脱敏置信度不足 / 识别服务不可用时禁止调用公网模型，可降级私有化模型。
- **统一审计与可观测性**（PRD §19.1 / §20）：上传 / 下载 / AI 调用 / 写回 / 删除、白名单放行、模型 fallback 全量进入审计日志；并采集 Metrics 与 Evals。
- **验收证据体系**（PRD §20.4 / §24）：交付内置 Demo 数据集与验收测试集，使全部 P0 需求可在个人机或内网环境被真实演示与验收，且禁用公网模型时主验收闭环仍可完成。

**约束与边界（PRD 第 22 章为准）**：仅覆盖 §22.1 P0 必做；高级合规审计、文档水印、电子签章（§22.3 V1.2+）与完整 Agent Evals 平台（§22.2 V1.1）不在本期；数字员工创建 / 运行 / 编排 / 执行历史（§24.8）不生成 task。本 phase 不引入新的业务能力，只在既有闭环上挂接安全约束与证据采集；数据模型复用 PRD §18 核心表命名，不新造平行表。

**Stakeholders**：医生 / 授权审核角色（高风险确认人）、普通用户（只能查看提示 / 生成草稿 / 提交审核）、租户管理员（脱敏策略与白名单放行）、验收人（执行验收测试集）。

## Goals / Non-Goals

**Goals:**

- 在 c01–c08 已有的租户隔离 / RBAC / 文档级与知识库级 ACL 之上，统一收口分享链接与下载权限、加密存储、访问与操作审计，使五类高敏动作（上传 / 下载 / AI 调用 / 写回 / 删除）全部落 `audit_logs`。租户隔离 + RBAC + 文档级 ACL 基线的唯一真值在 c01 `auth-rbac` / `document-center`，RAG 召回前权限过滤契约（六维含 document_acl/chunk_acl）唯一真值在 c04 `rag-retrieval`，知识库多库可见性 / 导入权限唯一真值在 c06 `kb-search-qa` / `kb-import`；本 phase 不以新 ADDED Requirement 平行重定义上述同名行为，仅做横切验收收口与统一审计。
- 建立**单一脱敏门禁执行点**：所有公网模型调用（c03 路由出口、c04 AIMed、c05 AI 面板写回、c06 知识库问答、c07 翻译、URL 导入预览）在出网前强制经过同一个 PHI / PII 识别与策略判断闸门，门禁不通过即阻断。
- 建立**单一高风险确认链路**：诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息类内容写回或下发前进入医生确认链路，并产出含 PRD §19.2 全部字段的 confirmation 记录。
- 所有医学回答与生成文档统一展示 PRD §19.3 免责声明，AI 生成内容标记为草稿 / 辅助建议。
- 交付内置 Demo 数据集与验收测试集（PRD §20.4 清单与用例字段），定义 Evals 阈值（§20.3）、Metrics（§20.2）、性能门槛（§21），每个验收用例关联到对应 P0 需求；禁用公网模型时主闭环可经私有化模型或离线 PubMed 缓存完成。

**Non-Goals:**

- 不重写 c01–c08 的业务逻辑，仅在其调用边界上挂接安全切面与证据钩子。
- 不实现高级合规审计、文档水印、电子签章（§22.3 V1.2+）。
- 不实现完整 Agent Evals 平台 / 数字员工执行历史（§22.2 / §24.8）；本期仅内置静态验收测试集与 Demo 数据集 + 一个轻量 Evals 跑批脚本。
- 不实现独立的脱敏 ML 训练；脱敏识别由规则 + 可插拔识别服务（公网 / 私有化）承担。
- 不承诺医院级商用密钥管理 / 国密合规；加密存储以 POC 可演示为准。

## Decisions

### D1 — 脱敏门禁作为「出网唯一闸门」横切中间件，而非各闭环各自实现

**决策**：将「PHI / PII 识别 + 脱敏 + 策略判断」实现为一个独立的 `redaction-gateway`（脱敏门禁服务 / 中间件），并把它放在 c03 模型 Provider 抽象层的**公网出口**之前。c04 / c05 / c06 / c07 以及 URL 导入预览的所有公网模型调用都经由 c03 路由出网，因此只需在该出口挂一个强制切面，即可覆盖全部出网路径，无需各闭环重复实现。

**归属与相位（横切契约）**：`redaction-gateway` 及 `privacy_detection_rules` / `privacy_redaction_events` 的唯一 owner 为本 change（c09）；c01 不实现 PHI / PII 识别脱敏，c03–c07 仅在公网出口预留「消费 c09 门禁」的接缝、自身不实现识别脱敏。`redaction-gateway` 是被 c03–c07 各更早 phase 前置消费的横切能力，但其完整实现落在收尾环 c09，因此存在「门禁需早于公网放开」的相位要求：在 `redaction-gateway` 接入前，凡 `deployment_kind=public` 的公网 provider 一律不可启用，本期主验收闭环默认**公网关闭、私有化 / 离线优先**，仅在私有化 / 离线路径上跑通闭环（符合 §16.4 / §24.9 禁用公网仍经私有化 / 离线闭环）；公网放开是接入门禁后的后续阶段动作。

**两个执行落点（§19.4 / §0.3 两个时点）**：PRD §19.4 与 §0.3「安全脱敏闭环」要求识别发生在「上传时」与「调用模型时」两个时点，对应「阻止上传」与「阻止调用模型」两类策略。`redaction-gateway` 同一识别能力（共用 `privacy_detection_rules`）因此有两个执行落点：(a) **出网闸**——c03 公网出网口，拦截「调用公网模型」，是脱敏留痕（`privacy_redaction_events` 四要素）的权威落点；(b) **上传闸**——文档中心（c01）/ AIMed 对话文件上传（c04）/ 知识库本地 / 批量上传（c06）/ 翻译文件上传（c07）四类入口，在内容**持久化入库或送模型前**执行识别并按「识别并提示 / 脱敏后送模型 / 阻止上传」策略处理，命中处理写 `privacy_redaction_events` 并回填 `audit_logs.id`，「阻止上传」时拒绝入库并写 `result=失败` 审计。两闸共用同一 owner 与同一规则源，但执行位点与拦截语义不同，共同闭合 §19.4 两类拦截点（避免仅出网闸导致「阻止上传」策略声明无实现）。门禁逻辑：

1. 调 PHI / PII 识别（覆盖 §19.4 范围：姓名 / 身份证号 / 手机号 / 住院号 / 门诊号 / 医保号 / 地址 / 检查号 / 影像号 + 可配置敏感词，规则源自 `privacy_detection_rules`）。
2. 按策略执行：识别并提示 / 脱敏后送模型 / 阻止调用 / 白名单放行（POC 默认「识别并提示 + 脱敏后送模型」，高安全部署「阻止公网、仅私有化」）。
3. **硬门禁**：识别失败、脱敏置信度不足、识别服务不可用三种情况下禁止调用公网模型；可切换私有化模型（走 c03 fallback）。
4. 每次出网写 `privacy_redaction_events`（是否脱敏 / 脱敏策略 / 命中敏感类型 / 置信度）并回填 `audit_logs.id`。

**备选方案与取舍**：
- *备选 A：每个闭环（c04/c05/c06/c07）各自在调用模型前自查脱敏*。否决——红线是「所有公网调用前必须脱敏」，分散实现必然出现遗漏路径与策略不一致，无法证明门禁完备，违反 §19.4「任何可能含 PHI / PII 的内容」。
- *备选 B：把门禁放在最外层 API 网关*。否决——网关层拿不到「本次调用目标是公网还是私有化模型」「内容是否将真正出网」的语义，会对私有化 / 离线路径误拦，且无法精确写命中类型。**选 c03 出口**因为它是公网 / 私有化路由的唯一收敛点，语义最完整，且天然能在 fallback 时复用同一条记录链路。

**降级路径**：公网识别服务不可用时，按 §19.4「公网识别服务不能作为含 PHI / PII 原文的唯一前置处理链路」，优先调私有化识别服务；私有化识别也不可用时，门禁判为「识别服务不可用」→ 禁止公网模型 → 切私有化模型（若私有化模型可用）或直接阻断并提示，全程留痕。识别服务配置在 `privacy_detection_rules` / 服务配置中提供公网与私有化两个入口。

### D2 — 高风险确认链路（引用式收口）：risk_type 分类器与 confirmation 表唯一 owner=c05，c09 仅消费/关联做统一验收与审计

**归属与边界（引用式收口，对齐 D1 redaction-gateway 的同级收口口径）**：高风险写回确认链路的两项核心构件——`risk_type` 风险分类器（含「诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息」判定标准与角色裁决）与 `writeback_confirmations` 确认记录表——的**唯一建表 / 判定 owner 为 c05 `ai-writeback-confirmation`**；c09 不平行重定义 `risk_type` 判定标准、不自建独立 confirmation 表，仅**消费 / 关联** c05 既有判定与 `writeback_confirmations` 记录做统一验收与审计收口（与 RBAC / ACL 的引用式横切收口同级）。

**决策**：在 c05 AI 写回（应用到文档 / 生成副本）、AIMed/翻译生成文书下发等动作前由 c05 的「风险分类 → 确认链路」拦截器把关。`risk_type` 命中诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息时，普通用户只能「查看提示 / 生成草稿 / 提交审核」，必须由具备**医生（`doctor`）或授权审核（`reviewer`）角色**（角色真值 owner=c01 `auth-rbac`，见 D2 角色口径）的确认人完成 `confirmation_action`。确认记录字段严格按 §19.2：`confirmation_id` / `document_id` / `message_id` / `confirmed_by` / `confirmed_role` / `confirmed_at` / `confirmed_scope` / `risk_type` / `before_content_hash` / `after_content_hash` / `confirmation_action` / `audit_log_id`。

高风险写回 confirmation 记录的唯一建表 owner 为 c05（`writeback_confirmations` 表，落 §19.2 全字段并通过 `audit_log_id` 回写 `audit_logs`），c09 仅消费 / 关联该表做统一验收与审计收口、不另建独立顶层表；写回沿用 c05 既有「原文 / 修改后 / 修改说明 / 影响范围」展示与「应用 / 生成副本 / 取消」三选项（PRD §1.3 原则 4 与 §24.7）。`before_content_hash` / `after_content_hash` 用于可回滚 + 防篡改校验（hash 算法与范围以 c05 写回实现为唯一真值）。

**确认链路覆盖 document 级与 message 级两类键（§19.2 `document_id` / `message_id`）**：高风险确认不止文档写回（`document_id` 为键），还覆盖**三类 message 级文书**——AIMed 答案（c04）/ 知识库问答 `kb_qa` 答案（c06）/ 医学翻译文书（c07）——在**下发前**的 message 级输出（`message_id` 为键）；三者均落 c04 所建 `conversations` / `messages` 表（kb_qa 以 `module=kb_qa` 标记），`writeback_confirmations` 既有 `document_id` 或 `message_id` 字段即支持二类键。message 级确认链路判定与记录的唯一 owner 仍为 c05，c09 仅引用消费做统一验收与审计收口（验收枚举覆盖上述三类生产方）：高风险 message 级输出下发前同样进 c05 确认链路、按 `confirmed_role`（∈ `{doctor, reviewer}`）裁决，普通用户只能提交审核 / 生成草稿、不能完成最终确认。

**角色口径（confirmed_role 取值可枚举）**：高风险确认人资格映射 §19.2「医生或授权审核角色」——RBAC 角色真值 owner 为 c01 `auth-rbac`，由 c01 在角色种子中提供 `doctor`（医生）/ `reviewer`（授权审核）角色或等价 `highrisk:confirm` 权限点；`confirmed_role` 取值枚举为 `{doctor, reviewer}`，c05 确认链路与 c09 验收均引用该确定角色名而非模糊「授权审核角色」。

**备选方案与取舍**：
- *备选 A：c09 自建独立 confirmation 表并自实现 risk_type 分类器*。否决——会与 c05 既有 `writeback_confirmations` 表 + 服务端 risk_type 判定形成双 owner / 双写、判定标准漂移，违反「同一安全红线单一真值」；与 D1 redaction-gateway 的单一 owner 收口口径自相矛盾。**选引用式收口**：判定与落库真值在 c05，c09 仅统一验收与审计关联。
- *备选 B：所有 AI 写回一律强制医生确认*。否决——非高风险内容（如普通办公文档润色）强制确认会破坏 POC 演示流畅性且无必要；按 `risk_type` 分级拦截更贴合 §19.2「高风险内容」的限定。**选「按风险分类的统一拦截器（owner=c05）」**，对低风险走原有快速写回，高风险才进确认链路，既守红线又不过度收紧。

### D3 — audit_logs 单一审计落点 + 横切埋点清单（access-log 切面）

**决策**：`audit_logs` 作为全系统唯一审计落点，由一个 access/audit 切面在五类动作处统一写入：上传（c01/c03 解析入口）、下载（c01 文档中心 / 分享链接）、AI 调用（c03 出口，含脱敏门禁结果）、写回（c05 确认链路结果）、删除。`audit_logs` 的建表 owner 为 c01-foundation，且**建表即含** `role` / `result`（成功 / 失败）/ `failure_reason` 列（角色真值经 c01 `auth-rbac` 提供）——c09 仅消费 / 写入这些既有列、不 ALTER 该表；c09 验收对每条审计记录核对 `tenant_id` / `user_id` / `role` / 操作类型 / 目标资源 ID / 时间戳 / `result`，失败操作（越权 / 被门禁阻断）写 `result=失败` 并填 `failure_reason`。`privacy_redaction_events` 专记脱敏 / 命中留痕并外键回 `audit_logs`；模型 fallback 四要素（provider / 失败原因 / 切换目标 / 时间戳）由 owner c03 写入 `audit_logs`（对齐 §24.9），c09 仅消费 / 验收溯源，不另落 `model_routes` / `provider_health_checks`（`model_routes` 仅承载绑定与优先级、`provider_health_checks` 仅承载连通性 / 健康结果，均无「切换目标 / audit_log_id」列）。`document_events` 仅承载 c01 定义的闭合 6 类 `event_type`（`upload_success` / `save_new_version` / `ai_writeback` / `translation_done` / `template_created` / `manual_reindex`，每类各有唯一产生方）；下载 / 分享 / 删除等访问 / 删除类动作 MUST NOT 写 `document_events`，一律落 `audit_logs`，`document_permissions` 承载分享与下载 ACL。日志维度对齐 §20.1（用户操作 / 文档编辑 / AI 写回 / Agent 运行日志 / 工具调用 / RAG 检索 / 翻译任务 / 错误）；其中 Agent 运行日志落 `agent_runs` / `agent_steps`，限 V1.0 内部 AI 任务追踪（AIMed / RAG / 翻译 / 文档 AI），不含数字员工执行历史。

**备选方案与取舍**：
- *备选 A：各服务写各自的日志表，验收时再聚合*。否决——验收要求「上传 / 下载 / AI 调用 / 写回 / 删除进入审计日志」可被单点核对（§24.7），多表聚合在 POC 阶段难以证明完备且易漏。**选单一 `audit_logs` + 专用 `privacy_redaction_events`**：既满足「统一可追溯」，又让脱敏命中这种高频细节不污染主审计表。

### D4 — 安全约束在 c01–c08 各闭环的挂接方式：切面 / 拦截器，零业务改写

**决策**：不修改各闭环业务实现，按下表把安全与证据钩子挂到调用边界：

| 闭环 | 挂接点 | 安全 / 证据动作 |
|---|---|---|
| c01 门户 / 文档中心 / 最近任务 | 上传 / 下载 / 分享 / 删除入口 | 下载 / 分享 / 删除入口：ACL 校验 + `audit_logs`（访问 / 删除类动作，MUST NOT 写 `document_events`）；上传入口：ACL 校验 + D1 **上传闸** PHI / PII 识别（持久化入库前，命中按「识别并提示 / 脱敏 / 阻止上传」处理）+ `audit_logs` + 一条 `event_type=upload_success` 的 `document_events`（唯一合法文档事件） |
| c02 ONLYOFFICE 保存回调 | 写回保存回调 | 写回经 D2 确认链路；保存回调成功率进 Metrics |
| c03 模型 Provider 路由 | 公网出网口 + fallback | D1 脱敏门禁 + `privacy_redaction_events` + fallback 留痕 |
| c04 AIMed RAG | 文件上传 + 检索过滤 + 出网 | 文件上传入口经 D1 **上传闸** PHI / PII 识别（送模型 / 向量化前）；RAG 召回前按 §11.9 六维 tenant_id/kb_id/user_id/role/document_acl/chunk_acl 过滤（维度与下推语义唯一真值在 c04 `rag-retrieval`，本 phase 仅统一验收与审计）；引用溯源 Evals 阈值以 PRD §20.3 与 c04 `citation-tracing` 为唯一真值；免责声明 |
| c05 AI 面板写回 | 应用 / 生成副本前 | D2 确认链路 + before/after hash |
| c06 知识库导入 / 检索 | 本地 / 批量上传 + URL 导入 + 检索 + 问答 | 本地 / 批量上传入口经 D1 **上传闸** PHI / PII 识别（持久化入 `kb_documents` 前，命中按「识别并提示 / 脱敏 / 阻止上传」处理）；未授权 URL 不默认抓取（白名单 / 管理员授权，授权不明确仅临时预览）；检索 ACL 过滤；高风险知识库问答（`kb_qa`）答案 message 级下发前经 c05 确认链路（D2） |
| c07 医学翻译 | 翻译文件上传 + 翻译出网 + 下发 | 翻译文件上传入口经 D1 **上传闸** PHI / PII 识别（送模型前）；翻译出网经 D1 **出网闸**门禁；高风险译文 message 级下发前经 c05 确认链路（D2）；术语一致性 / 版式保留 Evals |
| c08 模板中心 | 使用模板生成 | 生成文书走免责声明与（必要时）确认链路 |

**取舍**：以切面 / 拦截器挂接而非逐闭环改写，符合「Surgical Changes」原则，把安全关注点集中，降低对 c01–c08 的回归风险；代价是各闭环必须把公网调用统一经 c03 路由、把写回统一经 c05/确认入口，这一约束在前序 phase 已成立，故可行。

### D5 — Demo 数据集与验收测试集：以 PRD §20.4 清单为唯一数据源，用例—需求双向关联

**决策**：交付一套内置数据集与用例集，组织为「Demo 数据 + 验收用例 + 跑批工具」三层：

- **Demo 数据集**：严格按 §20.4 清单装载（医学论文 PDF 2–3 / DOCX 1、扫描 PDF 1、PPTX 1、XLSX 1、图片 1、双栏论文 PDF 1、表格 / 公式 / 参考文献翻译样例各 1、覆盖 docx/pptx/pdf/扫描 PDF 的参考译文、术语期望表、双语对照样例各 1、每默认知识库 ≥1 文档、1 术语库、1 语料库、覆盖六大模式的 PubMed 离线缓存）。其中 **§20.4 的「200 个真实可用模板」由 c08-template-center 作为资产 owner（`templates` / `template_categories` 表，含幂等导入脚本）交付，c09 仅引用纳入 Demo 验收清单、不另行重复装载**。
- **验收用例集**：覆盖 §20.4 全部用例类型（主闭环、AIMed 六大模式、翻译、知识库问答、模板创建、保存回调、最近任务恢复、模型连通性、文档视觉解析、安全合规 / 脱敏、公网禁用、未授权 URL 负向）。每条用例落 `eval_cases`，含 §20.4 字段：用例 ID / 前置数据 / 操作步骤 / 期望结果 / 通过证据 / 失败判定 / **关联 P0 需求**；结果落 `eval_results`。
- **关联 P0 需求**：每条用例显式回指 §22.1 P0 条目，形成「P0 需求 ↔ 验收用例」双向可追溯矩阵，确保 P0 无遗漏覆盖。

**备选方案与取舍**：
- *备选 A：复用线上真实院内数据做验收*。否决——含真实 PHI，违反隐私红线且不可随产物分发。**选内置可分发的 Demo 数据**（脱敏 / 合成样例），保证可在个人机或内网离线演示。
- *备选 B：用例只写文档不进库*。否决——验收要求「通过证据」可被核对、可被跑批；落 `eval_cases` / `eval_results` 才能与 Metrics / Evals 阈值联动。**选「用例落库 + 轻量跑批」**，避免引入 §22.2 的重型 Agent Evals 平台。

### D6 — Evals 与 Metrics 采集：阈值即验收门，性能门槛独立校验

**决策**：Evals 指标与阈值直接采用 §20.3 数值（模式识别 ≥85%、PubMed RAG Hit@5 ≥80%、引用可点击率 ≥95%、引用源定位 ≥90%、上传解析 ≥95%、保存回调 ≥99%、翻译任务 ≥95%、最近任务恢复 ≥98%、术语一致性 ≥95%、版式保留 ≥90%、视觉解析页码定位 ≥90% / 表格结构 ≥85%、引用页码误差 ≤1 页）。Metrics 按 §20.2 采集首 token 延迟 / RAG 检索延迟 / 各成功率 / AI 任务完成率，性能门槛按 §21（门户 ≤2s、AIMed 首 token ≤5s、知识库搜索 ≤3s、文档打开 ≤5s、保存回调 ≤10s 等）独立校验。跑批工具在 Demo 数据集上跑 `eval_cases`，把命中率 / 延迟写 `eval_results`，与阈值比对产出「通过 / 失败判定」。

**降级取舍**：禁用公网模型场景下（§24.9 / 公网禁用用例），同一套 Evals 经私有化模型或离线 PubMed 缓存重跑，主闭环指标仍须达阈值——这是验证「离线优先」红线的关键证据，故公网禁用是一条必跑的独立验收路径，而非可选项。

**脱敏置信度默认阈值**：D1 硬门禁中「脱敏置信度不足」的判定采用 POC 固化默认值 0.9（见 Open Questions），装载 `privacy_detection_rules` 时写入；「脱敏置信度不足→禁止公网」负向验收用例以该确定阈值作为失败判定基线，实测置信度与所用阈值写 `eval_results`。

### D7 — 数据表建表 owner 与延期表注记（§18）

**决策**：本 change（c09）为以下 §18 表的唯一**建表 owner**：`privacy_detection_rules`、`privacy_redaction_events`、`eval_cases`、`eval_results`（对齐脱敏门禁留痕与验收证据体系）。其余被本 phase 消费 / 写入的 §18 表均由各自 owner 建表，c09 仅消费 / 写入，不重复建表：

- `tenants` / `users` / `roles` / `permissions` / `documents` / `document_versions` / `document_permissions` / `document_events` / `recent_tasks` / `audit_logs` 由 c01-foundation 建表；
- `model_providers` / `model_routes` / `provider_health_checks` / `visual_parse_providers` / `document_chunks` / `embeddings` 由 c03-model-and-parse 建表；
- `conversations` / `messages` / `citations` / `agent_runs` / `agent_steps` / `tool_calls` / `feedbacks` 由 c04-aimed-rag-citation 建表（`feedbacks` 由 c04 泛化为可承载多来源反馈：`subject_type` ∈ `{message, translation_job}` / `subject_id` / `rating` / `reason`，c09 不写入 / 不消费该表）；
- `writeback_confirmations`（高风险写回确认记录，落 §19.2 全字段）由 c05-ai-panel-recent-tasks 建表，c09 仅消费 / 关联做统一验收与审计收口；
- `knowledge_bases` / `kb_documents` 由 c06-knowledge-admin 建表；
- `translation_jobs` / `translation_segments` / `term_bases` / `terms` / `corpora` 由 c07-medical-translation 建表；
- `templates` / `template_categories`（含 §20.4 的 200 个真实可用模板资产）由 c08-template-center 建表与导入，c09 仅引用纳入 Demo 验收清单、不重复装载。

本 phase 涉及的高风险 confirmation 确认记录唯一建表 owner 为 c05（`writeback_confirmations` 表，见 D2），c09 不新造独立顶层表，仅消费 / 关联该表并通过 `audit_log_id` 回写 `audit_logs` 做验收与审计收口。

**延期表注记**：`digital_agents` / `workflow_definitions` / `workflow_runs` / `agent_checkpoints` 为 §18 中数字员工路线图与长任务断点续跑（V1.1 / V1.2）预留，本 V1.0 POC **不建表、不生成 task**（PRD §18 说明明确数字员工相关表不生成 V1.0 POC 实施 task，§24.8 数字员工创建 / 运行 / 编排 / 执行历史亦延期；`agent_checkpoints` 与 c04 tasks 0.6 / design 第 40 行口径一致，仅为 V1.1 长任务断点续跑预留、本期不建）。`agent_runs` / `agent_steps` / `tool_calls` 虽与 Agent 追踪相关，但 PRD §18 说明界定其用于 V1.0 内部 AI 任务追踪（AIMed / RAG / 翻译 / 文档 AI），属本期范围，由 c04 建表、c09 消费（Agent 运行日志）。

## Risks / Trade-offs

- **[脱敏漏检导致 PHI 出网]** PHI / PII 规则识别存在漏检（如非标准格式住院号、嵌在影像描述里的姓名）。→ 缓解：D1 门禁对「识别置信度不足」即按硬门禁处理（禁止公网、切私有化）；`privacy_detection_rules` 支持可配置敏感词扩充；高安全部署默认走「阻止公网、仅私有化」策略，从源头不出网。
- **[门禁成为单点 / 增加延迟]** 所有公网调用串行经脱敏门禁，可能拉高 AIMed 首 token 延迟（§21 ≤5s）。→ 缓解：识别服务支持私有化本地部署降低 RTT；门禁结果可对同一会话内容缓存；超时即判「服务不可用」走降级而非阻塞。
- **[确认链路被绕过]** 若某闭环新增了不经 c05 / 确认入口的写回路径，则 D2 失效。→ 缓解：写回统一收敛到确认拦截器，并由「安全合规 / 脱敏」「公网禁用」验收用例 + 审计核对兜底；before/after hash 提供事后可追溯证据。
- **[审计落点不全]** 五类动作若有遗漏埋点，则审计不完备（§24.7）。→ 缓解：以单一 `audit_logs` + 切面统一埋点，并设计「审计完备性」验收用例，对每类动作核对是否生成审计记录。
- **[Demo 数据集合规性]** 演示数据若含真实 PHI 会自相矛盾。→ 缓解：Demo 数据全部用合成 / 脱敏样例，且其本身作为「脱敏测试」的正向输入。
- **[Evals 阈值在私有化模型下达不到]** 私有化小模型可能拉低模式识别 / 翻译一致性等指标。→ 缓解：公网与私有化两条路径分别记录 `eval_results`；私有化路径以「主闭环可完成 + 关键成功率（解析 / 保存回调 / 恢复）达标」为底线门，质量类指标允许在私有化路径标注差异，避免把 POC 验收卡死。
- **[范围蔓延]** 安全话题易牵出水印 / 签章 / 高级审计。→ 缓解：严格以 §22 第 22 章为界，越界项一律记入 Open Questions / backlog，不在本期实现。

## Migration Plan

本 phase 为收尾横切，不引入独立可回滚的数据迁移，部署按「先证据、再收紧」分步上线，保证对 c01–c08 既有路径可灰度：

1. **数据与配置就绪** → 验证：`privacy_detection_rules` 装载识别规则与可配置敏感词；公网 / 私有化两套识别服务配置入口可连通；Demo 数据集装载完成且清单齐全（§20.4）。
2. **审计与可观测性先行（只观测不拦截）** → 验证：五类动作均写 `audit_logs`、脱敏命中写 `privacy_redaction_events`、模型 fallback 四要素（provider / 失败原因 / 切换目标 / 时间戳）由 c03 写 `audit_logs`（c09 仅消费 / 验收）；Metrics 按 §20.2 上报。此阶段门禁为「识别并提示」软模式，便于观察漏检与误拦。
3. **开启脱敏硬门禁（D1）** → 验证：构造含 PHI 内容触发公网调用，确认识别失败 / 置信不足 / 服务不可用三种情况均被阻断并可切私有化（公网禁用用例通过）。
4. **收口高风险确认链路（D2，引用式）** → 验证：c05 既有确认链路对高风险写回拦住普通用户、`doctor` / `reviewer` 角色确认产出含 §19.2 全字段的 `writeback_confirmations`（owner=c05）记录并回写 `audit_logs`；c09 侧校验记录字段完整、可经 `audit_log_id` 关联审计，不自建表不重判 `risk_type`。
5. **挂接免责声明（§19.3）** → 验证：所有医学回答与生成文档均展示免责声明，AI 内容标记草稿 / 辅助建议。
6. **运行验收测试集（D5/D6）** → 验证：`eval_cases` 全部跑批，`eval_results` 对照 §20.3 阈值与 §21 性能门槛；公网禁用路径单独重跑主闭环；P0 需求—用例矩阵无遗漏。

**Rollback**：门禁与确认链路以特性开关控制。若硬门禁导致主闭环不可用，可回退到第 2 步软模式（仍留全量审计与命中记录），不丢失证据，不需回滚业务数据；确认链路同理可临时降级为「仅提示不强制」并记录降级原因进审计。

## Open Questions

- **脱敏置信度阈值取值**：「脱敏置信度不足」的具体阈值 PRD 未给定。为使「脱敏置信度不足→禁止公网」负向用例具备确定的失败判定基线，**POC 固化默认阈值 = 0.9**（置信度 ≥0.9 方可放行公网），在装载 `privacy_detection_rules` 时写入该缺省值并由 D6 验收用例引用；管理员仍可按部署安全等级覆盖（tasks 1.2 的管理员可配置项保留）。此项已从「待定」收敛为可验收默认值，不再悬空。
- **私有化路径下质量类 Evals 的验收底线**：私有化小模型在模式识别 / 翻译一致性等指标可能不达 §20.3 公网阈值，私有化路径应以哪些「必达指标」作为硬门、哪些可标注差异，需在验收前与验收人确认。
- **before/after content hash 算法与范围**：hash 计算覆盖整篇还是仅变更选区、用何算法（影响可回滚与防篡改证据强度），其唯一真值 owner 为 c05 写回实现（`writeback_confirmations` 表 owner=c05）；c09 仅消费 / 校验该字段存在与可关联审计，算法与范围以 c05 实现为准，本项归口 c05、c09 侧不悬空。
- **高风险确认角色名（已收敛）**：§19.2「医生或授权审核角色」在 RBAC 中对应 `doctor` / `reviewer`（或权限点 `highrisk:confirm`），角色真值 owner=c01 `auth-rbac` 并入种子；`confirmed_role` 取值枚举为 `{doctor, reviewer}`，c05 确认链路与 c09 验收均引用该确定角色名。此项已从「待定」收敛、不再悬空。
- **未授权 URL「临时预览」的留存边界**：PRD 规定授权不明确只进临时预览、不写入正式公共知识库，但临时预览数据的留存时长 / 清理策略待定。
- **审计日志保留期与加密存储强度**：POC 阶段 `audit_logs` 保留时长与「数据加密存储」的具体强度（字段级 / 卷级）未在 §22.1 P0 明确，按演示可行取最小实现，正式部署再升级。
