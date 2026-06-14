## Context

本 change（9 阶段中的第二阶段 `c02-onlyoffice-bridge`）在 c01 已交付的账号/权限、文档中心与对象存储（MinIO/S3）之上，引入 ONLYOFFICE Docs 作为四类文档（docx/xlsx/pptx/pdf）的在线编辑器底座，并建立医疗 AI 右侧面板与编辑器之间的文档 Bridge API、保存回调-版本链路，以及多人/AI 写回的冲突与版本保留机制。它承载主验收闭环中「文档中心 → 在线打开 → 右侧面板润色/校对/翻译选区 → 确认写回 → 保存新版本」这一段（PRD §9、§24.2）。

当前状态与约束：

- 上游依赖（c01 已交付）：`users` / `tenants` / `roles` / `permissions`、文档中心、对象存储、文档元数据与 ACL（`documents` / `document_permissions`）。本期只消费、不改其结构。
- 范围口径：V1.0 为 POC 演示版，可部署在个人机或内网服务器，部署环境可能无公网。仅覆盖 PRD §22.1 P0 中本期相关项：ONLYOFFICE Document/Presentation/Spreadsheet Editor 集成、PDF Viewer/Editor 预览与 AI 处理、医疗 AI 右侧面板（面板的「打开/读取选区/写回选区」承载能力）。
- 性能基线（PRD §21）：普通 docx/pptx/xlsx 在线打开 ≤ 5 秒；普通 docx 保存回调 ≤ 10 秒。
- 安全红线（context rules）：Bridge 读写与保存回调必须按 tenant_id / document ACL / role 校验；本期 Bridge 只「定义写回方法」，不得让 AI 自动落盘；读取类方法是「原文出口」，需为下游脱敏挂载预留位置；Bridge 调用与保存回调结果写审计。
- 利益相关方：医生/科研/医务行政（终端编辑与 AI 写回）、平台运维（ONLYOFFICE 与回调服务部署）、下游 phase 作者（c03/c04/c05/c07/c08 复用 Bridge 与版本链路）。

本期是净新增：`openspec/specs/` 当前为空，无既有 spec 行为被修改。proposal 定义的三个新能力为 `onlyoffice-integration` / `document-bridge-api` / `save-callback-versioning`。

## Goals / Non-Goals

**Goals:**

- 用 ONLYOFFICE Docs 私有化部署支撑四类文档在线打开/编辑/保存，按文件类型路由到对应编辑器；pdf 预览并提供 AI 处理入口；ofd 转 PDF 只读预览；png/jpg 图片预览并提供视觉解析入口（PRD §9.2、§9.5）。
- 定义医疗 AI 面板 ↔ 编辑器的文档 Bridge API 读取/写回/面板控制三类方法契约，及其 token/权限/租户/审计安全要求（PRD §9.7）。
- 建立保存回调链路：callbackUrl → Document Service 下载最新文件 → 写对象存储 → 创建 `document_versions` → 更新文档元数据 → 投递异步解析任务（PRD §9.8、§10.6）。
- 覆盖保存回调失败重试+告警、多人协作版本保留、AI 写回时文档已变更/被删除/无权限的冲突处理（PRD §9.9）。
- 满足打开 ≤ 5s、保存回调 ≤ 10s 的性能目标，并给出离线/私有化部署路径。

**Non-Goals（明确排除）:**

- AI 写回确认弹窗 UI（原文/修改后/修改说明/影响范围 + 应用/生成副本/取消）与具体 AI 技能（润色/校对/翻译/排版/转 PPT）——归 c05（PRD §9.6 的交互在 c05 落地，本期只定义写回方法与「不自动落盘」约束）。
- 文档视觉解析/异步解析的实现（OCR/多模态/版面/表格识别）——本期只「投递」解析任务，实现归 c03。
- 生成在线 Word 的 AIMed 生成入口（c04）、医学翻译任务编排（c07）、模板新建/排版（c08）——本期只提供它们复用的 `createNewDocument` / `createPresentation` / `saveDocument` 方法与版本 source 取值。
- 公网模型调用与 PHI/PII 脱敏的实际执行——本期不调用公网模型，仅在读取类接口契约中标注「原文出口」供下游脱敏挂载。
- OFD 原生在线编辑（V1.0 不支持，PRD §9.2/§9.5）；数字员工任何能力。

## Decisions

### D1. ONLYOFFICE 私有化部署 + 文档服务/业务服务网络隔离

**决策**：ONLYOFFICE Docs（含 Document Server，下称 DS）以容器方式私有化部署在内网，与业务后端（下称 BFF/业务服务）分处两个网络区：

- DS 区只暴露编辑器静态资源与 DS 自身端点给前端；DS 通过受控的 callbackUrl 与文件下载 URL 反向访问业务服务的「文档网关」端点，不直连业务库与对象存储。
- 业务服务持有对象存储凭证；DS 需要的源文件通过业务网关签发的「短时效预签名/一次性下载 URL」获取，保存回调的最新文件也由业务网关拉取，DS 不持久化对象存储密钥。
- 前端拿到的编辑器配置（document.url、callbackUrl、permissions）由业务服务签发并 JWT 签名，DS 校验 JWT 后才接受。

**备选与取舍**：

- 备选 A：DS 直连 MinIO/S3 与业务库。被否——把对象存储与 DB 凭证下放到第三方文档服务，违反多租户隔离与最小权限红线，且 DS 一旦被攻破即横向波及全部租户数据。
- 备选 B：用 ONLYOFFICE 公有云/SaaS。被否——部署环境可能无公网且涉及医疗文档，PHI 不得出内网。
- 选私有化+网络隔离：DS 只是「无状态渲染/协同引擎」，所有数据面经业务网关管控，满足离线优先与隔离红线。

**离线/私有化降级**：DS 全量私有化镜像，无任何外呼依赖；字体/插件本地打包。无公网时编辑/保存/协同链路完全可用。

### D2. JWT/token 校验贯穿编辑器配置、callbackUrl 与文件下载

**决策**：启用 ONLYOFFICE JWT（`inbox`/`outbox`/浏览器三处均开），业务服务用共享密钥签发编辑器初始化 config（含 `document.key`、`document.url`、`editorConfig.callbackUrl`、`permissions`）。同时叠加一层业务侧的一次性 `open_token`：

- 前端「打开文档」时，业务服务校验登录态 + document ACL（tenant_id / role / 文档级权限），通过后签发短时效 `open_token` 并据此生成 DS config；`permissions.edit` 严格由 ACL 推导（「可编辑」及以上→可编辑可写回；「可评论」→只读+批注；「可查看」→只读）。
- callbackUrl 内嵌不可猜测、绑定 `document_id`+`document.key`+签发者的一次性回调凭证；业务回调端点先验 DS 的 outbox JWT，再验回调凭证，再核对 `document.key` 与当前会话一致，三者皆过才接受存盘。
- 文件下载 URL 为短时效预签名，且与 `open_token` 绑定，过期即失效。

**备选与取舍**：

- 备选：仅依赖 DS JWT。被否——JWT 只证明「来自 DS」，不证明「这个用户对这个文档有权限」，无法满足文档级 ACL 与多租户过滤；故必须叠加业务侧 token 与 ACL 二次校验。
- 取舍：双层校验增加少量签发/校验开销，但换取「DS 不可伪造越权、回调不可重放」，符合安全红线，性价比高。

### D3. Bridge API 承载方式：ONLYOFFICE 插件 + 受控 postMessage + 宿主桥三段式

**决策**：医疗 AI 右侧面板作为编辑器外的宿主 UI（与 DS iframe 同源宿主页），Bridge 采用三段式：

1. **编辑器内**：用 ONLYOFFICE 官方插件机制 + `Asc.scope`/`callCommand`/`executeMethod`/`callModule` 在文档 DOM 内执行读取与写回（getFullText/getSelectedText/replaceSelection/insertText/insertComment/applyStyle 等都映射到插件 API 与 builder 调用）。
2. **桥接通道**：插件与宿主面板之间用受限 `postMessage`（严格校验 `event.origin` 白名单、固定消息 schema、带请求 id 做请求-响应配对），不暴露任意 eval。
3. **宿主桥（host bridge）**：宿主页提供统一 JS SDK（`bridge.getSelectedText()` 等 Promise 化方法），面板只调 SDK；SDK 内部转 postMessage 到插件，并对「写回类」方法在发起前回调业务服务做 token/ACL/租户校验+审计落点（见 D2、D7）。

**读写方法契约（与 PRD §9.7 一一对应）**：

- 读取类：`getCurrentDocument/getDocumentId/getDocumentTitle/getDocumentType/getFullText/getSelectedText/getCurrentParagraph/getDocumentOutline/getCurrentPage/getComments/getReferences`。统一返回 `{ok, data, docKey, revision}`，其中 `revision` 用于写回时的乐观校验；`getFullText/getSelectedText/getCurrentParagraph` 标注为「原文出口」，返回体携带对接 c09 `redaction-gateway` 的挂载锚点占位元数据（命名以 c09 `redaction-gateway` 契约为准，本期仅预留锚点不实现识别脱敏），供下游 AI 阶段在出网前由 c09 唯一 owner 的脱敏门禁挂载执行（本期不脱敏、不外发、不出网）。
- 写回类：`replaceSelection/insertText/appendSection/insertComment/insertCitation/applyStyle/createNewDocument/createPresentation/saveDocument`。统一入参带 `expectedRevision` 与 `writebackSource`（=`ai_writeback`/`translation`/`template`…）；执行前做权限+冲突校验（D6），执行后写 `document_events`/`audit_logs`。本期方法落地为「可被调用的能力」，但不内嵌确认弹窗（确认在 c05）。
- 面板控制类：`openAIPanel/closeAIPanel/runAIPanelSkill/streamContentToEditor`；`runAIPanelSkill` 在本期为「技能调度占位」（技能实现归 c05），`streamContentToEditor` 提供流式插入通道但同样受写回校验约束。

**备选与取舍**：

- 备选 A：仅 postMessage，不用官方插件。被否——读取全文/大纲、插入批注/引用、应用样式等深度操作必须经 DS 文档对象模型，纯 postMessage 拿不到文档内部结构。
- 备选 B：纯服务端改文档（DS 不在环内，后端用 docx 库改 OOXML 再让编辑器 reload）。被否——无法做「选区/当前段落/光标位置」级操作，且与「所见即所得 + 用户确认写回」体验冲突，丢失协同上下文。
- 选三段式：插件保证文档内能力深度，postMessage+宿主桥保证安全边界与统一 SDK，面板与编辑器解耦，便于 c05 在其上加确认 UI。

**离线/私有化降级**：插件与宿主 SDK 全部本地打包随 DS/前端发布，无外呼；无公网不影响 Bridge 读写本身（AI 推理的降级由下游模型 phase 负责）。

### D4. 文件类型路由与不可编辑类型的预览/解析入口

**决策**：业务服务按扩展名+MIME 路由：docx/doc→Document Editor，xlsx/xls→Spreadsheet，pptx/ppt→Presentation，pdf→PDF Editor/Viewer（预览+AI 处理入口），ofd→服务端转 PDF 后只读预览，png/jpg→图片预览组件+「文档视觉解析」入口（投递解析任务，实现归 c03）。doc/xls/ppt 旧格式在打开链路中由 DS 转换能力归一到新格式处理。

**备选与取舍**：

- 备选：ofd 直接接第三方 OFD SDK 在线编辑。被否——V1.0 §9.2/§9.5 明确不支持 OFD 原生在线编辑，转 PDF 只读预览最省成本且满足闭环。
- 取舍：旧格式转换在打开时一次性进行，可能增加首次打开耗时；通过缓存转换结果（按 file_hash）规避重复转换，保住 ≤5s 目标（见 D8）。

### D5. 保存回调链路与版本表

**决策**：保存链路严格按 PRD §9.8：

```
ONLYOFFICE Editor → callbackUrl(业务回调端点)
  → 校验(D2 三重) → 按 status 处理
  → status=2/6(可下载最终稿) 时: Document Service 提供下载 URL → 业务网关下载最新文件
  → 计算 file_hash → 写入对象存储(新对象键)
  → 创建 document_versions(version_id, document_id, file_hash, saved_by, saved_at, source)
  → 更新 documents 元数据(current_version, size, updated_at)
  → 产生 document_events(save_new_version/ai_writeback) 供唯一消费方 c03 消费后异步重解析+RAG 索引
  → 写 document_events / audit_logs
```

- 版本字段复用 PRD §10.5/§18：`document_versions(version_id, document_id, file_hash, saved_by, saved_at, source)`，`source ∈ {user_edit, ai_writeback, translation, import, template}`。来源由回调上下文判定：DS 协同保存默认 `user_edit`；若该保存会话由写回类 Bridge（带 `writebackSource`）触发，则记对应来源。
- 触发再解析的事件对齐 §10.6：c02 是 `save_new_version` 与 `ai_writeback` 两类 `document_events` 的唯一产生方——每次保存回调成功建版本时按回调上下文（是否带 `writebackSource`）产生其一；该事件由唯一重解析/索引消费方 c03 消费后异步发起重解析与索引（c03 以 `(document_id, version_id)` 幂等去重），c02 产生事件即完成职责、不在 c03 之外另立第二条解析触发路径。其余触发类型不由 c02 产生：`upload_success`（c01）、`translation_done`（c07）、`template_created`（c08）、`manual_reindex`（产生方=c06、c03 消费）。
- 幂等：以 `document.key`+`file_hash` 去重，DS 重复回调或同内容不产生重复版本。

**备选与取舍**：

- 备选：前端直接上传保存结果。被否——保存权威源是 DS 的 callbackUrl 终稿，前端上传不可信且绕过校验/审计。
- 取舍：以 file_hash 幂等去重，避免「编辑期间多次自动保存」刷出大量空版本，同时保证「真实变更」必然成版本，满足可回滚红线。

### D6. 多人编辑 / AI 写回的冲突处理与版本保留

**决策**（对齐 PRD §9.9）：

- 多人同时编辑：沿用 ONLYOFFICE 实时协同（同一 `document.key` 进同一协同会话），服务端在协同会话「保存终稿」时落版本，不在每次按键落版本。
- AI 写回乐观并发：读取类返回 `revision`；写回类必须带 `expectedRevision`。执行前比对当前文档 `revision`，不一致则拒绝写回并返回「文档已变更，请重新读取上下文」信号（c05 据此提示用户重新拉取）。
- 文档被删除：写回/保存前校验 `documents` 存在且未软删，否则拒绝并提示「文档不存在」。
- 无编辑权限：写回前校验 ACL，「可评论」仅允许 `insertComment` 类（仍可查看、复制文本），「可查看」全部写回拒绝、仅允许查看（含受权限控制的下载），不含复制文本（复制文本属「可评论」及以上专属，对齐 §10.4）。
- 版本保留：任何被接受的写回最终经 D5 落为独立 `document_versions`（含 source 与 file_hash），AI 写回可回溯、可回滚。

**备选与取舍**：

- 备选：悲观锁（AI 写回时锁文档）。被否——与多人协同体验冲突，且 POC 下并发低，乐观并发更轻量、不阻塞他人。
- 取舍：乐观并发在「读取后他人改动」时会让 AI 写回失败一次需重读，属可接受成本，换来无锁、不破坏协同。

### D7. 安全：ACL/租户校验落点、审计与「原文出口」标注

**决策**：

- 校验落点统一在「宿主桥 SDK → 业务服务校验端点」与「回调端点」两处，按 tenant_id / document ACL / role 过滤；写回类先校验后执行。
- 审计落点严格区分两张 c01 拥有的表：
  - 访问类（打开/编辑）与 Bridge 写回类调用（成功或被拒）写 `audit_logs`，仅写 c01 已定义列（操作者/`role`/`tenant_id`/操作类型/对象/时间/`result`/`failure_reason`）；本期不向 `audit_logs` 写入 IP（IP 不在 c01 owned schema 内，c02 非建表方亦不 ALTER）。
  - `document_events` 仅由保存回调链路在「成功产生新版本」时产生 `save_new_version` / `ai_writeback` 两类事件（携带 §10.6 稳定契约字段），不承载打开/编辑等访问类审计、亦不承载被拒/未落盘的写回。
- 原文出口标注：`getFullText/getSelectedText/getCurrentParagraph` 在契约中标注为 PHI 原文出口，返回体预留对接 c09 `redaction-gateway` 的挂载锚点（命名以 c09 契约为准，c02 不实现识别脱敏）；本期不外发、不出网、不调用公网模型，c02 为原文产出源，其原文经 c05 等消费后在 c03 公网出口由 c09 唯一 owner 的脱敏门禁拦截执行。

**备选与取舍**：备选「在前端做 ACL 判断」被否——前端不可信，权限必须服务端裁决。取舍：每次写回多一次服务端往返，但写回属低频高敏操作，安全优先。

### D8. 性能：打开 ≤ 5s、保存回调 ≤ 10s 的达成手段

**决策**：

- 打开 ≤5s：源文件下载用短时效预签名直连对象存储（DS 经业务网关取，路径短）；旧格式/ofd 转换结果按 `file_hash` 缓存复用；编辑器静态资源与字体本地 CDN/反代缓存；config 签发走轻量同步路径。
- 保存回调 ≤10s：回调端点只做「校验+下载+写存储+建版本+更元数据+产生 document_events」同步完成，把「异步解析+RAG 索引」从回调主链剥离——由唯一消费方 c03 消费 c02 产生的事件后异步建解析作业（`document_parse_jobs`），回调即返回；解析状态另由 §21 的 ≤3s 刷新通道呈现（解析实现归 c03）。
- 失败重试：DS 侧回调天然带重试；业务侧对「下载/写存储」瞬时失败做有界重试（指数退避，上限内），仍失败则标记该保存为失败并告警（见 Risks），不无限阻塞回调连接。

**备选与取舍**：备选「回调内同步解析+索引」被否——解析/视觉解析耗时不可控，必然击穿 10s 目标且把医疗解析耦合进保存关键路径。取舍：异步解析使「保存成功」与「索引就绪」解耦，用户可能短暂看到旧索引，由解析状态刷新（≤3s）弥补，符合 §21 分项指标。

### D9. 数据模型复用（PRD §18）

- 写/读：`documents`、`document_versions`、`document_permissions`、`document_events`。
- 触发：产生 `document_events`（`save_new_version`/`ai_writeback`）供 c03 消费；下游 `document_parse_jobs`/`document_chunks`/`embeddings` 由 c03 创建与消费，c02 不直接写解析作业表。
- 审计：`audit_logs`。
- 只消费不改结构：`users`/`tenants`/`roles`/`permissions`（c01 建立）。
- 本期不新建超出 §18 的核心表；回调凭证/open_token 等为运行态短时态数据，落缓存或随版本上下文，不进核心表。

## Risks / Trade-offs

- [DS 直连对象存储/业务库导致越权或数据外泄] → D1 网络隔离，DS 不持密钥，数据面全经业务网关与短时效预签名 URL；DS 仅渲染/协同。
- [JWT 仅证明来自 DS、无法表达文档级 ACL，存在越权打开/写回] → D2 双层校验：DS JWT + 业务侧 open_token/回调凭证 + 服务端 ACL 二次裁决；callbackUrl 凭证一次性、绑定 document.key 防重放。
- [保存回调失败造成版本丢失或用户编辑白做] → D8 有界重试+退避；失败后置该保存为 failed 并告警（写 audit_logs，运维可见），保留 DS 端可重试入口；幂等去重避免重试产生重复版本。
- [AI 写回与他人协同改动冲突，覆盖他人内容] → D6 乐观并发，写回带 expectedRevision，revision 不一致即拒绝并要求重读上下文；最终都落独立版本可回滚。
- [打开/保存击穿性能目标（旧格式转换、解析耗时）] → D8 转换结果按 file_hash 缓存；解析/索引异步剥离回调主链；静态资源本地缓存。
- [Bridge 读取类把 PHI 原文输出给上层 AI，存在出网风险] → 本期不外发不调公网模型；读取类标注原文出口并预留脱敏挂载点，强约束下游（c03/c04/c05）出网前必经 PHI/PII 脱敏，识别不可用时禁公网、切私有化模型。
- [AI 自动落盘绕过人工确认（医疗安全红线）] → 本期 Bridge 只定义写回方法、不内嵌自动落盘；每次写回独立成版本（含 source/file_hash）保证可回滚；确认弹窗在 c05 强制前置于写回。
- [postMessage 跨窗口被恶意页面注入] → 严格 origin 白名单 + 固定消息 schema + 请求 id 配对，宿主桥不暴露任意 eval。
- [无公网环境下集成不可用] → DS/插件/宿主 SDK/字体全量本地化打包，编辑/保存/协同/Bridge 读写均不依赖公网；仅 AI 推理依赖下游模型 phase 的私有化降级。

## Migration Plan

净新增、无破坏性变更（c01 之前无在线编辑器与 Bridge 能力，`openspec/specs/` 为空）。部署步骤：

1. 部署 ONLYOFFICE DS 私有化容器（含 JWT 密钥）→ 验证：内网可加载编辑器、JWT 开启后非签名请求被拒。
2. 业务侧落地 config 签发 + open_token/回调凭证 + 文件下载网关 → 验证：按 ACL 正确推导 permissions，越权打开被拒。
3. 接入四类编辑器路由 + pdf/ofd/图片预览入口 → 验证：docx/xlsx/pptx 可打开编辑保存，pdf 可预览且有 AI 入口（对齐 §24.2）。
4. 落地保存回调链路 + `document_versions` + 产生 `document_events`（供 c03 消费触发 `document_parse_jobs`）→ 验证：保存回调生成版本、含 file_hash/saved_by/source、产生对应事件；回调失败重试+告警可见。
5. 落地三段式 Bridge（插件+postMessage+宿主桥 SDK）+ 读写/面板控制方法 + 写回校验与审计 → 验证：可读当前选区、可将文本写回选区、写回写 audit_logs/document_events。
6. 性能验证：普通 docx/pptx/xlsx 打开 ≤5s、docx 保存回调 ≤10s（对齐 §21）。

回滚策略：本期为独立 phase，回滚=下线 DS 与回调端点、移除编辑器入口；c01 文档中心与对象存储不受影响（文档仍可下载）。已生成的 `document_versions` 为追加数据，回滚不删历史版本。

## Open Questions

- ONLYOFFICE 版本与授权：私有化部署采用社区版还是企业版（协同并发数、部分高级写回 API 可用性）需在部署期确认；若社区版个别 Bridge 写回能力受限，是否以服务端 OOXML 兜底（仅限非选区类操作）。
- `writebackSource` 与协同保存来源判定：当一次协同会话内既有用户手改又有 AI 写回，终稿 source 的归属规则（取最后一次写回来源 vs 标记为 mixed）待 c05 确认确认链路后定稿。
- 回调失败告警渠道：POC 阶段告警落地形式（仅写 audit_logs/运维日志，还是接通知）待与安全/运维红线（c09）对齐。
- `document.key` 生成与失效策略：是否以 `document_id`+`current_version` 组合，确保新版本产生后旧协同会话 key 失效，避免回调串版本——待部署联调验证。
