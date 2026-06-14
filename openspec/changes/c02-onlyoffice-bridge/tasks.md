## 1. ONLYOFFICE Docs 私有化部署与网络隔离

- [ ] 1.1 以容器方式私有化部署 ONLYOFFICE Document Server（DS）于内网，全量本地化镜像（字体、插件本地打包），不依赖任何公网外呼；验证：断网环境下内网可加载编辑器静态资源（对应 design D1 离线/私有化降级路径）
- [ ] 1.2 配置 DS 与业务服务（BFF/文档网关）的双网络区隔离：DS 仅暴露编辑器静态资源与自身端点给前端，经受控 callbackUrl 与文件下载 URL 反向访问业务网关，不直连对象存储与业务库；验证：DS 容器内无对象存储/DB 凭证，DS 取源文件只能经业务网关签发的短时效 URL（spec onlyoffice-integration「打开与编辑受文档 ACL 与租户隔离约束」、design D1）
- [ ] 1.3 启用 DS 的 JWT（inbox/outbox/浏览器三处），由业务服务用共享密钥签发并由 DS 校验；验证：JWT 开启后非签名的初始化/回调请求被 DS 拒绝（design D2、Migration 步骤 1）
- [ ] 1.4 提供「真实接入连通性测试」脚本/用例：内网拉起 DS 后用一个样例 docx 完成加载-编辑-保存回调全链路自检；同时提供「离线/私有化降级演示」：在完全断网环境复跑同一用例并通过（design D1/D8 失败重试、Migration 步骤 1）

## 2. 编辑器配置签发与打开鉴权

- [ ] 2.1 实现「打开文档」端点：校验登录态 + 按 tenant_id / role / 文档级 document_permissions 校验 document ACL，通过后签发短时效一次性 open_token 并据此生成 DS 初始化 config（document.key、document.url、editorConfig.callbackUrl、permissions）；验证：合法用户可取得 config（spec onlyoffice-integration「打开 docx 进入文字编辑器」、design D2、§24.2「docx 可在线打开」）
- [ ] 2.2 由 ACL 严格推导并注入 ONLYOFFICE editorConfig 的 permissions：permissions.edit（「可编辑」及以上→edit 模式可写回，否则 false）、permissions.comment（「可评论」→评论/审阅模式 true、可插批注不可改正文、不注入改正文 callbackUrl，「可查看」→false）、permissions.copy（「可评论」及以上→true、「可查看」→false，落实 §10.4「复制文本属可评论及以上专属」）；「可查看」→只读且不注入可写 callbackUrl；验证：仅查看权限用户以 view 模式加载、保存与写回被禁用且 permissions.copy=false 复制被禁用，「可评论」用户以评论模式（permissions.comment=true）加载、permissions.copy=true 复制可用（spec onlyoffice-integration「仅查看权限以只读模式打开」「可评论权限以评论模式打开」「复制文本能力按 ACL 注入 permissions.copy」、design D2、§10.4）
- [ ] 2.3 实现跨租户拦截：打开目标文档 tenant_id 与调用者不一致时拒绝、不生成编辑器配置并写一条审计；验证：跨租户打开被拒并记审计（spec onlyoffice-integration「跨租户访问被拒绝」、design D7）
- [ ] 2.4 实现文件下载网关：签发与 open_token 绑定的短时效预签名/一次性下载 URL 供 DS 取源文件，过期即失效；验证：DS 凭过期 URL 取文件失败（design D2/D8）

## 3. 文件类型路由与不可编辑类型预览/解析入口

- [ ] 3.1 实现按扩展名+MIME 的编辑器路由：docx/doc→Document Editor（documentType=word）、xlsx/xls→Spreadsheet（cell）、pptx/ppt→Presentation（slide），doc/xls/ppt 旧格式经 DS 转换归一；验证：三类文档各自路由到正确编辑器（spec onlyoffice-integration「打开 xlsx 与 pptx 路由到正确编辑器」、§22.1 三个 Editor 集成、§24.2 三类可打开编辑保存）
- [ ] 3.2 对未支持在线编辑的扩展名（如 zip）拒绝打开并提示「不支持的文件类型」，不生成编辑器配置、不静默回退到任意编辑器；验证：打开不支持类型被明确拒绝（spec onlyoffice-integration「不支持的文件类型被拒绝」）
- [ ] 3.3 实现 pdf→PDF Editor/Viewer 预览并提供「发起 AIMed」「发起医学翻译」AI 处理入口，保留页码信息供下游引用按页定位（误差 ≤ 1 页），不因不支持的编辑操作损坏原文件；验证：pdf 可预览、有 AI 入口、返回准确当前页（spec onlyoffice-integration「PDF 可预览且可发起 AI 处理」「PDF 页码信息保留以供溯源」、§24.2「pdf 可预览并支持 AI 处理」）
- [ ] 3.4 实现 ofd→服务端转 PDF 只读预览，界面标识「只读预览（OFD 转 PDF）」，V1.0 拒绝进入 OFD 原生在线编辑并提示改为转 PDF 处理；验证：ofd 以只读 PDF 视图展示、尝试编辑被拒（spec onlyoffice-integration「OFD 以转 PDF 方式只读预览」「OFD 不提供原生在线编辑」）
- [ ] 3.5 实现 png/jpg→图片预览组件并提供「文档视觉解析」入口：图片视觉解析复用 c01 上传产生的 `upload_success` → c03 事件驱动消费自动创建并执行 `document_parse_jobs`（owner=c03），本入口职责终点是按 image `document_id` 读取并展示/刷新 c03 既有解析作业的状态/结果（刷新 ≤ 3 秒），不把图片当作可编辑 Office 文档加载、不向 c03 另行投递任务、不自造按需解析触发接口；验证：图片可预览、点击「视觉解析」可展示/刷新由 upload_success 触发的解析作业状态且 ≤3s 刷新（spec onlyoffice-integration「图片可预览并提供视觉解析入口」「展示并刷新已投递的视觉解析状态」、§22.1 文档视觉解析服务入口）

## 4. 保存回调链路与 document_versions

- [ ] 4.1 实现保存回调端点的三重校验：先验 DS outbox JWT、再验一次性回调凭证、再核对 document.key 与当前会话一致，三者皆过才接受存盘；验证：伪造/重放回调被拒（spec save-callback-versioning、design D2/D5）
- [ ] 4.2 实现回调主链同步处理：按 status（可下载终稿）经业务网关下载最新文件 → 计算 file_hash → 写入对象存储（新对象键）→ 创建 document_versions → 更新 documents 元数据（current_version、size、updated_at）；验证：编辑保存后生成新版本并更新最新版本指针（spec save-callback-versioning「编辑保存生成新版本」、§24.2「保存回调能生成版本」）
- [ ] 4.3 落库后由 c02 按回调上下文（是否带 writebackSource）产生一条 document_events（event_type ∈ {save_new_version, ai_writeback}，携带 document_id/version_id/tenant_id/occurred_at/payload 稳定契约字段）作为重解析/索引触发的唯一职责终点，该事件由唯一消费方 c03 消费后以 (document_id, version_id) 幂等去重异步发起重解析+RAG 索引（c02 不在 c03 之外另立第二条解析触发路径），且保存响应不被解析耗时阻塞、解析状态另由 ≤3s 通道呈现；验证：不带 writebackSource 的保存产生 save_new_version、带 writebackSource 的 AI 写回保存产生 ai_writeback，保存即返回、由 c03 消费事件触发解析且状态置进行中（spec save-callback-versioning「落库后触发异步解析与索引」「保存新版本产生 save_new_version 事件并触发重新索引」「AI 写回保存产生 ai_writeback 事件并触发重新索引」、design D5/D8）
- [ ] 4.4 保证「文件成功落对象存储前不对外宣告保存成功」：下载或写存储失败时不创建 document_version、不返回保存成功并进入失败重试流程；验证：注入落盘失败时无版本产生且不报成功（spec save-callback-versioning「落盘失败不报成功」）
- [ ] 4.5 为每个 document_version 记录 version_id、document_id、file_hash、saved_by、saved_at、source，source 取值于 {user_edit, ai_writeback, translation, import, template}，来源由回调上下文（含 writebackSource）判定；验证：用户编辑产生 user_edit 版本且字段齐全、不同来源版本 source 可区分（spec save-callback-versioning「版本记录文件 hash…」相关三个场景、design D5/D9）

## 5. 保存回调失败重试、幂等与协作版本保留

- [ ] 5.1 实现回调处理失败的有界自动重试（指数退避、上限内），对下载/写存储瞬时失败重试成功后正常生成单一版本；验证：注入瞬时失败时重试后仅产生一条版本（spec save-callback-versioning「回调失败后自动重试成功」、design D8）
- [ ] 5.2 实现重试耗尽后的告警与失败事件记录（写 audit_logs/运维日志），并向用户提示保存未成功；验证：达到最大重试仍失败时触发告警并记录（spec save-callback-versioning「重试耗尽后告警」、design Risks）
- [ ] 5.3 实现以 document.key+file_hash 的幂等去重：DS 重复回调或同内容不产生重复 document_version；验证：相同 file_hash 的重复回调仅保留一条版本（spec save-callback-versioning「重复回调不产生重复版本」、design D5）
- [ ] 5.4 沿用 ONLYOFFICE 实时协同（同一 document.key 进同一协同会话），服务端在协同会话保存终稿时落版本（非每次按键），并写 document_version 与审计；验证：多人协作后保存生成新版本且记录保存人与时间（spec save-callback-versioning「多人协作后保存保留版本」、design D6）
- [ ] 5.5 暴露 ONLYOFFICE 保存回调成功率指标，目标 ≥ 99%；验证：指标可被采集并在演示数据上 ≥ 99%（spec onlyoffice-integration「在线打开与保存的性能约束」、spec save-callback-versioning 失败重试要求）

## 6. 三段式文档 Bridge：读取类方法

- [ ] 6.1 搭建三段式 Bridge 骨架：编辑器内 ONLYOFFICE 官方插件（Asc.scope/callCommand/executeMethod/callModule）+ 受限 postMessage 通道（严格 origin 白名单、固定消息 schema、请求 id 配对、不暴露任意 eval）+ 宿主桥统一 JS SDK；验证：面板经 SDK 可与编辑器插件完成一次请求-响应往返（design D3）
- [ ] 6.2 实现读取类方法 getCurrentDocument/getDocumentId/getDocumentTitle/getDocumentType/getFullText/getSelectedText/getCurrentParagraph/getDocumentOutline/getCurrentPage/getComments/getReferences，统一返回 {ok,data,docKey,revision} 并附溯源定位信息（段落索引/选区 range/页码/大纲层级）；验证：getSelectedText 返回选区文本+range+段落/页码、getFullText+getDocumentOutline 返回全文与带层级大纲（spec document-bridge-api「读取选中文本及其定位」「读取全文与文档大纲」、§24.2「可读取当前选区」）
- [ ] 6.3 保证读取类方法只读、不改文档、不触发保存回调；验证：连续调用任意读取方法后文档内容与版本不变（spec document-bridge-api「读取方法不改动文档」）
- [ ] 6.4 在 getFullText/getSelectedText/getCurrentParagraph 返回体标注「原文出口」并预留对接 c09 redaction-gateway 的挂载锚点占位元数据（命名以 c09 redaction-gateway 契约为准，c02 不出网、本期仅标记不实现识别脱敏），Bridge 自身不调用公网模型、不绕过下游 c09 脱敏门禁外发原文；验证：读取方法仅将原文返回请求方上下文、不外发（spec document-bridge-api「原文出口可被 c09 脱敏门禁拦截」「Bridge 不自行外发原文」、design D3/D7）

## 7. 三段式文档 Bridge：写回类与面板控制方法

- [ ] 7.1 实现写回类方法 replaceSelection/insertText/appendSection/insertComment/insertCitation/applyStyle/createNewDocument/createPresentation/saveDocument，统一入参带 expectedRevision 与 writebackSource，仅定义对编辑器的修改/新建行为，不内嵌确认 UI（确认归 c05）、不让 AI 自动落盘；其中 createNewDocument 仅为「编辑器内新建」（最低权限「可编辑」及以上），MUST NOT 扩展为「无编辑器会话/按文档空间创建权限校验」的服务端净生成变体——AIMed/在线文档 AI 的「服务端生成在线 Word/在线文档」由 c01 文档中心创建服务承担并产生 §10.6 upload_success、经 c03 索引后再经 c02 打开 ONLYOFFICE，c02 不据本方法发明服务端新建契约；createPresentation 本期仅做「方法签名/承载存在且不覆盖原文」验证，不验收 PPT 内容生成（其消费方文档生成 PPT/论文转 PPT 属 §22.2 V1.1，与 c04 design.md PPT 大纲占位、c05 ai-writeback-confirmation「PPT 生成不在 V1.0 范围」口径对齐）；验证：用户确认后 replaceSelection 替换选区并保留可回退原选区信息、insertComment 仅插批注不改正文、insertCitation 插入携带回源元数据的引用、createNewDocument 在编辑会话内新建 docx 不覆盖原文（P0 可验收，且不承担服务端净生成闭环）、createPresentation 仅校验签名承载且不覆盖原文（不验收 PPT 内容生成）（spec document-bridge-api「替换选区写回」「以批注形式插入校对建议」「插入带溯源的引用」「新建文档不覆盖原文」「createPresentation 仅作签名承载且不覆盖原文」、§24.2「可将润色结果写回选区」、§22.2 V1.1）
- [ ] 7.2 写回前向上层提供确认所需数据（原文片段/修改后内容/影响范围：选区/段落/全文），由 c05 承载确认 UI；用户取消则不改动文档与版本；验证：可取得确认数据、取消后文档与版本不变（spec document-bridge-api「写回前提供确认所需数据」「用户取消则不改动文档」）
- [ ] 7.3 写回落盘经 §4 链路生成独立 document_version（source=ai_writeback 且含 file_hash），可据此回滚到写回前版本；验证：应用写回并保存后产生可回滚的 ai_writeback 版本（spec document-bridge-api「写回落盘生成可回滚版本」、spec save-callback-versioning「AI 写回产生 ai_writeback 版本」、§24.2 AI 写回闭环）
- [ ] 7.4 实现面板控制类方法 openAIPanel(command,payload)/closeAIPanel()/runAIPanelSkill(skillName,payload，本期为技能调度占位)/streamContentToEditor(content)：openAIPanel 携初始命令与上下文打开右侧医疗 AI 面板并预置技能上下文，streamContentToEditor 流式呈现且未确认前不构成落盘写回；验证：openAIPanel 打开并预置命令、streamContentToEditor 仅预览不写版本（spec document-bridge-api「打开并预置命令的医疗 AI 面板」「流式内容呈现不等于落盘」、§24.2「右侧医疗 AI 面板可打开」「可从当前文档发起 AIMed/医学翻译」入口承载）

## 8. Bridge 与写回的安全校验、冲突处理与审计

- [ ] 8.1 在宿主桥 SDK→业务校验端点对每次 Bridge 调用做安全校验：token 有效性、目标文档 tenant_id 与 ACL、方法类别最低权限（读取类需「可查看」及以上；写回类中 insertComment 需「可评论」及以上、其余改正文/新建类写回需「可编辑」及以上）；验证：无效/过期 token 被拒不执行任何读写、越权改正文写回被拒（「可查看」用户仅允许查看含受控下载不含复制文本、「可评论」用户仍可查看/复制文本）、「可评论」用户 insertComment 放行而 replaceSelection 被拒、跨租户调用被拒（spec document-bridge-api「无效或过期 token 被拒绝」「越权改正文写回被拒绝」「可评论用户可插批注但不可改正文」「跨租户 Bridge 调用被拒绝」、design D2/D6/D7、§10.4 文档权限表）
- [ ] 8.2 实现 AI 写回乐观并发冲突校验：写回类比对当前文档 revision 与入参 expectedRevision，不一致则拒绝并返回「文档已更新，请重新读取上下文」信号；一致且已确认时正常写回生成 ai_writeback 版本；验证：上下文版本陈旧时被拒、一致时正常写回（spec save-callback-versioning「文档已变更时提示重新读取上下文」「上下文一致时正常写回」、design D6）
- [ ] 8.3 实现写回/保存前的存在性与权限校验：文档已被删除时拒绝并提示「文档不存在」、不创建版本；调用者无「可编辑」权限时禁止改正文/新建类写回（「可查看」用户仅允许查看含受控下载、不含复制文本，「可评论」用户仍可查看/复制文本），「可评论」用户仍可经 insertComment 插入批注（批注类最低权限为「可评论」）；被拒事件写审计；验证：已删除文档与无编辑权限触发改正文写回两种情形均被拒并记审计、「可评论」用户 insertComment 不被该校验拦截（spec save-callback-versioning「文档已删除禁止写回」「无编辑权限禁止改正文写回」、spec document-bridge-api「可评论用户可插批注但不可改正文」、spec onlyoffice-integration ACL 约束、§10.4）
- [ ] 8.4 将打开/编辑等访问类事件与所有写回类调用（成功或被拒）写入 audit_logs，仅写 c01 已定义列（操作者、role、tenant_id、操作类型、对象、时间、result、failure_reason），不写入 IP 等 c01 未定义列；打开/编辑事件 MUST NOT 写 document_events；验证：打开事件仅写 audit_logs（不写 document_events）、写回类调用全部写 audit_logs（spec onlyoffice-integration「打开事件仅写 audit_logs」、spec document-bridge-api「写回类调用全部写 audit_logs」、design D7/D9）

## 9. 性能验收与主验收闭环连通

- [ ] 9.1 落地打开 ≤5s 手段并验证：源文件短时效预签名直连、旧格式/ofd 转换结果按 file_hash 缓存复用、编辑器静态资源与字体本地缓存、config 签发走轻量同步路径；验证：普通 docx/pptx/xlsx 从点击到可交互 ≤ 5 秒（spec onlyoffice-integration「普通文档在 5 秒内打开」、§21、design D8）
- [ ] 9.2 验证保存回调 ≤10s：回调主链仅做校验+下载+写存储+建版本+更元数据同步完成、异步解析剥离主链；验证：普通 docx 从回调到新版本落库 ≤ 10 秒并累计一次成功保存回调（spec onlyoffice-integration「普通文档保存回调在 10 秒内完成」、§21）
- [ ] 9.3 串联本期对应的主验收闭环段并做端到端连通性演示：文档中心 → 在线打开（docx/pptx/xlsx/pdf 路由）→ 右侧医疗 AI 面板打开 → 读取当前选区 → 写回选区（经 c05 确认机制承载）→ 保存新版本；验证：覆盖 §24.2 在线 Office 全部验收项；并提供完全离线/私有化环境下复跑同一闭环（AI 推理由下游模型 phase 私有化降级）的演示路径（spec 三份、§24.2、design Migration 步骤 3-6）
