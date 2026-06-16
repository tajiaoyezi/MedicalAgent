## 1. 数据模型与迁移（复用 PRD §18 命名）

- [x] 1.1 新增迁移：创建 `writeback_confirmations` 写回确认记录表（该表与 `risk_type` 分类器唯一 owner=c05，c09 引用式消费做统一验收/审计、不新造独立 confirmation 表），落 §19.2 全字段（`confirmation_id`、subject 多态键 `subject_type` ∈ {document, message, translation_job} + `subject_id`（承载 `document_id` / `messages.message_id` / `translation_jobs.job_id`，与 §19.2「document_id / message_id」对齐并泛化覆盖译文文书）、`confirmed_by`、`confirmed_role`、`confirmed_at`、`confirmed_scope`、`risk_type`、`before_content_hash`、`after_content_hash`、`confirmation_action`、`audit_log_id`、`tenant_id`），并补 doc_ai §6.6 恢复载体列 `operation_type`（AI 操作类型枚举：全文润色/选区润色/校对/选区翻译/补引用/插入标注/AI 论文排版）与 `output_version_id`（指向写回所落 `document_versions`），`confirmed_role` 取值可枚举为 {doctor, reviewer}；验证：迁移可正向/回滚执行且字段与 §19.2 一一对齐、(`subject_type`,`subject_id`) 多态键可承载 document/message/translation_job 三类回源、`operation_type`/`output_version_id` 可承载 §6.6 doc_ai 恢复内容（对应「确认写回生成完整确认记录」场景）
- [x] 1.2 新增迁移：对 c01 所建最小 `recent_tasks` 表执行 ALTER 补列（`recent_tasks` 唯一建表 owner=c01，本期绝不重复建表），补 D4 所需列 `title_preview`、`status`、`created_at`、`related_document_id`（保留并对齐 c01 既有的 `task_id`/`tenant_id`/`user_id`/`source`/`title`/`ref_type`/`ref_id`/`updated_at`/`deleted_at`），新增 (`ref_type`,`ref_id`) 唯一约束保证幂等 upsert，并对 `updated_at` 建排序索引；验证：ALTER 迁移可正向/回滚、须在 c01 建表迁移之后执行，按 (tenant_id,user_id,updated_at) 可高效倒序查询（对应「六类来源汇聚到统一列表」场景）
- [x] 1.3 确认本期对既有 `recent_tasks` 仅做 ALTER 补列（不重复建表），新建表仅 `writeback_confirmations`；`document_versions`/`audit_logs`/`document_permissions`/`conversations`/`messages`/`citations`/`translation_jobs`/`recent_tasks` 等表均由各自 owner 所建（recent_tasks→c01，文档/审计/权限→c01，会话/消息/引用→c04，翻译→c07），本期仅以外键/指针消费/写入其既有结构；验证：迁移评审记录中本期新建表仅 `writeback_confirmations`，对 `recent_tasks` 仅有 ALTER、无对其它上游表结构的修改

## 2. 写回确认网关（AI 改文档唯一收口）

- [x] 2.1 实现写回确认网关服务骨架，定义统一入参（操作类别、原文上下文、修改后内容、读取时 `revision`/内容哈希、目标 `document_id`、选区/位置定位）与「未确认零写回」状态机；验证：结果生成后、用户未点确认前网关 MUST NOT 调用任何 c02 写回方法（对应「未确认前禁止写回」场景）
- [x] 2.2 实现四要素确认面板前端：逐项呈现原文/修改后/修改说明/影响范围，并提供「应用到文档 / 生成副本 / 取消」三按钮，底部固定展示 §19.3 医疗免责声明；验证：任一面板 AI 操作产生待写回结果时均先弹出四要素面板与三按钮及免责声明（对应「写回前展示四要素与操作按钮」场景）
- [x] 2.3 实现自适应 diff 呈现（D2）：选区类行内字符级 diff、全文类分段并排 diff、批注类建议列表（位置+原片段+建议+理由）、排版类结构/样式变更摘要+关键页预览；验证：四类操作各自按对应粒度呈现且影响范围与策略一致
- [x] 2.4 实现「取消」分支：丢弃待写回结果，文档内容不变且不创建版本或副本；验证：点取消后文档内容、`document_versions`、副本均无新增（对应「用户取消不改动文档」场景）

## 3. 默认写回策略矩阵 → c02 写回方法映射

- [x] 3.1 实现策略矩阵（D3）：选区修改→`replaceSelection`、全文修改→`createNewDocument`、校对→`insertComment`、补引用/插入标注→`insertCitation`/`insertComment`、排版→`applyStyle`/`appendSection`+`saveDocument`、选区短文本翻译→`replaceSelection`，默认值不可被技能静默覆盖；文件级译文副本（`source=translation`）的产出与落库确认归 c07，本矩阵不二次确认；验证：每类「应用到文档」分别命中对应 c02 写回方法且默认不覆盖原文，文件级译文副本不进入本网关二次确认
- [x] 3.2 实现「选区修改→替换选区」写回：用户对选区润色/翻译结果点「应用到文档」时经 `replaceSelection` 写回原选区并带 `expectedRevision`；验证：选区结果写回后仅该选区变更（对应「选区修改替换选区」场景）
- [x] 3.3 实现「全文修改→生成新文档」写回：全文润色点「应用到文档」经 `createNewDocument` 生成新文档，MUST NOT 覆盖原文档；验证：原文档内容与版本不变、新增一份新文档（对应「全文修改生成新文档」场景）
- [x] 3.4 实现「校对→批注插入」写回：校对结果点「应用到文档」经 `insertComment` 批量以批注/建议形式插入，不改写正文；验证：正文文本无改动、批注按定位点插入（对应「校对结果以批注插入」场景）
- [x] 3.5 实现「排版→生成新版本」写回：AI 论文排版结果点「应用到文档」生成新 `document_version` 并保留旧版本支持回滚；验证：旧版本仍可读、新版本 `source`=`ai_writeback`（对应「排版结果生成新版本」场景）
- [x] 3.6 实现「生成副本」分支：任一结果点「生成副本」先 `createNewDocument` 复制原文档再写入结果，原文档保持不变；验证：原文档零变更、副本含写回结果（对应「用户改选生成副本」场景）
- [x] 3.7 实现写回经 c02 保存回调链路统一落独立 `document_versions`（含 `source`/`file_hash`）以保证可回滚；验证：用户撤销一次已应用写回时可基于 `document_versions` 回滚到写回前版本，确认记录与审计日志保留不删（对应「写回可回滚」场景）

## 4. 写回冲突、权限与高风险确认链路

- [x] 4.1 实现乐观并发冲突校验：写回前比对当前文档 `revision`/内容哈希与读取时是否一致，不一致则阻止写回并提示「文档已变更，请重新读取上下文」，MUST NOT 用过期结果覆盖；验证：文档自读取后被修改时本次写回被阻止（对应「文档已变更提示重新读取上下文」场景）
- [x] 4.2 实现保存回调失败自动重试与告警：写回后保存回调失败时自动重试，仍失败则告警并保留待保存内容不丢失，保存回调成功率目标 ≥ 99%；验证：模拟保存回调失败时触发重试且用户已确认结果不丢失（对应「保存回调失败自动重试」场景）
- [x] 4.3 实现写回前文档存在性与编辑权限校验，按 `tenant_id`/`user_id`/`role`/`acl` 过滤 `document_permissions`/ACL：文档被删→禁写回并提示「文档不存在」；验证：对已删除文档点「应用到文档」被拒并提示文档不存在（对应「文档被删除禁止写回」场景）
- [x] 4.4 实现无编辑权限分支：无编辑权限时禁用「应用到文档」，仅允许「生成副本」或查看，越权写回 MUST 被拒；验证：无编辑权限用户只能生成副本/查看（对应「无编辑权限仅允许复制」场景）
- [x] 4.5 实现服务端 `risk_type` 高风险判定（命中诊疗/用药/医嘱/临床文书/患者个体信息）并裁决角色（`risk_type` 分类器唯一 owner=c05，c09 引用式消费、不重复实现）：普通用户对高风险写回仅能「生成草稿/提交审核」，MUST NOT 最终确认；验证：普通用户对高风险内容点「应用到文档」被阻止直接确认（对应「普通用户高风险写回仅能提交审核」场景）
- [x] 4.6 实现授权角色最终确认：具备医生（`doctor`）或授权审核（`reviewer`）角色（角色定义为 c01 auth-rbac 唯一真值，本期引用确定角色名）的用户对高风险写回点确认时允许写回，并记录 `confirmed_by`/`confirmed_role`（取值 ∈ {doctor, reviewer}）/`risk_type`；验证：授权角色确认后写回成功且确认记录含上述字段（对应「授权角色完成最终确认」场景）
- [x] 4.7 实现三类高风险内容产生方（AIMed 答案 c04 / 知识库问答 kb_qa c06 / 医学翻译文书 c07）下发前的高风险拦截与确认：本能力为确认 owner、三者为生产方挂载并前置消费本链路；服务端 `risk_type` 识别为高风险时，在内容下发前进入同一确认链路，普通用户仅能生成草稿/提交审核（MUST NOT 直接下发），`doctor`/`reviewer` 角色确认后方可下发，并以 (`subject_type`, `subject_id`) 多态键落 `writeback_confirmations`（含 `confirmed_by`/`confirmed_role`/`risk_type`/`audit_log_id`）——AIMed 答案 / kb_qa 答案取 `subject_type=message`、`subject_id=messages.message_id`（回源 c04 所建 `conversations`/`messages`），医学翻译文书取 `subject_type=translation_job`、`subject_id=translation_jobs.job_id`（回源 c07 `translation_jobs`，c07 MUST NOT 为取确认键而向 c04 `conversations`/`messages` 写 message 行）；c09 引用式消费做统一验收/审计（收口枚举覆盖该三类、c07 按 `subject_type=translation_job` 核对）；验证：普通用户高风险输出被拦仅能提交审核、授权角色确认后下发并生成以 (`subject_type`,`subject_id`) 为键的确认记录、三类产生方复用同一链路并按 `subject_type` 区分（对应「普通用户高风险 message 级输出仅能提交审核」「授权角色确认后下发并落 subject 多态确认记录」「三类产生方复用同一确认链路并按 subject_type 区分」场景）

## 5. 写回确认记录与审计留痕

- [x] 5.1 实现每次写回确认（应用到文档/生成副本）生成 `writeback_confirmations` 记录，含 §19.2 全字段，其中 `before_content_hash`/`after_content_hash` 分别对应写回前后内容哈希，并写入 doc_ai §6.6 恢复载体字段 `operation_type`（AI 操作类型）、`confirmed_scope`（选区）、`output_version_id`（指向写回所落 `document_versions`，承载输出结果）；验证：完成一次确认后记录字段完整、前后哈希正确，且 `operation_type`/`confirmed_scope`/`output_version_id` 足以还原 §6.6 doc_ai 恢复内容（对应「确认写回生成完整确认记录」场景）
- [x] 5.2 实现确认动作写入 `audit_logs` 并以 `audit_log_id` 反向关联确认记录；验证：每条确认记录可经 `audit_log_id` 关联到对应审计日志（对应「确认写回生成完整确认记录」场景）

## 6. 医疗 AI 面板挂载与三类入口

- [x] 6.1 实现医疗 AI 面板挂载为 c02 宿主页右侧 UI，全程仅调 c02 宿主桥 SDK（`bridge.*`），不直接接触 ONLYOFFICE 插件/postMessage/文档 DOM；验证：面板无任何绕过 c02 的底层读写代码（对应 D1 协作边界）
- [x] 6.2 实现右侧固定图标「医疗 AI」入口：点击经 `openAIPanel` 展示面板，按 `getDocumentType`（docx/pdf/ofd）渲染对应 P0 功能列表，面板顶部展示 §19.3 医疗免责声明；验证：点击右侧图标按文档类型渲染功能并展示免责声明（对应「通过右侧固定图标打开面板」场景）
- [x] 6.3 实现选区浮层入口：选中文本时在选区附近浮层展示「润色/翻译/解释/补引用」四动作，点击任一动作经 `getSelectedText` 取选区文本作上下文；验证：选中文本后浮层出现四动作且以选区文本为上下文（对应「选中文本触发选区浮层」场景）
- [x] 6.4 实现顶部自定义按钮「医疗空间」入口：点击打开面板并将焦点定位到文档级 AI 功能区（全文润色/排版/校对/发起 AIMed/发起医学翻译）；验证：点顶部按钮聚焦文档级功能区（对应「顶部医疗空间按钮入口」场景）
- [x] 6.5 实现关闭面板：点关闭按钮经 `closeAIPanel` 收起面板且不改动文档内容；验证：关闭后文档内容零变更（对应「关闭面板」场景）
- [x] 6.6 实现「文档打开后默认展示医疗 AI 面板」触发（§5.4/§14.6/§14.8，该触发的唯一 owner=c05，c08/c04 等打开入口仅引用、不自建）：文档在 ONLYOFFICE 打开后由本能力拥有的默认展示触发经 `openAIPanel` 自动展示面板，按 `getDocumentType` 渲染对应 P0 功能集并展示 §19.3 免责声明，各打开入口（c08 模板生成文档/c04 等）以新 `document_id` 引用本触发而非重定义；验证：文档打开后默认展示面板、下游以 document_id 引用本能力触发（对应「文档打开默认展示医疗 AI 面板」「下游入口引用本能力的默认展示触发」场景）

## 7. Word 文档 P0 AI 功能集（docx）

- [x] 7.1 实现「全文润色」技能：选风格（更正式/更学术/更简洁/SCI 英文/中文医学论文）后经 `getFullText` 取全文生成草稿，MUST NOT 直接覆盖原文，并将四要素交确认网关，默认策略「全文修改→生成新文档」；验证：全文润色产生草稿并进入确认网关而非覆盖原文（对应「全文润色生成草稿并进入确认」场景）
- [x] 7.2 实现「选区润色」技能：对 `getSelectedText` 文本生成润色结果，用户确认后经 `replaceSelection` 替换选区；验证：选区润色确认后仅替换该选区（对应「选区润色替换选区」场景）
- [x] 7.3 实现「校对」技能：对全文执行错别字/标点/医学术语/单位格式/缩写一致性/中英文空格/统计学表达/常识性文本错误校对（§15.2 八项），以批注/建议（`insertComment`）输出，MUST NOT 改写正文；验证：校对输出为批注且正文不变（对应「校对以批注形式给出」场景）
- [x] 7.4 实现「插入标注/补引用」技能：生成引用/批注/脚注内容，确认后经 `insertCitation`/`insertComment` 写回光标或选区指定位置，标注内容可溯源至来源（标题/出处/段落定位），引用可点击率目标 ≥ 95%；验证：标注写回指定位置且引用可点击溯源（对应「插入标注写回指定位置」场景）
- [x] 7.5 实现「辅助显示」：在面板内展示文档结构/引用/修改建议，MUST NOT 对文档执行任何写回；验证：辅助显示仅在面板内呈现、文档零变更（对应「辅助显示不写回」场景）
- [x] 7.6 实现编辑器排版类操作（目录/更新目录/目录级别/分页/页眉页脚/段落对齐缩进行距）经 c02 编辑器操作（`getDocumentOutline`/`applyStyle` 等）直接执行，不经确认网关；验证：排版动作直接执行且不弹确认网关（对应「编辑器排版类操作直接执行」场景）
- [x] 7.7 确认 docx 面板 MUST NOT 提供论文转 PPT、AI 文档脑图（§22.2）；验证：docx 功能列表不含上述 V1.1 项

## 8. PDF / OFD 文档 P0 AI 功能子集

- [x] 8.1 实现 PDF Viewer 中面板仅渲染 P0 子集（医学翻译/AIMed/批注），支持预览态发起处理，MUST NOT 提供 docx 专属全文润色/排版/目录等写回功能；验证：PDF 面板仅含子集功能、无 docx 写回项（对应「PDF 预览并发起 AI 处理」场景）
- [x] 8.2 实现 OFD 经转 PDF/文本抽取后再发起 AIMed 或医学翻译；验证：对 OFD 发起 AIMed/翻译时先得到可处理文本再执行（对应「OFD 经转换后支持」场景）
- [x] 8.3 实现 OFD 转换/抽取失败处理：提示该 OFD 暂不可处理且不进入公网模型调用；验证：转换/抽取失败时提示「暂不可处理」且无公网调用（对应「OFD 经转换后支持」场景）
- [x] 8.4 确认 PDF/OFD 面板 MUST NOT 提供 AI 文档脑图、文档生成 PPT（§22.2），OFD 原生在线编辑与依赖第三方 SDK 的 OFD 批注 MUST NOT 纳入 V1.0；验证：PDF/OFD 功能列表不含上述项

## 9. 从当前文档发起 AIMed 与医学翻译（面板侧发起）

- [x] 9.1 实现「从当前文档发起 AIMed」：经 `getDocumentId`/`getDocumentTitle`/`getFullText`（或选区）组装上下文，调用 c04 创建会话并附 `tenant_id`/`user_id`，按 `tenant_id`/`kb_id`/`user_id`/`role`/`document_acl`/`chunk_acl` 六维（§11.9，六维过滤由 c04 rag-retrieval 召回前执行、本期消费其结果）过滤可用知识库与检索源，越权来源 MUST NOT 进入上下文，发起 AIMed MUST NOT 直接写回；验证：以当前文档为上下文创建 c04 会话且越权源被过滤（对应「以当前文档为上下文创建会话」场景）
- [x] 9.2 实现 AIMed 面板回答展示带可点击引用定位（定位到来源页码/段落，引用源定位成功率目标 ≥ 90%），回答 MUST NOT 直接写入文档，需写入须经确认网关；验证：回答含可点击引用且不直接写回（对应「AIMed 回答可溯源且不直接写回」场景）
- [x] 9.3 实现「文档内点击医学翻译/翻译全文」按 §8.12 路由至 c07 医学翻译模块，携带 `document_id`、语言方向、术语库发起文件级异步任务（本期仅发起，不落 `translation_jobs`）；文件级译文副本的产出与落库确认归 c07（`source=translation`，生成新版本不覆盖原文，对齐 §9.6「翻译结果→生成译文副本」），本能力 MUST NOT 经 `ai-writeback-confirmation` 对该文件级译文副本二次确认；验证：文档级翻译请求路由至 c07、文件级译文副本由 c07 落库确认而非经本网关二次确认（对应「文档内点击医学翻译路由至翻译模块」「文件级译文副本不经本网关二次确认」场景）
- [x] 9.4 实现选区短文本翻译由面板/AIMed 就地返回译文，不创建文件级翻译任务；验证：选区浮层点「翻译」就地出译文且无 `translation_jobs` 创建（对应「选区短文本翻译由面板就地处理」场景）

## 10. 选区浮层技能就地处理与分流（D5）

- [x] 10.1 实现选区「润色」动作：技能结果经确认网关默认 `replaceSelection`；验证：选区润色经确认后替换选区
- [x] 10.2 实现选区「解释」动作：调 c04 AIMed 就地问答，结果仅在浮层/面板展示，MUST NOT 调任何 c02 写回方法、MUST NOT 进入确认网关、MUST NOT 写回；验证：解释只展示不写回（对应 medical-ai-panel「选区解释只展示不写回」场景）
- [x] 10.3 实现选区「补引用」动作：调 c04 检索带引用来源经确认网关 `insertCitation` 写回光标/选区，引用可溯源可点击；验证：补引用写回指定位置且引用可点击溯源

## 11. 面板公网模型调用脱敏门禁（医疗红线）

- [x] 11.1 实现面板需公网模型操作（润色/校对/翻译/辅助显示/解释/补引用/发起 AIMed，覆盖全部可能含 PHI/PII 的面板动作）在出网前强制前置消费 c09 redaction-gateway 的 PHI/PII 识别与脱敏判定，本期仅落「门禁接缝 + 不绕过」，不实现识别脱敏引擎；验证：识别成功且置信度达标时以脱敏后文本调公网并写 `audit_logs`（关联 `privacy_redaction_events`）留痕（对应「识别通过后脱敏送公网模型」场景；真实判定接入与端到端验收随 c09 落地）
- [x] 11.2 实现识别失败/置信度不足/识别服务（redaction-gateway）不可用时禁止本次公网调用并提示改用私有化模型或取消，MUST NOT 外发任何未脱敏文本；验证：识别失败/服务不可用时无任何明文出网且提示切私有化（对应「识别失败禁止公网调用」场景）
- [x] 11.3 实现 redaction-gateway 未接入时默认关闭公网 provider：本期公网默认关闭，仅私有化/离线路径跑通闭环（§16.4/§24.9），门禁判定不可用即按「识别服务不可用」保守拒绝公网；验证：在未接入 c09 redaction-gateway 的部署上发起需公网操作时一律走私有化/离线、无公网放行（对应「redaction-gateway 未接入时默认关闭公网」场景）

## 12. 最近任务统一聚合与展示

- [x] 12.1 实现最近任务聚合服务：按 `tenant_id`/`user_id` 过滤后将六类来源（`source` 列取 c01 §6.4 中文规范值：AIMed 学术助手 / 医疗知识库问答 / 医疗数字员工 / 医学翻译 / 在线文档 AI 操作 / 模板生成文档，MUST NOT 写入机器短码）汇聚为统一列表，每条标识来源模块，列表 MUST NOT 含其它租户或非授权用户任务；验证：六类来源汇聚为单列表、`source` 列均为中文规范值且无越权条目（对应「六类来源汇聚到统一列表」场景）
- [x] 12.2 实现本期 `doc_ai`（在线文档 AI 操作）来源向 `recent_tasks` 的 upsert 投递（`ref_type`=`writeback_confirmation`、`ref_id`=`writeback_ref` 指向 `writeback_confirmations` 的单次写回确认记录 id，按 `ref_type`+`ref_id` 幂等键、`source` 使用 c01 枚举规范值「在线文档 AI 操作」、`title` 取「AI 操作类型（确认记录 `operation_type`）+ 目标文档名（`document_id` 回源文档名）」组合、目标文档名缺失时回退为仅 AI 操作类型、`title_preview` 取 `title` 前 10 字，按 `tenant_id`/`user_id` 隔离，同一文档的多次不同操作 / 选区按各自 `writeback_ref` 独立成条），并定义其余来源的统一投递契约——写入侧 owner 为产生来源的对应 change：c04→aimed（AIMed 学术助手，`ref_type=conversation`）、**c06→kb_qa（医疗知识库问答，`ref_type=conversation`）**、c07→translation（医学翻译，`ref_type=translation_job`）、c08→template（模板生成文档，`ref_type=document` 回源 `documents`）、c05→doc_ai（`ref_type=writeback_confirmation` 回源 `writeback_confirmations`）；`ref_type` 取值 MUST 与回源表唯一对应、MUST NOT 出现同一 `ref_type` 指向两张源表的过载，任何消费方 MUST NOT 按 `ref_type=document` 直推 `ref_id` 为 `document_id` 去读 doc_ai；数字员工占位不写入；本期不替它们实现写入、亦不重复定义其展示/删除 Scenario（归 c01 列表壳）；验证：一次在线文档 AI 操作产生/更新一条 `ref_type=writeback_confirmation` 的 `recent_tasks` 记录且重复投递幂等，恢复分发器仅凭 `ref_type` 即可分别回源 `writeback_confirmations` 与 `documents`（对应「ref_type 唯一对应回源表」场景）
- [x] 12.3 实现数字员工来源占位：仅展示其历史条目并显示「规划中」，MUST NOT 在本能力内提供创建/运行/编排入口；验证：数字员工条目可见但无创建/运行/编排入口（对应「数字员工来源占位」场景）
- [x] 12.4 复用 c01 recent-tasks-shell 列表壳的 §6.5 展示规则（标题生成与首页前 10 字截断 / 悬浮全标题 / `updated_at` 倒序 / 今天-7天-30天-1年-全部分组 / 按模块多选筛选 / 重命名）于六类来源混合列表，本期不重复实现/不重复定义其 Scenario（唯一真值与可验收 Scenario 归 c01 recent-tasks-shell）；验证：六类来源混合列表上 §6.5 展示行为复用 c01 列表壳同名 Scenario、本 change 无重复展示 Scenario

## 13. 最近任务六类来源恢复

- [x] 13.1 实现 AIMed 来源恢复（「查看」/「继续追问」）：经 `ref_id`=`conversation_id` 回 c04 取问答记录/模式/上传文件/知识库选择/引用资料/Agent 状态并可续聊，引用资料可点击定位（引用源定位成功率目标 ≥ 90%），最近任务恢复成功率目标 ≥ 98%；验证：AIMed 任务可恢复并续聊、引用可点击（对应「恢复 AIMed 任务」场景）
- [x] 13.2 实现在线文档 AI 来源恢复（「查看」）：经 `ref_type`=`writeback_confirmation`、`ref_id`=`writeback_ref`（指向 `writeback_confirmations` 的单次操作记录、非裸 `document_id`，按操作粒度区分）回源，从该确认记录的非哈希字段恢复文档 ID（`document_id`）/选区（`confirmed_scope`）/AI 操作类型（`operation_type`）/输出结果（`output_version_id` 指向的 `document_versions`）/写回记录（确认记录本体）并可重新定位到对应选区，同一文档名下多次不同操作各自按其 `writeback_ref` 逐次恢复、MUST NOT 因 document_id 相同而互相覆盖；验证：在线文档 AI 任务可按操作粒度由非哈希字段恢复并定位选区/回看写回（对应「恢复在线文档 AI 操作」场景）
- [x] 13.3 实现医学翻译来源恢复（「查看」）：经 `ref_id`=`job_id` 回 c07 恢复原文文件/译文文件/语言方向/术语库/翻译进度/历史版本；验证：医学翻译任务可恢复上述内容（对应「恢复医学翻译任务」场景）
- [x] 13.6 实现医疗知识库问答来源恢复（「查看」/「继续追问」）：经 `ref_id`=`conversation_id` 消费 c06 知识库问答会话恢复问答记录/知识库选择/检索源/引用段落，本期仅负责恢复编排、详情数据由 c06 保证；验证：kb_qa 任务可恢复并续聊、检索源经权限过滤（对应「恢复医疗知识库问答任务」场景）
- [x] 13.7 实现模板生成来源恢复（「查看」）：经 `ref_id`=`document_id`（c08 产物）恢复模板 ID/生成文档/使用时间/编辑状态并打开生成文档，本期仅负责恢复编排、详情数据由 c08 保证；验证：模板生成任务可恢复并打开生成文档（对应「恢复模板生成任务」场景）
- [x] 13.8 实现「继续追问」操作的来源边界：仅会话类来源（AIMed/kb_qa）展示并可用「继续追问」，非会话来源（doc_ai/translation/template/digital_agent 占位）不展示或禁用该入口；验证：对非会话来源任务列表项无「继续追问」入口、对会话来源可续聊（对应「非会话来源不提供继续追问」「会话来源可继续追问」场景）
- [x] 13.4 实现恢复时按 `tenant_id`/`kb_id`/`user_id`/`role`/`document_acl`/`chunk_acl` 六维（§11.9，六维过滤由 c04 rag-retrieval / c06 召回前执行、本期消费其结果）过滤检索源与引用段落，越权来源（含 chunk_acl 严于文档级的不可见 chunk）MUST NOT 在恢复内容中展示；验证：恢复含越权检索源/越权 chunk 时被过滤不展示（对应「恢复时按权限过滤来源」场景）
- [x] 13.5 实现恢复来源失效处理：关联资源（已删文档/翻译文件）不存在时提示该部分不可用并恢复其余可用内容，而非整体失败；验证：部分资源缺失时其余内容仍可恢复（对应「恢复来源已失效」场景）

## 14. 最近任务删除的来源差异（删除规则壳归 c01）

- [x] 14.1 复用 c01 recent-tasks-shell 列表壳的 §6.7 删除规则（二次确认 / 默认不删关联文件 / 仅勾选「同时删除关联文档」才删 / 单条与批量 / 删除审计）于六类来源混合列表，本期不重复实现/不重复定义其 Scenario（唯一真值与可验收 Scenario 归 c01 recent-tasks-shell）；验证：六类来源删除行为复用 c01 列表壳同名 Scenario、本 change 无重复删除 Scenario
- [x] 14.2 实现「同时删除关联文档」时按各来源 `related_document_id` 解析其关联文档对象的差异：doc_ai→写回生成文档/副本、translation→译文副本、template→生成文档；会话来源（aimed/kb_qa）与数字员工占位无独立关联文档；`ref_id` 为空时关联文档解析为空操作，解析得到的文档删除仍交 c01 删除规则执行（经 ACL 校验、软删进回收站、可经 `document_versions` 追溯）；验证：按来源正确解析关联文档对象并交 c01 规则删除（对应「按来源解析关联文档对象」场景）

## 15. 真实接入测试与离线/私有化降级演示

- [ ] 15.1 真实接入测试：面板经 c02 真实 ONLYOFFICE 宿主桥 SDK 完成挂载→读取选区→确认→`replaceSelection` 写回→保存回调落 `document_versions` 全链路；验证：在真实 ONLYOFFICE 部署上选区润色可写回并生成新版本（对应 §24.2 写回验收）。【代码就绪：面板→网关 confirm→`replaceSelection`/`saveDocument` 链路已实现，写回经 c02 已验证的保存回调→`document_versions` 机制；待 `onlyoffice/documentserver` 容器起后人工验证真实 DS 渲染写回，本地环境未起 DS 故未勾选】
- [x] 15.2 真实接入测试：面板发起 AIMed 经 c04 真实会话返回带引用回答、发起文档级医学翻译真实路由至 c07，本期公网出口仅预留脱敏门禁接缝、未接入 c09 时强制走私有化/离线（公网默认关闭）；验证（本期可验证）：真实环境下 AIMed/翻译发起与带引用回答经私有化/离线路径可用、公网默认不放行；验证（随 c09 phase 9 落地完成）：公网模型经真实脱敏门禁放行后调用的端到端验收随 c09 落地（真实判定接入与公网放开随 c09 落地完成，与 11.1/11.3 及 medical-ai-panel 脱敏门禁相位约束一致）
- [x] 15.3 离线/私有化降级演示：在无公网环境下面板挂载、读取、确认网关、写回、最近任务展示/恢复/删除均本地可用，AI 推理经 c03 私有化模型路由，OFD 转换/视觉解析走私有化解析服务，脱敏不可用时禁公网并切私有化模型；验证：断网环境下闭环非推理能力全可用且推理走私有化路径（对应 D1/D5/D7 离线降级）
- [x] 15.4 主验收闭环端到端：润色/校对/翻译选区 → 写回确认四要素 → 确认写回 → 保存新版本 → 最近任务恢复，对齐 §24.2；验证：单次端到端串通选区 AI→确认→新版本→最近任务恢复（对应主验收闭环）
