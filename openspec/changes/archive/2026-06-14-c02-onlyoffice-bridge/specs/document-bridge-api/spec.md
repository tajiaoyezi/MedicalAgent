## ADDED Requirements

### Requirement: Bridge 读取类方法提供文档内容出口
系统 SHALL 提供供医疗 AI 面板调用的 Bridge 读取类方法：getCurrentDocument、getDocumentId、getDocumentTitle、getDocumentType、getFullText、getSelectedText、getCurrentParagraph、getDocumentOutline、getCurrentPage、getComments、getReferences。每个读取方法 MUST 返回当前编辑器内最新内容并附带可用于溯源定位的位置信息（段落索引 / 选区 range / 页码 / 大纲层级）。读取方法 MUST 是只读的，且 MUST NOT 修改文档。

#### Scenario: 读取选中文本及其定位
- **WHEN** 医疗 AI 面板调用 getSelectedText
- **THEN** Bridge 返回当前选区文本以及选区的 range（起止位置）与所在段落/页码，供后续写回与引用定位使用

#### Scenario: 读取全文与文档大纲
- **WHEN** 面板调用 getFullText 与 getDocumentOutline
- **THEN** Bridge 返回文档全文及带标题层级的大纲结构，每个大纲节点附带其在文档中的定位信息

#### Scenario: 读取方法不改动文档
- **WHEN** 面板连续调用任意读取类方法
- **THEN** 文档内容与版本均不发生变化，且不触发保存回调

### Requirement: 读取类方法的原文出口须标记为对接 c09 脱敏门禁的挂载锚点
系统 SHALL 在 getFullText / getSelectedText / getCurrentParagraph 等会向上层 AI 输送文档原文的读取方法上，明确标识其为「原文出口」并预留对接 c09 `redaction-gateway` 的挂载锚点（命名以 c09 `redaction-gateway` 契约为准，本期仅预留锚点不实现 PHI / PII 识别脱敏），以便下游 AI 阶段在调用公网模型前由 c09 唯一 owner 的脱敏门禁执行识别与脱敏。Bridge 本身 MUST NOT 直接调用公网模型，也 MUST NOT 绕过下游 c09 脱敏门禁向公网发送原文；c02 不出网，仅作原文产出源并标记锚点。

#### Scenario: 原文出口可被 c09 脱敏门禁拦截
- **WHEN** 下游 AI 能力经由 getFullText 取得原文并准备调用公网模型
- **THEN** 该原文必须先经过 c09 `redaction-gateway` 的 PHI / PII 识别与脱敏门禁，门禁未通过时不得将原文送往公网模型

#### Scenario: Bridge 不自行外发原文
- **WHEN** 任一读取方法被调用
- **THEN** Bridge 仅将原文返回给请求方上下文，不直接向任何公网模型或外部服务发送原文

### Requirement: Bridge 写回类方法定义文档修改入口
系统 SHALL 提供 Bridge 写回类方法：replaceSelection、insertText、appendSection、insertComment、insertCitation、applyStyle、createNewDocument、createPresentation、saveDocument。这些方法 MUST 仅定义对编辑器的修改与新建行为；本期 MUST NOT 让 AI 自动落盘——任何替换/插入/新建在持久化前都由上层（c05 确认 UI）经用户确认后再调用，Bridge 自身不弹出确认 UI。

createPresentation 本期 MUST 仅定义可被调用的方法签名/承载能力，其唯一消费方（文档生成 PPT / 论文转 PPT）属 PRD §22.2 V1.1，本期 MUST NOT 产出可被 P0 验收的 PPT 内容生成结果；本期对 createPresentation 仅校验「方法签名存在且不覆盖原文档」的承载语义，不绑定「生成可用 pptx 内容」的验收。新建文档的 P0 可验收断言以 createNewDocument(content, templateId) 产出 docx 为准。

createNewDocument 的语义边界 MUST 明确为「编辑器内新建」：它在当前编辑会话上下文中创建一个新的 docx 文档/版本、最低权限为「可编辑」及以上，是 Bridge 写回类编辑器内能力，MUST NOT 被理解为「无既存目标文档/无编辑器会话的服务端净生成」入口。AIMed/在线文档 AI 等「服务端生成在线 Word/在线文档」的净生成闭环不依赖本方法的任何服务端新建变体——其文档创建由 c01 文档中心服务端创建服务承担（owner=c01，落 `documents`/`document_versions`、由 c01 文档中心入口产生 §10.6 `upload_success` 事件、c03 据此解析索引使生成文档可被 RAG 检索），c02 MUST NOT 自发明服务端新建契约、MUST NOT 将 createNewDocument 扩展为「按文档空间创建权限校验、无编辑器会话」的服务端变体。createNewDocument 的版本来源经保存回调链路落为 `document_versions`（见 save-callback-versioning），与 c01 服务端生成文档的入库链路相区分。

写回类方法 MUST 按子类区分最低权限（对齐 PRD §10.4 文档权限表「可评论 = 评论、查看、复制文本」「可编辑 = 编辑、保存、AI 写回」）：

- insertComment（仅插入批注/校对建议，不改写正文）最低权限为「可评论」及以上；「可评论」用户 SHALL 可调用 insertComment，「可查看」用户 MUST 被拒绝。
- replaceSelection、insertText、appendSection、insertCitation、applyStyle、createNewDocument、createPresentation、saveDocument（改写正文 / 新建文档 / 持久化）最低权限为「可编辑」及以上；「可评论」与「可查看」用户调用这些改正文/新建类方法 MUST 被拒绝。被拒后的兜底动作按 §10.4 区分：「可评论」用户仍可查看与复制文本，「可查看」用户仅允许查看（含受权限控制的下载），MUST NOT 复制文本（复制文本属「可评论」及以上专属能力）。

#### Scenario: 替换选区写回
- **WHEN** 上层在用户确认后调用 replaceSelection(text)
- **THEN** Bridge 用 text 替换当前选区内容，并保留可回退所需的原选区信息

#### Scenario: 以批注形式插入校对建议
- **WHEN** 具备「可评论」及以上权限的用户经上层调用 insertComment(range, comment)
- **THEN** Bridge 在指定 range 上插入批注而不直接改写正文

#### Scenario: 可评论用户可插批注但不可改正文
- **WHEN** 仅具备「可评论」权限的用户先调用 insertComment 再调用 replaceSelection
- **THEN** insertComment 放行并在 range 上插入批注，replaceSelection 因不具备「可编辑」权限被拒绝且仅允许读取或复制，两次调用均写入审计

#### Scenario: 插入带溯源的引用
- **WHEN** 上层调用 insertCitation(position, citation)，citation 含来源标识与定位信息
- **THEN** Bridge 在 position 处插入引用角标/条目，且引用条目携带可点击定位回源的元数据

#### Scenario: 新建文档不覆盖原文
- **WHEN** 上层在编辑会话内调用 createNewDocument(content, templateId)（编辑器内新建，调用者具「可编辑」及以上权限）
- **THEN** Bridge 创建新的 docx 文档/版本，原文档保持不变（此为本期 P0 可验收断言）
- **AND** 该方法不承担「服务端净生成在线 Word」闭环——AIMed/在线文档 AI 的服务端文档生成由 c01 文档中心创建服务承担并产生 `upload_success`，c02 不据本方法新增服务端新建变体

#### Scenario: createPresentation 仅作签名承载且不覆盖原文
- **WHEN** 上层调用 createPresentation(slideOutline, templateId)
- **THEN** Bridge 将其作为可被调用的方法签名承载，不覆盖原文档；本期不产出可被 P0 验收的 PPT 内容（PPT 内容生成属 §22.2 V1.1）

### Requirement: 写回须经用户确认且可回滚
系统 SHALL 保证任何 AI 触发的写回在落盘前向用户展示「原文 / 修改后 / 修改说明 / 影响范围」并提供「应用到文档 / 生成副本 / 取消」选择（确认 UI 实现归 c05，Bridge 提供承载所需的原文与变更数据）。系统 MUST 使每次落盘写回生成独立 document_version（含 source 与 file_hash），从而保证 AI 写回可回溯、可回滚。

#### Scenario: 写回前提供确认所需数据
- **WHEN** 上层准备应用一次 AI 写回
- **THEN** Bridge 能提供原文片段、修改后内容、影响范围（选区/段落/全文）供确认界面展示

#### Scenario: 用户取消则不改动文档
- **WHEN** 用户在确认界面选择「取消」
- **THEN** Bridge 不执行任何写回，文档与版本保持不变

#### Scenario: 写回落盘生成可回滚版本
- **WHEN** 用户选择「应用到文档」并完成保存
- **THEN** 系统生成一条 source=ai_writeback 且含 file_hash 的 document_version，可据此回滚到写回前版本

### Requirement: Bridge 面板控制方法
系统 SHALL 提供面板控制类方法：openAIPanel(command, payload)、closeAIPanel()、runAIPanelSkill(skillName, payload)、streamContentToEditor(content)。openAIPanel MUST 能携带初始命令与上下文打开右侧医疗 AI 面板；streamContentToEditor MUST 支持将 AI 生成内容以流式方式呈现到编辑器/面板（在未确认前不构成落盘写回）。

#### Scenario: 打开并预置命令的医疗 AI 面板
- **WHEN** 调用 openAIPanel(command, payload)
- **THEN** 系统在编辑器右侧打开医疗 AI 面板并按 command 预置对应技能上下文

#### Scenario: 流式内容呈现不等于落盘
- **WHEN** 调用 streamContentToEditor(content) 将生成内容流式展示
- **THEN** 内容仅作预览呈现，未经用户确认不写入文档版本

### Requirement: Bridge 调用的 token、权限、租户与审计安全
系统 SHALL 对每次 Bridge 调用进行安全校验：校验调用 token 的有效性、调用者对目标文档的 tenant_id 与 ACL 权限、以及方法类别所需的最低权限（读取类需「可查看」及以上；写回类中 insertComment 需「可评论」及以上，其余改正文/新建类写回需「可编辑」及以上）。系统 MUST 拒绝越权或跨租户调用。

审计落点 MUST 区分两类表，对齐 c01 拥有的表契约：所有写回类调用（成功或被拒）MUST 写入 `audit_logs`（仅写 c01 已定义列：操作者、`role`、`tenant_id`、操作类型、对象、时间、`result`、`failure_reason`），MUST NOT 写入 IP 等 c01 未定义列。`document_events` 仅由「成功落盘并产生新版本」的写回经保存回调链路产生（event_type=ai_writeback，携带 version_id 与 §10.6 稳定契约字段，见 save-callback-versioning）；被拒调用、读取调用、未落盘的写回 MUST NOT 写入 `document_events`。

#### Scenario: 无效或过期 token 被拒绝
- **WHEN** Bridge 收到携带无效或过期 token 的调用
- **THEN** 系统拒绝调用并返回鉴权失败，不执行任何读写

#### Scenario: 越权改正文写回被拒绝
- **WHEN** 仅有「可查看」权限的用户触发 replaceSelection 等改正文/新建类写回方法，或仅有「可评论」权限的用户触发 replaceSelection 等改正文/新建类写回方法
- **THEN** 系统拒绝该改正文/新建类写回并记录一条审计日志；被拒后「可查看」用户仅允许查看（含受权限控制的下载）不含复制文本，「可评论」用户仍可查看、复制文本并调用 insertComment

#### Scenario: 跨租户 Bridge 调用被拒绝
- **WHEN** 调用目标文档的 tenant_id 与调用者租户不一致
- **THEN** 系统拒绝调用并写入审计

#### Scenario: 写回类调用全部写 audit_logs
- **WHEN** 任一写回类方法被执行（成功或被拒）
- **THEN** 系统向 `audit_logs` 写入一条记录，仅含 c01 已定义列（操作者、`role`、`tenant_id`、操作类型、对象=document、时间、`result`、`failure_reason`），方法名归入操作类型、来源归入对象信息，不写入 IP 等未定义列
- **AND** 仅当该写回成功落盘并产生新版本时，才由保存回调链路另产生一条 `document_events`(event_type=ai_writeback)，被拒或未落盘的写回不写 `document_events`
