## ADDED Requirements

### Requirement: 文档视觉解析服务抽象与实现透明
系统 SHALL 提供文档视觉解析服务，用于扫描 PDF、图片、复杂 PDF、图表、表格与版式结构抽取。视觉解析 MUST NOT 被限定为单一 OCR 能力，其底层实现（传统 OCR、多模态大模型、版面分析模型、表格识别模型、公网第三方 API 或私有化解析服务）MUST 对上层透明，上层 MUST 仅依赖统一的结构化输出契约。视觉解析 provider 配置 SHALL 持久化于 `visual_parse_providers`。

#### Scenario: 上层不感知底层实现
- **WHEN** 视觉解析底层实现由 OCR 切换为多模态大模型或第三方 API
- **THEN** 上层文本解析与调用方无需改动，仍按统一结构化输出契约消费结果

#### Scenario: 非 OCR 场景被支持
- **WHEN** 输入为含图表/表格/复杂版式的 PDF
- **THEN** 服务不仅返回文本，还返回表格结构、图片位置与版式信息，而非仅纯文本 OCR 结果

### Requirement: 公网与私有化双配置
文档视觉解析服务 SHALL 同时支持公网解析配置与私有化解析配置两个独立入口，二者可独立配置、独立启用与独立连通性验收。私有化部署 SHALL 允许仅配置私有化解析服务而不配置公网解析。

#### Scenario: 公网与私有化解析独立配置
- **WHEN** 管理员分别配置一个公网视觉解析 provider 与一个私有化视觉解析 provider
- **THEN** 系统在 `visual_parse_providers` 分别持久化两条配置，且可对二者各自触发连通性测试并独立启用

#### Scenario: 仅私有化解析的离线部署
- **WHEN** 部署环境无公网，仅配置私有化视觉解析 provider
- **THEN** 系统对扫描/复杂文档的视觉解析全部走私有化路径，不因缺少公网解析配置失败

### Requirement: 公网视觉解析前的脱敏门禁接缝
在将文档内容送往任意公网视觉解析服务之前，系统 MUST 先消费 **c09 `redaction-gateway`** 的 PHI/PII 识别与脱敏判定结果。PHI/PII 识别+脱敏引擎的唯一 owner 为 c09，c01 不实现该能力；本能力仅在公网解析出口预留门禁接缝并强制门控，不自行实现识别脱敏算法。当识别失败、置信度不足或识别服务不可用时，系统 MUST 禁止调用公网解析服务，并 SHALL 在存在私有化解析 provider 时切换私有化路径，否则拒绝解析。门禁与切换 MUST 写入 `audit_logs`。本期口径：在 `redaction-gateway`（c09，phase 9）接入前，公网解析 provider 默认按「识别服务不可用」处理而拒绝/降级，扫描/复杂文档主闭环经私有化/离线解析完成；公网解析经脱敏放行的端到端验收随 c09 落地完成。

#### Scenario: 脱敏通过后调用公网解析
- **WHEN** 待解析文档经 c09 `redaction-gateway` 的 PHI/PII 识别与脱敏成功且置信度达标，且配置了公网解析 provider
- **THEN** 系统以脱敏后的内容调用公网视觉解析服务并记录脱敏已通过的审计日志

#### Scenario: 识别失败时禁止公网解析并切私有化
- **WHEN** 公网解析前经 c09 `redaction-gateway` 的 PHI/PII 识别失败或置信度不足，且存在启用的私有化解析 provider
- **THEN** 系统禁止调用公网解析服务，改走私有化解析，并记录脱敏门禁触发与切换审计日志

#### Scenario: 识别服务不可用且无私有化解析时拒绝
- **WHEN** c09 `redaction-gateway` 识别服务不可用且无可用私有化解析 provider
- **THEN** 系统拒绝该文档的视觉解析并返回明确错误，绝不在未脱敏情况下送往公网解析

#### Scenario: 脱敏门禁未接入时公网解析默认拒绝（本期保守降级）
- **WHEN** c09 `redaction-gateway` 判定结果尚不可用（本期 phase 3 默认公网关闭、门禁未接入），上层请求经公网视觉解析处理某扫描件
- **THEN** 系统按「识别服务不可用」处理，禁止启用公网解析 provider，存在私有化解析 provider 时改走私有化、否则返回明确错误并落拒绝审计

### Requirement: 结构化输出契约
文档视觉解析服务 SHALL 对每个解析结果输出统一结构化内容，至少包含：文本内容、页码、段落、坐标、标题层级、表格结构、图片位置、页眉页脚、置信度、失败原因与 chunk 定位信息，并持久化于 `document_visual_parse_results`。该输出 MUST 可直接作为 `document-parsing` 的 chunk 切分输入，且 chunk 定位信息 MUST 支持后续引用回 chunk 与页码溯源。

#### Scenario: 输出完整结构化字段
- **WHEN** 一份复杂 PDF 完成视觉解析
- **THEN** 结果包含文本内容、页码、段落、坐标、标题层级、表格结构、图片位置、页眉页脚、置信度与 chunk 定位信息，并写入 `document_visual_parse_results`

#### Scenario: 结构化输出作为切分输入
- **WHEN** `document-parsing` 取用某扫描文档的视觉解析结果
- **THEN** 解析流水线以其页码/段落/标题层级/chunk 定位信息为依据完成 chunk 切分，保持页码与段落可溯源

#### Scenario: 解析失败返回失败原因
- **WHEN** 某图片清晰度过低导致视觉解析无法完成
- **THEN** 服务返回失败标志与具体 `failure_reason`，不输出空置信度的伪结果，并使上游解析作业转入失败处理

### Requirement: 表格与页码识别质量指标
文档视觉解析服务 SHALL 对解析结果给出置信度。页码定位成功率 SHALL ≥ 90%，表格结构识别成功率 SHALL ≥ 85%，且支撑下游引用源定位页码误差 ≤ 1 页。置信度低于阈值的结果 MUST 被标记，供下游决定是否进入人工复核。所引用的「内置文档视觉解析测试集」MUST 由 c09 提供（§20.4 内置验收测试集 / Demo 数据集，`eval_cases` owner）；c03 本期仅在该内置子集上自验指标计算逻辑与低置信度标记逻辑，§20.3 三项数值指标（页码定位 ≥ 90%、表格结构识别 ≥ 85%、引用源页码误差 ≤ 1 页）的最终达标判定随 c09 Evals 跑批（c09 tasks 10.2，phase 9）完成，c03 不在 phase 3 单独对该测试集做最终达标判定。

#### Scenario: 页码定位达标
- **WHEN** 对 c09（§20.4 内置验收测试集 / Demo 数据集，`eval_cases` owner）提供的内置文档视觉解析测试集执行解析
- **THEN** c03 在该内置子集上自验指标计算与低置信度标记逻辑，输出页码定位成功率、表格结构识别成功率与引用源页码误差三项指标；其与 §20.3 阈值（页码定位 ≥ 90%、表格结构识别 ≥ 85%、引用源页码误差 ≤ 1 页）的最终达标判定随 c09 Evals 跑批（c09 tasks 10.2，phase 9）完成

#### Scenario: 低置信度结果被标记
- **WHEN** 某页解析置信度低于设定阈值
- **THEN** 该结果被标记为低置信度，下游可据此提示人工复核而非直接当作可信溯源源

### Requirement: 视觉解析结果的租户隔离与可审计
`document_visual_parse_results` SHALL 继承来源文档的 `tenant_id` 与**文档级访问权限（由 c01 `document_permissions` 派生执行）**，按租户隔离存储与访问。该表 MUST NOT 引入独立 `chunk_acl` 物理列，亦 MUST NOT 作为 §11.9 六维 RAG 检索过滤维——其可见性由文档级权限派生，与 `document_chunks` 的 `document_acl`（由 `document_permissions` 派生）/`chunk_acl`（物理列）二维口径区隔，消除跨表权限维命名漂移。视觉解析的调用（公网/私有化路径选择、provider、成功/失败、置信度、失败原因）SHALL 写入 `audit_logs`，支持对解析过程的溯源与审计。

#### Scenario: 解析结果继承文档权限
- **WHEN** 一份受限文档完成视觉解析
- **THEN** 其 `document_visual_parse_results` 记录 `tenant_id` 与来源文档一致，可见性继承来源文档级权限（由 `document_permissions` 派生、无独立 `chunk_acl` 列），不对未授权范围开放

#### Scenario: 解析调用可审计
- **WHEN** 审计人员查询某文档的视觉解析过程
- **THEN** 系统返回该次解析所走路径（公网/私有化）、provider、置信度、成功或失败原因的审计记录，可完整溯源
