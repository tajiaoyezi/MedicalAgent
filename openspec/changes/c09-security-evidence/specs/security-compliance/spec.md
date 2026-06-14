## ADDED Requirements

### Requirement: 多租户隔离与 RBAC 权限控制
本要求为对 c01-foundation `auth-rbac`（RBAC 权限模型与租户隔离）既有租户隔离与角色权限行为的**横切统一约束与验收收口**，底层判定的唯一真值（权威定义）仍在 c01 `auth-rbac`；本 change 不平行重定义其判定逻辑，仅追加全系统范围的统一验收与越权审计留痕。在此前提下，系统 SHALL 在 c01 已实现的多租户数据隔离与基于角色的访问控制（RBAC）之上统一收口：所有业务数据 MUST 携带 `tenant_id`，任何查询与写入 MUST 强制按当前会话的 `tenant_id` 过滤，禁止跨租户读取或写入；系统 MUST 按用户角色（如管理员 / 医生 / 授权审核角色 / 普通用户）授予差异化操作权限，越权操作 MUST 被拒绝并记入审计日志。

#### Scenario: 跨租户访问被拒绝
- **WHEN** 租户 A 的用户携带租户 A 会话请求访问归属租户 B 的文档或知识库资源
- **THEN** 系统 MUST 拒绝访问并返回无权限错误，不泄露目标资源是否存在
- **AND** 该越权尝试 MUST 以 `tenant_id` / `user_id` / 目标资源 ID 写入审计日志

#### Scenario: 角色越权操作被拒绝
- **WHEN** 普通用户尝试执行仅管理员或授权审核角色可执行的操作（如修改全局策略、完成高风险确认）
- **THEN** 系统 MUST 拒绝该操作并提示权限不足
- **AND** 系统 MUST 不产生任何状态变更并记录一条越权审计

### Requirement: 文档级与知识库级 ACL 权限过滤
本要求为对既有 ACL 与召回前权限过滤行为的**横切统一约束与验收收口**：文档级 ACL 基线的唯一真值在 c01-foundation `document-center`（文档权限分级）；RAG 检索召回前的权限过滤契约（含 §11.9 六维 `tenant_id` / `kb_id` / `user_id` / `role` / `document_acl` / `chunk_acl`）唯一真值在 c04-aimed-rag-citation `rag-retrieval`；知识库多库可见性与导入权限唯一真值在 c06-knowledge-admin `kb-search-qa` / `kb-import`。本 change 不平行重定义上述判定，仅统一验收并对无权访问产生审计留痕。在此前提下，系统 SHALL 对文档与知识库分别实施访问控制列表（ACL）：文档与知识库的读取 / 编辑 / 分享 / 删除等操作 MUST 依据 `tenant_id` / `kb_id` / `user_id` / `role` / `document_acl` / `chunk_acl` 维度进行权限判定（具体维度与下推语义以 c04 `rag-retrieval` 为唯一真值）；RAG 检索、知识库问答与文档列举 MUST 在召回阶段即按 ACL 过滤，无权访问的内容 MUST NOT 出现在结果或引用中。

#### Scenario: 知识库检索按 ACL 过滤召回
- **WHEN** 用户对知识库发起检索问答，候选片段中包含其无 `acl` 权限的知识库或文档
- **THEN** 系统 MUST 在召回阶段过滤掉无权内容，仅返回该用户有权访问的片段与引用
- **AND** 被过滤内容 MUST NOT 在回答正文、引用列表或溯源定位中以任何形式暴露

#### Scenario: 无权文档访问被拒绝
- **WHEN** 用户请求打开或下载一个其 ACL 未授权的文档
- **THEN** 系统 MUST 拒绝并返回无权限错误
- **AND** 系统 MUST 记录一条包含 `document_id` / `user_id` / 操作类型的访问审计

### Requirement: 分享链接与下载权限控制
系统 SHALL 控制分享链接与下载行为：分享链接 MUST 绑定权限范围（可访问对象、可执行操作、有效期），失效或越权的分享链接 MUST 被拒绝；下载操作 MUST 校验用户对目标文档的下载权限，未授权下载 MUST 被阻断。

#### Scenario: 失效分享链接被拒绝
- **WHEN** 用户访问一个已过期或权限范围不匹配的分享链接
- **THEN** 系统 MUST 拒绝访问并提示链接无效或无权限
- **AND** 系统 MUST NOT 因该链接放宽底层文档的 ACL 判定

#### Scenario: 无下载权限被阻断
- **WHEN** 仅具备查看权限的用户尝试下载文档原文
- **THEN** 系统 MUST 阻断下载并提示无下载权限
- **AND** 该下载尝试 MUST 进入审计日志

### Requirement: 数据加密存储
本要求为对 c01-foundation `object-storage` 已建对象存储层的**横切安全收口**，并非在末阶段倒灌重写 c01 的落盘业务逻辑；本期取最小可演示加密强度（字段级 / 卷级强度参 design Open Questions），不替换 c01 既有 presigned URL / ACL / file_hash 能力。在此前提下，系统 SHALL 对存储的文档、知识库内容与敏感配置实施加密存储：对象存储（MinIO / S3）中的文件与数据库中的敏感字段 MUST 以加密形式持久化，密钥 MUST NOT 与密文同库明文保存；未经授权的存储层直接读取 MUST 无法获得明文内容。

#### Scenario: 存储层密文不可直接读取
- **WHEN** 在不经过应用授权链路的情况下直接读取对象存储或数据库中的受保护数据
- **THEN** 读取到的内容 MUST 为密文，无法还原为明文
- **AND** 加密密钥 MUST NOT 与密文存放于同一可被同时读取的位置

### Requirement: 访问日志与操作审计
`audit_logs` 的唯一建表 owner 为 c01-foundation，且建表即含 `role` / `result`（成功 / 失败）/ `failure_reason` 列（角色真值经 c01 `auth-rbac` 提供）；本 change（c09）仅消费 / 写入这些既有列、MUST NOT ALTER 该表结构。在此前提下，系统 SHALL 提供统一审计日志（`audit_logs`）：上传、下载、AI 调用、写回、删除等关键操作 MUST 全量进入审计日志，每条记录 MUST 至少包含 `tenant_id` / `user_id` / `role` / 操作类型 / 目标资源 ID / 时间戳 / `result`（成功或失败），失败记录 MUST 同时填写 `failure_reason`；审计记录 MUST 与高风险确认记录（`writeback_confirmations`，owner=c05）及模型 fallback 记录可关联。

#### Scenario: 关键操作进入审计日志
- **WHEN** 用户执行上传 / 下载 / AI 调用 / 写回 / 删除中的任意一项
- **THEN** 系统 MUST 生成一条审计记录，含 `tenant_id` / `user_id` / `role` / 操作类型 / 目标资源 ID / 时间戳 / `result`（成功或失败）
- **AND** 失败操作（如越权、被门禁阻断）同样 MUST 记录，且 `result=失败` 时 MUST 填写 `failure_reason`

#### Scenario: 审计记录可关联确认与 fallback
- **WHEN** 一次高风险写回经 `doctor` / `reviewer` 角色确认完成（记录落 c05 `writeback_confirmations`），或一次公网出网调用触发脱敏留痕 / 模型 fallback
- **THEN** 关联键统一落在确认 / 留痕 / fallback 侧、回指 `audit_logs.id`：c05 `writeback_confirmations.audit_log_id`（确认场景）、c09 `privacy_redaction_events.audit_log_id`（脱敏场景）、c03 写入 `audit_logs` 的 fallback 四要素（fallback 场景）MUST 可从确认 / 留痕 / 审计记录反查到对应 `audit_logs.id`；`audit_logs` MUST NOT 新增 `confirmation_id` 列、c09 MUST NOT ALTER `audit_logs`
- **AND** 通过该 `audit_log_id` 关联 MUST 能溯源到该次操作的确认人 / 脱敏命中 / 切换目标与失败原因

### Requirement: 医疗安全边界与系统定位约束
系统 SHALL 将自身定位约束为医学科研辅助、文献检索辅助、院内知识库查询、医学翻译辅助、临床文书草稿辅助、办公文档处理辅助；系统 MUST NOT 被定位或运行为自动诊断系统、自动处方系统、自动医嘱执行系统或替代医生决策系统；所有 AI 生成内容 MUST 默认标记为草稿 / 辅助建议，MUST NOT 直接作为最终诊疗、用药或医嘱结论被自动执行。

#### Scenario: AI 不输出自动诊断 / 处方 / 医嘱执行
- **WHEN** 用户请求 AI 直接给出确定性诊断、处方或自动执行医嘱
- **THEN** 系统 MUST 以草稿 / 辅助建议形式给出内容并提示需具备资质的医务人员确认
- **AND** 系统 MUST NOT 将该内容标记为可自动执行的诊断 / 处方 / 医嘱结论

#### Scenario: AI 生成内容标记为草稿 / 辅助建议
- **WHEN** AIMed、医学翻译或 AI 面板产出任意医学相关内容
- **THEN** 系统 MUST 在结果上标记为草稿 / 辅助建议
- **AND** 该标记 MUST 随内容写回文档与版本，确保后续可识别其辅助性质

### Requirement: 高风险内容进入医生确认链路并留存 confirmation 记录
本要求为对 c05-ai-panel-recent-tasks `ai-writeback-confirmation` 既有高风险写回确认链路的**横切统一约束与验收收口**：`risk_type` 风险分类器（含诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息判定标准与角色裁决）与 `writeback_confirmations` 确认记录表的唯一建表 / 判定 owner 为 c05；本 change（c09）不平行重定义 `risk_type` 判定、不自建独立 confirmation 表，仅消费 / 关联 c05 既有判定与记录做统一验收与审计收口（与 RBAC / ACL 的引用式横切收口同级）。确认人资格映射 §19.2「医生或授权审核角色」——RBAC 角色真值 owner 为 c01 `auth-rbac`，`confirmed_role` 取值枚举为 `{doctor, reviewer}`（即医生 / 授权审核角色，或等价 `highrisk:confirm` 权限点）。在此前提下，系统 SHALL 将涉及诊疗、用药、医嘱、临床文书与患者个体信息的高风险内容强制纳入人工确认链路：确认人 MUST 具备 `doctor`（医生）或 `reviewer`（授权审核）角色，普通用户只能查看提示、生成草稿或提交审核，MUST NOT 完成最终确认；每次确认 MUST 由 c05 留存 confirmation 记录（`writeback_confirmations`，owner=c05），字段至少包含 `confirmation_id` / `document_id` 或 `message_id` / `confirmed_by` / `confirmed_role` / `confirmed_at` / `confirmed_scope` / `risk_type` / `before_content_hash` / `after_content_hash` / `confirmation_action` / `audit_log_id`，且该记录 MUST 不可篡改并关联审计日志；c09 对该记录字段完整性与审计关联做验收收口。

#### Scenario: 高风险内容写回前进入确认链路
- **WHEN** AI 面板对高风险内容（诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息）发起写回文档
- **THEN** 系统 MUST 先展示「原文 / 修改后 / 修改说明 / 影响范围」并阻止直接写回，要求进入确认链路
- **AND** 仅当具备 `doctor` 或 `reviewer` 角色的确认人选择「应用到文档」后，写回方可执行

#### Scenario: 普通用户不能完成最终确认
- **WHEN** 普通用户对高风险内容尝试完成最终确认
- **THEN** 系统 MUST 拒绝其确认操作，仅允许其查看提示、生成草稿或提交审核
- **AND** 系统 MUST NOT 生成任何标记为已确认的 confirmation 记录

#### Scenario: confirmation 记录字段完整且关联审计
- **WHEN** 具备 `doctor` 或 `reviewer` 角色的确认人完成一次高风险内容确认
- **THEN** c05 MUST 在 `writeback_confirmations` 生成包含 `confirmation_id` / `document_id` 或 `message_id` / `confirmed_by` / `confirmed_role`（取值 ∈ `{doctor, reviewer}`）/ `confirmed_at` / `confirmed_scope` / `risk_type` / `before_content_hash` / `after_content_hash` / `confirmation_action` / `audit_log_id` 的确认记录
- **AND** c09 MUST 校验该记录字段完整、与对应审计日志关联，且 MUST 不可被事后篡改

### Requirement: 医疗免责声明展示
系统 SHALL 在所有医学回答与生成文档中统一展示 PRD §19.3 规定的医疗免责声明：声明文本 MUST 为「本内容由 AI 生成，仅供医学科研、文档处理和办公参考，不构成诊断、治疗、用药或医嘱建议。涉及患者诊疗和用药决策时，请由具备资质的医务人员最终确认。」；AIMed 回答、医学翻译输出、AI 面板结果与生成的在线 Word / PDF / Markdown MUST 均携带该声明，MUST NOT 因导出格式而丢失。

#### Scenario: AI 回答展示免责声明
- **WHEN** 用户从 AIMed、知识库问答或 AI 面板获得任意医学回答
- **THEN** 系统 MUST 在回答中展示 §19.3 规定的免责声明全文
- **AND** 声明文本内容 MUST 与 PRD §19.3 一致，不得删改

#### Scenario: 生成文档导出后仍保留免责声明
- **WHEN** 用户将带医学内容的回答生成在线 Word 或保存为 PDF / Markdown
- **THEN** 导出的文档 MUST 包含该医疗免责声明
- **AND** 声明 MUST NOT 因格式转换而缺失或被截断

### Requirement: 脱敏门禁 redaction-gateway 横切归属与相位约束
PHI / PII 识别 + 脱敏引擎 `redaction-gateway` 及其 `privacy_detection_rules` / `privacy_redaction_events` 表的唯一 owner 为本 change（c09）；`redaction-gateway` 是被 c03–c07 各 phase **前置消费**的横切能力，c01 不实现 PHI / PII 识别与脱敏，c03–c07 仅在公网出口预留消费该门禁的接缝，自身不实现 PHI / PII 识别脱敏。相位约束：`redaction-gateway` MUST 早于任何会触达公网模型的能力被放开；在 `redaction-gateway` 接入前，凡 `deployment_kind=public` 的公网 provider MUST 一律不可启用，本期主验收闭环 MUST 默认公网关闭、私有化 / 离线优先，仅在私有化 / 离线路径上跑通闭环。

#### Scenario: 脱敏门禁未接入前公网 provider 不可启用
- **WHEN** `redaction-gateway` 尚未接入或被禁用，而部署尝试启用 `deployment_kind=public` 的公网 provider
- **THEN** 系统 MUST 拒绝启用该公网 provider，并提示「脱敏门禁未接入，公网调用不可启用」
- **AND** 主验收闭环 MUST 仍可经私有化 / 离线路径完整跑通，不依赖任何公网模型调用

#### Scenario: c03–c07 公网出口经 c09 门禁前置消费
- **WHEN** c03 路由出口、c04 AIMed、c05 AI 面板写回、c06 知识库问答、c07 翻译中任一会触达公网模型的调用即将出网
- **THEN** 该调用 MUST 先经由本 change 提供的 `redaction-gateway` 完成识别与策略判断，c03–c07 自身 MUST NOT 各自实现 PHI / PII 识别脱敏
- **AND** 脱敏命中与策略留痕 MUST 由本 change（c09）的 `redaction-gateway` 在 c03 公网出口**单一写入** `privacy_redaction_events` 并回填 `audit_logs.id`，c03–c07 自身 MUST NOT 另行写入或并行维护该表字段口径

### Requirement: 隐私 PHI / PII 识别与脱敏
系统 SHALL 提供 PHI / PII 识别与脱敏能力，识别范围 MUST 至少覆盖姓名、身份证号、手机号、住院号 / 门诊号、医保号、地址、检查号 / 影像号及可配置敏感词；系统 MUST 支持「识别并提示」「脱敏后送模型」「阻止上传 / 阻止调用模型」「管理员白名单放行」四类处理策略，并 MUST 同时提供公网识别服务与私有化识别服务两套配置入口；患者数据 MUST NOT 进入模型训练；高安全部署 MUST 能将策略调整为仅允许私有化模型处理。POC 默认策略 SHALL 为「识别并提示 + 脱敏后送模型」。

#### Scenario: 命中敏感类型并按默认策略脱敏
- **WHEN** 待送模型的内容命中姓名 / 身份证号 / 手机号 / 住院号 / 医保号 / 地址 / 检查号 / 影像号或可配置敏感词
- **THEN** 系统 MUST 识别并提示命中的敏感类型，并在默认策略下对内容脱敏后再送模型
- **AND** 系统 MUST 记录命中的敏感类型与所用脱敏策略

#### Scenario: 阻止上传 / 阻止调用策略生效
- **WHEN** 管理员将策略配置为「阻止上传 / 阻止调用模型」且内容命中敏感信息
- **THEN** 系统 MUST 阻止该上传或模型调用并提示原因
- **AND** 系统 MUST NOT 将含敏感信息的原文外送

#### Scenario: 私有化高安全策略仅允许私有化模型
- **WHEN** 部署采用私有化高安全策略
- **THEN** 系统 MUST 禁止含 PHI / PII 内容调用公网模型，仅允许私有化模型处理
- **AND** 含 PHI / PII 原文 MUST NOT 以公网识别服务作为唯一前置处理链路，MUST 优先使用私有化识别服务

#### Scenario: 患者数据不进入模型训练
- **WHEN** 系统处理含患者个体信息的内容
- **THEN** 该数据 MUST NOT 被用于任何模型训练用途
- **AND** 系统 MUST 遵循文档访问最小权限原则并对 AI 输出留痕

### Requirement: 公网模型调用前置门禁与策略
本要求所定义的 PHI / PII 识别 + 脱敏引擎 `redaction-gateway` 及 `privacy_detection_rules` / `privacy_redaction_events` 留痕表，其唯一 owner 为本 change（c09）；该门禁是被 c03–c07 前置消费的横切能力，c03 模型 Provider 路由的**公网出网口**为该门禁的统一执行落点与脱敏留痕的**权威落点**。脱敏门禁的「是否经过脱敏 / 脱敏策略 / 命中的敏感类型 / `audit_log_id`」四要素 MUST 由本 change 统一写入 `privacy_redaction_events` 并外键回 `audit_logs`，c03 仅作为该留痕在公网出口的执行落点，上层 change（c04 / c05 / c06 / c07）复用同一条留痕链路、不另维护并列字段口径。系统 SHALL 在调用公网模型前强制执行敏感信息识别与策略判断门禁：任何可能包含 PHI / PII 的内容在调用公网模型前 MUST 先经识别与策略判断；当**敏感信息识别失败**、**脱敏置信度不足**或**识别服务不可用**时，系统 MUST 禁止调用公网模型（识别服务不可用时 MUST 允许切换私有化模型）；管理员白名单放行 MUST 记录原因、放行人、放行时间与适用范围；每次公网模型调用 MUST 记录是否经过脱敏、脱敏策略、命中的敏感类型与审计日志 ID。

#### Scenario: 识别失败时禁止调用公网模型
- **WHEN** 公网模型调用前的敏感信息识别失败
- **THEN** 系统 MUST 禁止本次公网模型调用并提示门禁拦截原因
- **AND** 系统 MUST 记录一条被拦截的审计，且 MUST NOT 外送原文

#### Scenario: 脱敏置信度不足时禁止调用公网模型
- **WHEN** 识别完成但脱敏置信度低于阈值（POC 默认阈值为 0.9，装载 `privacy_detection_rules` 时写入，管理员可按部署安全等级覆盖）
- **THEN** 系统 MUST 禁止调用公网模型
- **AND** 系统 MUST 提示置信度不足并记录命中的敏感类型与拦截结果

#### Scenario: 识别服务不可用时禁用公网并可切私有化
- **WHEN** 敏感信息识别服务不可用
- **THEN** 系统 MUST 禁止调用公网模型，并 MUST 允许切换到私有化模型完成处理
- **AND** 该禁用与切换 MUST 进入审计日志，记录失败原因与切换目标

#### Scenario: 白名单放行留痕
- **WHEN** 管理员对某次受门禁拦截的调用执行白名单放行
- **THEN** 系统 MUST 记录放行原因、放行人、放行时间与适用范围
- **AND** 放行后的公网调用 MUST 仍记录是否脱敏 / 脱敏策略 / 命中敏感类型 / 审计日志 ID

#### Scenario: 公网调用脱敏与命中类型留痕
- **WHEN** 一次内容通过门禁后调用公网模型
- **THEN** 系统 MUST 记录是否经过脱敏、所用脱敏策略、命中的敏感类型与对应审计日志 ID
- **AND** 该记录 MUST 可经审计日志溯源到本次公网调用

### Requirement: 上传时 PHI / PII 识别与「阻止上传」策略执行
PRD §19.4 与 §0.3「安全脱敏闭环」明确 PHI / PII 识别发生在**两个时点**：**上传时**（对应「阻止上传」策略）与**调用模型时**（对应「阻止调用模型」策略）；本要求落地前者，与「公网模型调用前置门禁与策略」（出网闸）共同闭合 §19.4 两类拦截点。`redaction-gateway` 及其识别能力的唯一 owner 为本 change（c09），c09 区分两个执行落点——**出网闸**（c03 公网出网口，拦截「调用公网模型」）与**上传闸**（上传入口，拦截「上传」）。系统 SHALL 在**全部四类上传入口**——文档中心（c01）/ AIMed 对话文件上传（c04 §8.6）/ 知识库本地 / 批量上传（c06 §11.5.1）/ 医学翻译文件上传（c07）——在内容**持久化入库或送模型前** MUST 经 c09 `redaction-gateway` 复用 `privacy_detection_rules` 完成 PHI / PII 识别，并按策略处理：默认「识别并提示」（提示命中后允许继续）/「脱敏后送模型」（脱敏后再向量化或送模型）/「阻止上传」（命中且策略=阻止上传时拒绝入库）；命中处理结果 MUST 写 `privacy_redaction_events` 并回填 `audit_logs.id`，被阻止的上传 MUST 写 `result=失败` 且 `failure_reason` 非空的审计。上传入口所属 change（c01 文档中心 / c04 AIMed 文件上传 / c06 知识库本地 / 批量上传 / c07 翻译文件上传）仅前置消费本契约、自身 MUST NOT 各自实现 PHI / PII 识别脱敏；该四类入口枚举为对外契约，引用方以本 owner 枚举为准。该上传闸接缝 SHALL 为**可插拔、缺省放行**：因 c09 为第 9（末）阶段而上传入口分布于 c01（phase 1）/ c04 / c06 / c07 等前序阶段，当 `redaction-gateway` 上传闸**尚未接入或不可用**时，四类上传入口 MUST 按 §19.4 POC 默认策略（「识别并提示」语义——识别能力缺位则视同未命中、允许继续入库）放行并照常写 `result=成功` 的上传审计，使前序阶段上传能力可独立交付 / 验收；仅当上传闸**已接入且策略=阻止上传且命中**时方拒绝入库；该缺省放行 MUST NOT 被实现为永久软门——上传闸接入后 MUST 收紧为强制门禁（命中按策略处理、被阻止时写 `result=失败` 审计）。

#### Scenario: 上传内容命中敏感信息并按「阻止上传」策略被阻断入库
- **WHEN** 用户经文档中心 / AIMed / 知识库本地批量 / 医学翻译四类上传入口中任一上传的内容命中 PHI / PII，且当前策略配置为「阻止上传」
- **THEN** 系统 MUST 在持久化入库或送模型前阻止该上传并提示命中原因，MUST NOT 将含敏感信息的原文写入存储或外送
- **AND** 系统 MUST 写一条 `result=失败` 且 `failure_reason` 非空的审计，并落 `privacy_redaction_events` 记录命中类型与所用策略

#### Scenario: 知识库本地 / 批量上传命中敏感信息按「阻止上传」被阻断入库
- **WHEN** 管理员或知识库管理员经 c06 知识库本地 / 批量上传入口上传的资料命中 PHI / PII，且当前策略配置为「阻止上传」
- **THEN** 系统 MUST 在持久化入 `kb_documents` 前阻止该上传并提示命中原因，MUST NOT 将含敏感信息的原文写入知识库存储
- **AND** 系统 MUST 写一条 `result=失败` 且 `failure_reason` 非空的审计，并落 `privacy_redaction_events` 记录命中类型与所用策略

#### Scenario: 上传内容命中敏感信息按默认策略脱敏 / 提示后入库
- **WHEN** 上传内容命中 PHI / PII 且当前为 POC 默认策略「识别并提示 + 脱敏后送模型」
- **THEN** 系统 MUST 识别并提示命中的敏感类型，并按策略脱敏后再持久化或送模型
- **AND** 系统 MUST 落 `privacy_redaction_events` 记录是否脱敏 / 命中类型 / 所用策略并回填 `audit_log_id`

#### Scenario: 上传闸未接入 / 不可用时按 §19.4 默认策略缺省放行入库
- **WHEN** `redaction-gateway` 上传闸尚未接入或不可用，而 c01 文档中心 / c04 AIMed / c06 知识库本地批量 / c07 翻译四类上传入口中任一发起文件上传（典型如 c01 phase 1 上传能力先于 c09 phase 9 交付）
- **THEN** 系统 MUST 按 §19.4 POC 默认策略放行入库，照常完成持久化并写一条 `result=成功` 的上传审计，使前序阶段上传能力可独立交付 / 验收
- **AND** 仅当上传闸**已接入且策略=阻止上传且命中**敏感信息时方拒绝入库；该缺省放行 MUST NOT 成为永久软门，上传闸接入后 MUST 收紧为强制门禁

### Requirement: 高风险 message 级文书下发前的确认链路收口
本要求为对 c05 `ai-writeback-confirmation` 既有高风险确认链路在 **message 级**（以 `message_id` 为键、在下发前）的**横切验收收口**：该链路判定与 `writeback_confirmations` 记录（已支持 `document_id` 或 `message_id`）的唯一 owner 为 c05，本 change（c09）不平行重定义、仅引用消费做统一验收与审计关联。message 级文书产生方为**三类**——AIMed 答案（c04）/ 知识库问答 `kb_qa` 答案（c06）/ 医学翻译文书（c07），三者均落 c04 所建 `conversations` / `messages` 表（kb_qa 以 `module=kb_qa` 标记），是同一条 message 级确认链路的生产方。系统 SHALL 收口：除文档写回（`document_id` 为键）外，AIMed 答案、知识库问答（`kb_qa`）答案与医学翻译文书在**下发前**若被识别为高风险（诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息），同样 MUST 进入 c05 确认链路并按 `confirmed_role`（取值 ∈ `{doctor, reviewer}`）裁决：普通用户只能查看提示 / 生成草稿 / 提交审核、MUST NOT 完成最终确认；确认记录 MUST 以 `message_id` 落 c05 `writeback_confirmations` 且字段（含 `confirmed_by` / `confirmed_role` / `risk_type` / `audit_log_id`）完整。c09 对该 message 级记录字段完整性与审计关联做验收收口，验收枚举覆盖上述三类生产方。

#### Scenario: 高风险 AIMed / 知识库问答 / 翻译文书 message 级下发前进入确认链路
- **WHEN** AIMed 答案、知识库问答（`kb_qa`）答案或医学翻译文书在下发前被 c05 拦截器识别为高风险内容
- **THEN** 系统 MUST 阻止其直接下发并要求进入 c05 确认链路，仅具备 `doctor` 或 `reviewer` 角色的确认人方可确认下发
- **AND** 普通用户 MUST NOT 完成最终确认，仅可生成草稿或提交审核

#### Scenario: 高风险知识库问答 kb_qa 答案 message 级下发前进入确认链路并以 message_id 落库
- **WHEN** 知识库问答（`module=kb_qa`）答案在下发前被 c05 拦截器识别为高风险内容
- **THEN** 系统 MUST 阻止其直接下发并要求进入 c05 确认链路，确认记录 MUST 以 `message_id` 落 c05 `writeback_confirmations`
- **AND** 普通用户 MUST NOT 完成最终确认，仅可生成草稿或提交审核

#### Scenario: message 级 confirmation 记录以 message_id 落库并关联审计
- **WHEN** 具备 `doctor` 或 `reviewer` 角色的确认人完成一次 message 级（AIMed / 知识库问答 / 翻译）高风险确认
- **THEN** c05 MUST 在 `writeback_confirmations` 生成以 `message_id` 为键、含 `confirmed_by` / `confirmed_role`（∈ `{doctor, reviewer}`）/ `risk_type` / `audit_log_id` 的确认记录
- **AND** c09 MUST 校验该记录字段完整、经 `audit_log_id` 关联审计，且 MUST 不可被事后篡改
