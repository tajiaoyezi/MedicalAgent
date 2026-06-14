## ADDED Requirements

### Requirement: 文档中心文件空间
系统 SHALL 提供文档中心作为文件存储与管理入口，并提供以下文件空间视图：我的文档、团队文档、应用文档（含 AIMed 学术助手、医学翻译、医疗模板生成、数字员工输出、知识库文档子来源）、回收站。文件空间内展示的文件 MUST 按当前用户在所属租户内的权限过滤，无权限文件 MUST NOT 在列表中可见。

#### Scenario: 按空间分类展示文件
- **WHEN** 用户进入文档中心
- **THEN** 系统提供我的文档、团队文档、应用文档（含各来源子分类）与回收站视图

#### Scenario: 按权限过滤文件列表
- **WHEN** 用户浏览团队文档或应用文档空间
- **THEN** 仅展示该用户在本租户内有访问权限的文件，无权限文件不出现在列表

#### Scenario: 删除文件进入回收站
- **WHEN** 用户删除一个文件
- **THEN** 该文件移入回收站而非立即物理清除，并记录删除审计事件

### Requirement: 文件操作
系统 SHALL 支持以下文件操作：上传、新建、打开（本期仅路由占位）、重命名、复制、移动、删除、下载、分享、收藏、版本历史、权限管理、加入知识库、发起 AIMed、发起翻译、用模板新建。每个操作 MUST 先校验用户对该文件的权限分级，权限不足时 MUST 拒绝；下载与分享 MUST 受权限控制，且上传、删除、下载、权限变更、分享操作 MUST 写入审计日志。其中文件上传审计由 c01 在上传入口产生（成功/失败均留痕），c09 仅核对审计完备性、不重复产生（对齐 PRD §24.7 与 c09 tasks 2.2/7.1）。文档中心上传入口在内容持久化入 `documents` / `document_versions` 与对象存储前 MUST 先经 c09 `redaction-gateway` 上传闸做 PHI / PII 识别，并按策略「识别并提示 / 脱敏后送模型 / 阻止上传」处理（对齐 PRD §19.4 上传时识别时点与 §0.3 安全脱敏闭环）；当 c09 已接入且策略=阻止上传且命中敏感信息时 MUST 拒绝入库；PHI / PII 识别与脱敏、`privacy_redaction_events` 留痕的唯一 owner 为 c09，c01 文档中心上传入口仅前置消费本契约、MUST NOT 自行实现识别脱敏（该上传闸入口枚举与 c04 AIMed / c07 翻译并列，由 c09 security-compliance「上传时 PHI / PII 识别与『阻止上传』策略执行」契约统一收口）。该上传闸接缝 MUST 设计为「可插拔、缺省放行」：当 c09 `redaction-gateway` 尚未接入或不可用时（如 foundation 在 phase 1 独立交付、c09 为第 9 阶段尚未上线），上传入口 MUST 按 PRD §19.4 POC 默认策略「识别并提示 + 脱敏后送模型」放行入库（允许继续，不阻断），并照常写一条 `result=成功` 的上传成功审计；仅当 c09 已接入且其策略=阻止上传且命中敏感信息时方拒绝入库。由此 task 9.1 / 9.5（上传文件落盘并算出 `file_hash`、离线跑通主线）在 phase 1 即可独立验收，phase 9 c09 接入后再将门禁收紧为强制执行。其中「打开」「加入知识库」「发起 AIMed」「发起翻译」「用模板新建」本期 MUST 仅提供入口/路由占位并完成权限分级校验，实际触发编排分别由 c04（发起 AIMed）、c07（发起翻译）、c08（用模板新建）、c06（加入知识库）、c02（打开 ONLYOFFICE）在各自相位实现，本 phase MUST NOT 真正发起下游能力。

#### Scenario: 有下载能力的用户成功下载
- **WHEN** 拥有 §10.4「下载」能力（即 `owner` / `manage` / `edit` / `view` 四级之一，仅 `comment` 被排除）的用户对文件发起下载
- **THEN** 系统在校验权限通过后提供文件下载，并记录下载审计事件

#### Scenario: 可评论级别不含下载能力被拒绝
- **WHEN** 仅 `comment`（可评论）级别（按 §10.4 不含下载能力）或更低权限的用户尝试下载文件
- **THEN** 系统按 §10.4 能力位查表判定其不具备下载能力，拒绝下载并提示权限不足，记录被拒绝的访问审计事件

#### Scenario: 分享受权限控制
- **WHEN** 用户对一个无“可管理”及以上权限的文件发起分享
- **THEN** 系统拒绝分享操作并提示权限不足

#### Scenario: 上传内容在持久化入库前经 c09 上传闸识别按策略处理
- **WHEN** 用户在文档中心上传入口上传文件，文件内容命中 PHI / PII，且 c09 `redaction-gateway` 已接入
- **THEN** 系统在持久化入 `documents` / `document_versions` 与对象存储前 MUST 先经 c09 `redaction-gateway` 上传闸做 PHI / PII 识别，并按「识别并提示 / 脱敏后送模型 / 阻止上传」策略处理
- **AND** 当策略=阻止上传且命中敏感信息时 MUST 拒绝入库、写一条 `result=失败` 且 `failure_reason` 非空的上传审计，识别脱敏与 `privacy_redaction_events` 留痕由 c09 owner 承担，c01 仅前置消费本契约

#### Scenario: c09 上传闸未接入时按默认策略放行
- **WHEN** c09 `redaction-gateway` 上传闸尚未接入或不可用（如 foundation 在 phase 1 独立交付、c09 尚未上线），用户在文档中心上传入口上传文件
- **THEN** 上传入口按 PRD §19.4 POC 默认策略「识别并提示 + 脱敏后送模型」放行入库（允许继续、不阻断），照常持久化入 `documents` / `document_versions` 与对象存储并算出 `file_hash`，并写一条 `result=成功` 的上传成功审计
- **AND** 仅当 c09 已接入且其策略=阻止上传且命中敏感信息时方拒绝入库；该上传闸接缝为「可插拔、缺省放行」，使 task 9.1 / 9.5 在 phase 1 即可独立验收，phase 9 c09 接入后再收紧为强制门禁

### Requirement: 文档权限分级
系统 SHALL 提供文档级权限分级（`document_permissions`）：所有者、可管理、可编辑、可评论、可查看、无权限。各级能力 MUST 按 PRD §10.4 能力矩阵声明，且该矩阵 MUST NOT 按级别序单调累加：所有者具备全部操作；可管理具备编辑/分享/权限管理/删除；可编辑具备编辑/保存/AI 写回；可评论具备评论/查看/复制文本；可查看具备查看与下载（下载受权限控制）；无权限不可访问。「下载」是 §10.4 按能力位声明的独立能力——`owner`（所有者）/ `manage`（可管理）/ `edit`（可编辑）/ `view`（可查看）四级均具备下载能力（owner=全部操作隐含含下载；manage / edit 为高于 view 的功能级别，按 §10.4「下载」为高功能级别隐含包含而具备下载；view 级别本身显式声明下载），唯一例外是 `comment`（可评论）级别 MUST NOT 具备下载能力——`comment` 是整张矩阵中下载能力位上的唯一非单调缺口（即便其在级别序上高于 `view`（可查看））。故「下载」判定 MUST 按 §10.4 能力位查表（owner / manage / edit / view = 含下载，仅 comment = 不含下载），而 MUST NOT 按「级别 ≥ 可查看」做简单级别序推断。权限授予与变更 MUST 仅由所有者或可管理者执行，并 MUST 写入审计日志。

#### Scenario: 可编辑者可触发 AI 写回
- **WHEN** 拥有“可编辑”权限的用户对文档执行保存或 AI 写回
- **THEN** 系统允许该操作并生成新版本

#### Scenario: 可查看者不可编辑
- **WHEN** 仅“可查看”权限的用户尝试编辑或写回文档
- **THEN** 系统拒绝操作并提示权限不足

#### Scenario: 可管理或可编辑用户下载成功
- **WHEN** 拥有 `manage`（可管理）或 `edit`（可编辑）级别的用户对文件发起下载
- **THEN** 系统按 §10.4「下载」为高功能级别隐含包含（owner=全部操作、manage / edit 为高于 view 的功能级别含下载、`comment` 为唯一被排除的非单调缺口）判定其具备下载能力，校验通过后放行下载并记录一条下载审计事件
- **AND** 同一用户被置为 `comment`（可评论）级别时再发起下载 MUST 被拒绝（comment 不含下载能力）

#### Scenario: 权限变更需授权并审计
- **WHEN** 所有者或可管理者修改某用户对文档的权限分级
- **THEN** 系统保存新的权限分级并写入审计日志
- **AND** 非所有者且非可管理者执行同一操作时被拒绝

### Requirement: 文档版本
系统 SHALL 在每次保存时生成文档版本（`document_versions`），每个版本 MUST 至少包含 PRD §10.5 清单字段 `version_id`、`document_id`、`file_hash`、`saved_by`、`saved_at` 以及 `source`。`source` 取值 MUST 限定为 `user_edit` / `ai_writeback` / `translation` / `import` / `template` 之一。此外，系统 MAY 记录 `document_version` 作为对 PRD §10.5 的补充字段（人读版本序号/版本号，非 §10.5 原清单项），用于展示与排序，非版本唯一标识（唯一标识仍为 `version_id`）。版本记录 MUST 不可篡改既有内容，新保存以追加新版本方式记录，以支撑后续可回滚与“原文/修改后/说明/影响范围”确认链路。

#### Scenario: 保存生成带来源的新版本
- **WHEN** 用户保存文档
- **THEN** 系统追加一个新版本，记录 `document_version`、`file_hash`、`saved_by`、`saved_at` 与对应 `source`

#### Scenario: AI 写回版本可识别来源
- **WHEN** 通过 AI 写回保存文档
- **THEN** 新版本的 `source` 取值为 `ai_writeback`，可被后续模块据此区分

#### Scenario: 历史版本可查看
- **WHEN** 有权限用户查看文档版本历史
- **THEN** 系统按时间顺序列出各历史版本及其 `source`、`saved_by`、`saved_at`

### Requirement: 触发重新解析与索引的事件
系统 SHALL 定义一份覆盖 PRD §10.6 全部 6 类触发源的稳定 `document_events` 事件契约，`event_type` 取值 MUST 覆盖 `upload_success`（上传成功）/ `save_new_version`（保存新版本）/ `ai_writeback`（AI 写回）/ `translation_done`（翻译完成）/ `template_created`（模板创建）/ `manual_reindex`（手动重建索引）六类；每条事件 MUST 携带 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)` 稳定字段。`document_events` 表 MUST 仅承载上述 6 类 `event_type`；文档打开等访问类操作与解析作业生命周期（作业创建/解析中/失败/重试）审计 MUST NOT 写入 `document_events`，一律写 `audit_logs`。每类 `event_type` MUST 有唯一产生方：`upload_success` 由本 phase（c01 文档中心入库入口）产生——`upload_success` 的唯一产生方为 c01 文档中心，覆盖「用户上传文件成功」与「文档中心服务端创建服务净生成文档首版入库成功」两条入库路径（详见下「文档中心服务端创建服务」Requirement，AIMed「生成在线 Word / 在线文档」经该创建服务落 `documents` / `document_versions` 后由 c01 产生 `upload_success`），c01 为 `upload_success` 的唯一产生方、其它 change MUST NOT 产生 `upload_success`；`save_new_version` 与 `ai_writeback` 由 c02 保存回调产生（保存回调产生新版本时落 `save_new_version`、回调上下文带 `writebackSource`（AI 写回）时落 `ai_writeback`）；`translation_done` 由 c07（翻译成功时）产生；`template_created` 由 c08（使用模板复制成功时）产生；`manual_reindex` 由 c06（管理员重建索引时）产生、并由 c03 在消费侧触发重解析。本 phase（foundation，未实现解析/写回/翻译/索引）MUST 仅产生 `upload_success` 一类事件；`save_new_version` / `ai_writeback`（c02）/ `translation_done`（c07）/ `template_created`（c08）/ `manual_reindex`（c06）由各自唯一产生方在其相位产生，本 phase MUST NOT 自行触发，仅以契约形态承载其 `event_type`。事件消费（解析、视觉解析、向量/全文索引）属 c03/c06，本 phase 仅定义并产生 `upload_success` 事件，不实现消费侧逻辑。

#### Scenario: 上传成功触发解析索引事件
- **WHEN** 用户上传文件成功
- **THEN** 系统产生一条 `event_type=upload_success` 的 `document_events`，携带 `tenant_id`、`document_id`、`version_id`、`occurred_at` 与 `payload`

#### Scenario: 服务端创建服务净生成文档首版入库产生 upload_success
- **WHEN** 文档中心服务端创建服务（如 AIMed「生成在线 Word / 在线文档」调用）将一份净生成内容落入 `documents` / `document_versions` 首版成功
- **THEN** 系统由 c01 文档中心产生一条 `event_type=upload_success` 的 `document_events`，携带 `tenant_id`、`document_id`、`version_id`、`occurred_at` 与 `payload`，与上传路径同构、供 c03 消费解析索引使生成文档可被 RAG 检索
- **AND** `upload_success` 的唯一产生方为 c01 文档中心，下游 change MUST NOT 自行产生 `upload_success`

#### Scenario: 打开与解析作业审计不写 document_events
- **WHEN** 发生文档打开（访问类）或解析作业生命周期（创建/解析中/失败/重试）等需审计的动作
- **THEN** 系统将该审计写入 `audit_logs`（操作类型=open 或对应作业状态），MUST NOT 写入 `document_events`
- **AND** `document_events` 仅承载 §10.6 的 6 类 `event_type`

#### Scenario: 下游 phase 触发类型由唯一产生方在各相位产生
- **WHEN** 检视 `document_events` 的 `event_type` 契约
- **THEN** 契约 MUST 能承载 `save_new_version` / `ai_writeback` / `translation_done` / `template_created` / `manual_reindex` 等取值
- **AND** 本 phase MUST NOT 产生这五类事件——`save_new_version` 与 `ai_writeback` 由 c02 保存回调产生（`ai_writeback` 在回调上下文带 `writebackSource` 时产生）、`translation_done` 由 c07 产生、`template_created` 由 c08 产生、`manual_reindex` 由 c06 产生并由 c03 消费侧触发重解析

### Requirement: 文档中心服务端创建服务
系统 SHALL 提供文档中心服务端创建服务（创建 API）作为「无既存目标文档、无 ONLYOFFICE 编辑器会话」时由服务端净生成一份新文档并落库的唯一入口，将净生成内容写入 `documents` / `document_versions` 首版（`document_versions.source` 取 `ai_writeback` / `template` / `import` 等对应来源），`documents` / `document_versions` 的建表与落库 owner 为 c01。该创建服务 MUST 按目标文档空间的「新建 / 创建」能力校验调用者权限（而非对一份尚不存在的目标文档校验「可编辑」），`templateId` 在 AIMed 净生成等无模板场景 MAY 缺省。创建服务落库成功后 MUST 由 c01 文档中心产生一条 `event_type=upload_success` 的 `document_events`（c01 为 `upload_success` 唯一产生方），供 c03 消费解析索引使生成文档可被 RAG 检索。AIMed「生成在线 Word / 在线文档」MUST 经本创建服务落库并由本服务产生 `upload_success`，再经 c02 打开 ONLYOFFICE 编辑；c02 编辑器内 `createNewDocument` 仅为编辑器内新建（要求可编辑权限），与本服务端创建服务区分，c04 MUST NOT 依赖 c02 编辑器内 `createNewDocument(content, templateId)` 变体写回作为服务端新建入口。本 phase（foundation）MUST 实现本创建服务的落库与 `upload_success` 产生闭环；AIMed 净生成内容的具体编排归 c04、打开 ONLYOFFICE 归 c02，本 phase 仅提供并验收创建服务自身的落库与事件产生。

#### Scenario: AIMed 生成在线 Word 经创建服务落库并产生 upload_success
- **WHEN** AIMed「生成在线 Word / 在线文档」调用文档中心服务端创建服务，按目标文档空间「新建 / 创建」能力校验通过（`templateId` 缺省），将净生成内容落入 `documents` / `document_versions` 首版
- **THEN** 系统由 c01 文档中心产生一条 `event_type=upload_success` 的 `document_events`，携带 `tenant_id`、`document_id`、`version_id`、`occurred_at` 与 `payload`，供 c03 消费解析索引、使生成文档可被 RAG 检索
- **AND** 该文档随后经 c02 打开 ONLYOFFICE 编辑，本创建服务 MUST NOT 依赖 c02 编辑器内 `createNewDocument` 变体作为服务端落库入口

#### Scenario: 无创建权限调用者被拒绝
- **WHEN** 在目标文档空间不具备「新建 / 创建」能力的调用者请求服务端创建服务
- **THEN** 系统拒绝创建、不写入 `documents` / `document_versions`、不产生 `upload_success`，并记录被拒绝的审计事件
