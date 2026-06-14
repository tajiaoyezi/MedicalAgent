## Why

MedOffice AI 已彻底放弃 WPS 全家桶，文档的在线打开、编辑、保存、版本与「医疗 AI 写回」必须落在一套自建的在线 Office 底座上，否则主验收闭环（上传 → AIMed → 生成在线 Word → 文档中心 → 在线打开 → 右侧面板润色/校对/翻译选区 → 确认写回 → 保存新版本）就无法贯通（PRD §9、§24.2）。本期（9 阶段中的第二阶段 onlyoffice-bridge）在 c01 已交付的账号/权限、文档中心与对象存储之上，引入 ONLYOFFICE Docs 作为四类文档的在线编辑器，并建立医疗 AI 面板与编辑器之间的文档 Bridge API 和保存回调-版本链路。它是后续 model-and-parse、aimed-rag-citation、ai-panel-recent-tasks、medical-translation、template-center 等阶段全部「在文档里做 AI」的承载层，没有它后面阶段的能力无处落地。

## What Changes

- 新增 ONLYOFFICE 四类编辑器接入：docx/doc → Document Editor、xlsx/xls → Spreadsheet Editor、pptx/ppt → Presentation Editor、pdf → PDF Editor/Viewer；按文件类型从文档中心打开、编辑、保存（PRD §9.2、§22.1、§24.2）。
- 新增 PDF 预览并支持 AI 处理入口；ofd 通过「转 PDF」做只读预览（V1.0 不支持 OFD 原生在线编辑）；png/jpg 走图片预览并提供文档视觉解析入口（PRD §9.2、§9.5、§23）。
- 新增医疗 AI 右侧面板与编辑器之间的文档 Bridge API：读取类（getFullText / getSelectedText / getDocumentOutline / getCurrentParagraph / getComments / getReferences 等）、写回类（replaceSelection / insertText / appendSection / insertComment / insertCitation / applyStyle / createNewDocument / createPresentation / saveDocument）、面板控制类（openAIPanel / closeAIPanel / runAIPanelSkill / streamContentToEditor），并要求 Bridge 调用具备 token/权限/租户校验与审计（PRD §9.3、§9.7）。
- 新增保存链路：编辑器 callbackUrl → Document Service 下载最新文件 → 写入对象存储 → 创建 document_version → 更新文档元数据 → 触发异步解析与索引；版本记录文件 hash、保存人、保存时间与来源（user_edit / ai_writeback / translation / import / template）（PRD §9.8、§10.5、§10.6）。
- 新增文档冲突与回调失败处理：保存回调失败自动重试、失败后告警；多人同时编辑沿用 ONLYOFFICE 协作并由服务端保留版本；AI 写回时文档已变更则提示重新读取上下文；文档被删除或无编辑权限时禁止写回（PRD §9.9）。
- 性能约束：普通 docx/pptx/xlsx 在线打开 ≤ 5 秒、普通 docx 保存回调 ≤ 10 秒（PRD §21）。
- 本期不含破坏性变更：c01 之前无任何在线编辑器与 Bridge 能力，全部为净新增，无对既有 spec 行为的修改。

## Capabilities

### New Capabilities

- `onlyoffice-integration`：ONLYOFFICE Document / Spreadsheet / Presentation / PDF 四类编辑器接入，按文件类型从文档中心打开、编辑、保存；pdf 预览并提供 AI 处理入口；ofd 转 PDF 只读预览（不含 OFD 原生在线编辑）；png/jpg 图片预览并提供视觉解析入口。
- `document-bridge-api`：医疗 AI 面板与编辑器之间的文档 Bridge API，含读取类（getFullText / getSelectedText / getDocumentOutline 等）、写回类（replaceSelection / insertComment / insertCitation / createNewDocument / saveDocument 等）、面板控制类，以及 token / 权限 / 租户 / 审计安全要求（写回方法在此定义，不含 AI 写回确认弹窗 UI 与具体 AI 技能）。
- `save-callback-versioning`：保存链路 callbackUrl → Document Service 下载 → 写对象存储 → 创建 document_version → 更新元数据 → 触发异步解析；并覆盖保存回调失败重试与文档冲突处理。

### Modified Capabilities

（无。`openspec/specs/` 当前为空，本期能力全部为新增，无既有 spec 的需求级行为变更。）

## Impact

- 受影响服务：在线 Office 服务（ONLYOFFICE Docs / Document Service 部署与接入）、医疗 AI 面板宿主、Bridge API 网关、保存回调处理服务、异步解析与索引触发器；依赖 c01 交付的对象存储（MinIO/S3）、文档中心与权限服务。
- 受影响数据表（PRD §18）：写入/读取 `documents`、`document_versions`、`document_permissions`、`document_events`（c02 产生 `save_new_version` / `ai_writeback` 两类事件，由唯一消费方 c03 消费后创建 `document_parse_jobs` 并消费下游 `document_chunks` / `embeddings`，c02 不直接写解析作业表）；安全相关写入 `audit_logs`。本期只消费 c01 已建立的 `users` / `tenants` / `roles` / `permissions`，不改其结构。
- 对其它 phase 的依赖与解锁：上游依赖 c01（账号/权限、文档中心、对象存储）；本期产出的 Bridge 写回方法与保存-版本链路被 c03 model-and-parse（异步解析/视觉解析）、c04 aimed-rag-citation（生成在线 Word 入口）、c05 ai-panel-recent-tasks（写回确认 UI 与 AI 技能）、c07 medical-translation（当前文档发起翻译、译文副本）、c08 template-center（用模板新建/排版新版本）直接复用。AI 写回确认 UI 与具体技能不在本期实现，归 c05。
- 医疗安全与合规影响：
  - 权限与多租户——Bridge 读写与保存回调必须按 tenant_id / document ACL / role 校验：「可编辑」及以上才允许 AI 改正文写回，「可评论」可调用 insertComment 插入批注但不可改正文（仍可查看、复制文本），「可查看」仅允许查看（含受权限控制的下载）、不含复制文本（复制文本属「可评论」及以上专属，对齐 §10.4「可评论 = 评论、查看、复制文本；可查看 = 查看、下载受权限控制」）；文档被删除或无编辑权限一律禁止改正文写回（PRD §9.9、§10.4）。
  - 人工确认与可回滚——本期 Bridge 只定义写回方法，不得让 AI 自动落盘；每次保存生成独立 document_version（含 source 与 file_hash），保证 AI 写回可回溯、可回滚（确认交互由 c05 承担）（PRD §9.6、§10.5）。
  - 脱敏前置——Bridge 读取类方法（getFullText/getSelectedText 等）会向上层 AI 输送文档原文，调用公网模型前的 PHI/PII 识别与脱敏由 c09 redaction-gateway（脱敏门禁唯一 owner）在下游公网出口执行，本期需在接口契约中明确「原文出口」位置并预留对接 c09 redaction-gateway 的挂载锚点（命名以 c09 契约为准），c02 不出网、不在本期直接调用公网模型。
  - 审计——访问类（打开/编辑）与 Bridge 写回类调用（成功或被拒）写入 `audit_logs`（仅 c01 已定义列：操作者/role/tenant_id/操作类型/对象/时间/result/failure_reason，本期不含 IP）；`document_events` 仅由保存回调链路在产生新版本时写入 `save_new_version` / `ai_writeback` 两类（owner=c01 的 §10.6 契约，携带 document_id/version_id/tenant_id/occurred_at/payload），不承载访问类审计，满足可审计红线。
