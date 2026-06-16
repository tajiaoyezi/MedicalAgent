## ADDED Requirements

### Requirement: AIMed 六大模式与数据源约束
AIMed 学术助手 SHALL 提供六大模式：通用问答、深度文献伴读、科研态势分析、循证证据溯源、智能综述生成、学术写作辅助。每个模式 MUST 强制约束其可用数据源与文件入口：通用问答数据源为 PubMed + 上传文件 + 医疗知识库；深度文献伴读仅上传文件且禁止连接 PubMed 并强制上传；科研态势分析仅 PubMed 且禁用文件上传；循证证据溯源仅 PubMed 且禁用文件上传；智能综述生成为 PubMed + 上传文件；学术写作辅助为 PubMed + 上传文件 + 当前文档。系统 MUST NOT 在某模式下检索其禁用的数据源。

#### Scenario: 深度文献伴读仅基于上传文件且不连 PubMed
- **WHEN** 用户处于深度文献伴读模式并对已上传文献提问
- **THEN** 系统仅基于该上传文件检索与作答，MUST NOT 连接 PubMed 或医疗知识库
- **AND** 答案中的关键结论引用全部来自该上传文件

#### Scenario: 科研态势分析仅检索 PubMed
- **WHEN** 用户在科研态势分析模式输入研究领域并发送
- **THEN** 系统仅检索 PubMed（或离线 PubMed 缓存），MUST NOT 使用上传文件或知识库
- **AND** 文件上传入口处于禁用/不可见状态

#### Scenario: 模式禁用的数据源不被检索
- **WHEN** 当前模式数据源约束不包含某数据源
- **THEN** RAG 检索的数据源选择 MUST 排除该数据源，检索结果与引用中不出现该数据源内容

### Requirement: 模式切换规则
切换 AIMed 模式时，系统 SHALL 按规则处理数据源标签、文件列表与输入框内容。深度文献伴读模式 MUST 隐藏「数据源：PubMed」标签，其他模式 MUST 显示该标签。切换到科研态势分析或循证证据溯源时 MUST 清空已上传文件；切换到其他模式 MUST 保留已上传文件。任何模式切换 MUST 强制保留输入框内容。

#### Scenario: 切换到科研态势分析清空文件
- **WHEN** 用户已上传文件后从通用问答切换到科研态势分析
- **THEN** 系统清空已上传文件列表
- **AND** 输入框内容保持不变

#### Scenario: 切换到深度文献伴读隐藏 PubMed 标签
- **WHEN** 用户切换到深度文献伴读模式
- **THEN** 系统隐藏「数据源：PubMed」标签
- **AND** 保留已上传文件与输入框内容

#### Scenario: 模式间切换保留输入框内容
- **WHEN** 用户在输入框输入文本后在任意两个模式间切换
- **THEN** 输入框内容 MUST 原样保留，不被清空

### Requirement: 输入框占位文案
系统 SHALL 为每个模式展示规定的占位文案：通用问答「向 AIMed 提问，或上传文档获取解答。」；深度文献伴读「请上传文献，开启专注的深度解读。」；科研态势分析「请输入研究领域，如"肺癌免疫治疗"，分析其前沿趋势与热点。」；循证证据溯源「请输入一个临床结论或医学问题，我将为您追溯证据。」；智能综述生成「请描述综述主题，如"近 5 年 XX 治疗进展"，或上传参考文献。」；学术写作辅助「请粘贴需要润色 / 扩写的文本，或直接描述您的写作需求。」。

#### Scenario: 占位文案随模式变化
- **WHEN** 用户切换到循证证据溯源模式
- **THEN** 输入框占位文案显示为「请输入一个临床结论或医学问题，我将为您追溯证据。」

### Requirement: 发送按钮状态机
发送按钮的高亮/置灰状态 SHALL 由当前模式、文件上传状态、文件解析状态与输入框内容共同决定。对通用问答 / 智能综述 / 学术写作：未上传文件时输入框为空或纯空格则置灰、有效文本则高亮；已上传文件时若全部解析失败或存在解析中则置灰、存在解析成功则高亮。对深度文献伴读：未上传文件 MUST 置灰；已上传且全部解析失败或存在解析中 MUST 置灰；已上传且有解析成功则高亮。对科研态势分析 / 循证证据溯源：输入框为空或纯空格则置灰、有效文本则高亮。解析中时 MUST 禁止发送。

#### Scenario: 深度文献伴读未上传文件置灰
- **WHEN** 用户处于深度文献伴读模式且未上传任何文件
- **THEN** 发送按钮置灰，无论输入框是否有文本

#### Scenario: 存在解析中文件禁止发送
- **WHEN** 用户已上传文件且存在文件处于解析中状态
- **THEN** 发送按钮置灰，禁止发送

#### Scenario: 通用问答有效文本高亮
- **WHEN** 用户在通用问答模式未上传文件且输入框含非空格有效文本
- **THEN** 发送按钮高亮，可发送

#### Scenario: 纯空格视为空
- **WHEN** 用户在科研态势分析模式输入框仅含空格
- **THEN** 发送按钮置灰

### Requirement: 智能模式匹配与优先级
点击发送时系统 SHALL 触发模式识别，依据关键词映射推荐模式。命中多个候选时 MUST 按优先级排序：深度文献伴读 > 循证证据溯源 > 科研态势分析 > 智能综述生成 > 学术写作辅助。当推荐模式与当前模式不一致时，系统 MUST 仅对推荐模式 Tab 高亮提示并展示该模式的引导文案，MUST NOT 自动强制切换模式。模式识别准确率验收目标 MUST ≥ 85%。各模式的引导文案取值 MUST 与 §8.11.2 一致：深度文献伴读「仅基于该文献逐段深度解读，避免外部信息干扰」；科研态势分析「获取该领域发文趋势、热点与演化图谱」；循证证据溯源「检索并验证临床结论，获取高级别证据」；智能综述生成「基于文献自动生成结构化综述，支持溯源」；学术写作辅助「获得段落生成、润色优化与引用补充支持」。

#### Scenario: 关键词触发推荐高亮但不自动切换
- **WHEN** 用户在当前模式输入含「RCT、Meta 分析」并发送
- **THEN** 系统高亮「循证证据溯源」Tab 作为推荐并展示引导文案「检索并验证临床结论，获取高级别证据」
- **AND** 当前模式保持不变，不自动切换

#### Scenario: 推荐模式展示对应引导文案
- **WHEN** 系统识别并推荐某模式（深度文献伴读/科研态势分析/循证证据溯源/智能综述生成/学术写作辅助）
- **THEN** 该模式 Tab 旁展示 §8.11.2 规定的引导文案：深度文献伴读「仅基于该文献逐段深度解读，避免外部信息干扰」、科研态势分析「获取该领域发文趋势、热点与演化图谱」、循证证据溯源「检索并验证临床结论，获取高级别证据」、智能综述生成「基于文献自动生成结构化综述，支持溯源」、学术写作辅助「获得段落生成、润色优化与引用补充支持」
- **AND** 引导文案随推荐模式变化且与当前模式是否切换无关

#### Scenario: 多模式命中按优先级取最高
- **WHEN** 用户输入同时命中深度文献伴读与学术写作辅助关键词
- **THEN** 系统按优先级推荐深度文献伴读

### Requirement: 复合任务与无关问题处理
当用户输入同时匹配两个或以上模式时，系统 SHALL NOT 自动切换多个模式，MUST 提示分步操作建议并说明「上传的文件和对话内容在兼容模式间会自动保留」。在通用问答模式下，若用户提问非医学/科研问题，系统 MUST 返回规定的拒答文案；在专业模式下若问题不符合当前模式能力，系统 MUST 基于当前模式尽力回答，不自动切换目标 Skill。

#### Scenario: 复合任务提示分步操作
- **WHEN** 用户单条输入同时匹配两个以上模式能力
- **THEN** 系统提示「识别到您可能需要组合使用多个功能。建议分步操作」并列出步骤①②
- **AND** 不自动切换为任何新模式

#### Scenario: 通用问答拒答无关问题
- **WHEN** 用户在通用问答模式提问非医学/科研问题
- **THEN** 系统返回「抱歉，AIMed 专注于医学科研问题，暂不支持该类提问。如需文献 / 科研相关帮助，请切换至对应模式。」

#### Scenario: 专业模式不切换 Skill
- **WHEN** 用户在循证证据溯源模式提出不符合该模式能力的问题
- **THEN** 系统基于当前模式尽力回答，MUST NOT 自动切换到其他 Skill

### Requirement: 文件上传与管理
AIMed 文件上传与管理 SHALL 遵循 §8.6 约束。上传限制（§8.6.3）：单次对话最多上传 10 个文件、单文件大小 MUST NOT 超过 100MB、MUST NOT 支持加密文件、上传后 MUST 自动调 c03 解析、解析中 MUST 禁止发送。文件来源（§8.6.1）MUST 支持本地文件 / 我的文档中心 / 团队文档中心 / 当前在线文档四类。支持格式（§8.6.2）MUST 为 pdf / ofd / doc / docx / xlsx / xls / ppt / pptx / png / jpg；OFD V1.0 仅支持上传 / 转换 / 预览 / 文档视觉解析 / 问答 / 翻译，MUST NOT 支持原生在线编辑。文件状态（§8.6.4）MUST 为五态：上传中 / 解析中 / 解析成功 / 解析失败 / 已删除；文件展示字段 MUST 含文件名 / 文件类型 / 文件大小 / 解析状态 / 上传时间 / 文件来源。异常提示文案（§8.6.5）MUST 与 PRD 一致：格式不支持「文件类型支持：pdf / ofd / doc / docx / xlsx / xls / ppt / pptx / png / jpg」；文件超限「所选文件中存在超过 100MB 的文件，已自动去除」；批量上传超量「一次最多上传 10 个文件」；解析失败「文件解析失败，可移除后重新上传」；文件已删除「该文件已删除，无法继续作为上下文使用」。AIMed 文件上传入口属 §19.4 上传时 PHI/PII 识别时点：文件在持久化入库或送模型解析前 MUST 经 c09 security-compliance 的上传时 PHI/PII 识别契约（owner=c09，本能力为消费方，不自行实现识别脱敏）按策略处理——识别并提示 / 脱敏后送模型 / 命中且策略=阻止上传时拒绝入库；c04 仅消费该门禁判定并据策略放行或拒绝，识别命中与策略留痕由 c09 统一写入，本能力调用/审计记录写 audit_logs（端到端随 c09 落地验收）。

AIMed 对话内「本地文件」上传 MUST 在持久化时经 c01 文档中心上传服务落为 `documents` / `document_versions` 并获得 `document_id`（深度文献伴读模式的可检索数据源与「上传文档第 N 页」引用定位的唯一 `document_id` 来源）；该上传入口 MUST 经 c01 文档中心上传服务产生 §10.6 `upload_success` `document_event`（产生方=c01、消费方=c03，对齐 c01/c02 的 upload_success/save_new_version 契约），由 c03 消费 `upload_success` 完成解析 → chunk → 索引就绪，再由本能力（rag-retrieval）作为索引就绪事件消费方构建检索索引，使上传论文进入「上传 → 解析 → 带引用回答」主闭环。本能力 MUST NOT 仅把上传文件存入 `conversations.uploaded_files` JSON 而绕过文档持久化与 `upload_success` 事件，本能力自身不直接产生 document_events、仅复用 c01 上传服务触发；`citations` 的 upload 来源定位指针 MUST 绑定该真实 `document_id`。

#### Scenario: 上传文件经 c09 上传时 PHI 识别按策略处理
- **WHEN** 用户向 AIMed 上传文件且文件内容命中 PHI/PII（如姓名/住院号）
- **THEN** 系统在持久化入库或送 c03 解析前 MUST 先经 c09 上传时 PHI/PII 识别契约判定，并按策略「识别并提示 / 脱敏后送模型 / 阻止上传」处理
- **AND** 当策略为阻止上传且命中敏感信息时，MUST 拒绝该文件入库并提示原因，识别脱敏由 c09 实现、本能力仅消费判定

#### Scenario: 批量上传超过 10 个文件
- **WHEN** 用户单次对话上传的文件数量超过 10 个
- **THEN** 系统提示「一次最多上传 10 个文件」
- **AND** 仅保留不超过 10 个文件

#### Scenario: 单文件超过 100MB 被去除
- **WHEN** 用户所选文件中存在大小超过 100MB 的文件
- **THEN** 系统提示「所选文件中存在超过 100MB 的文件，已自动去除」并自动去除该文件

#### Scenario: 格式不支持
- **WHEN** 用户上传不在白名单（pdf/ofd/doc/docx/xlsx/xls/ppt/pptx/png/jpg）内的文件
- **THEN** 系统提示「文件类型支持：pdf / ofd / doc / docx / xlsx / xls / ppt / pptx / png / jpg」

#### Scenario: 加密文件被拒
- **WHEN** 用户上传加密文件
- **THEN** 系统拒绝该文件，MUST NOT 接受加密文件上传

#### Scenario: 本地上传文件落库为 documents 并触发解析索引主闭环
- **WHEN** 用户在 AIMed 对话内上传本地论文（如 PDF/DOCX）且经上传时 PHI 门禁放行
- **THEN** 系统 MUST 经 c01 文档中心上传服务将该文件落为 `documents` / `document_versions` 并获得 `document_id`，由该上传入口产生 §10.6 `upload_success` `document_event`（产生方=c01、消费方=c03）
- **AND** c03 消费该 `upload_success` 完成解析 → chunk → 索引就绪，本能力据索引就绪事件构建检索索引使该论文可被 RAG 检索，且 `citations` 的 upload 来源以该真实 `document_id` 定位「上传文档第 N 页」

#### Scenario: 上传后自动解析且解析中禁止发送
- **WHEN** 文件上传成功后进入自动解析，存在文件处于解析中状态
- **THEN** 系统自动调 c03 解析该文件，且在解析期间发送按钮置灰禁止发送

#### Scenario: 解析失败提示
- **WHEN** 文件解析失败
- **THEN** 系统将该文件标记为「解析失败」状态并提示「文件解析失败，可移除后重新上传」

#### Scenario: 引用已删除文件
- **WHEN** 用户继续以一个已删除文件作为上下文提问
- **THEN** 系统提示「该文件已删除，无法继续作为上下文使用」

#### Scenario: 文件状态与展示字段
- **WHEN** 系统展示已上传文件列表
- **THEN** 每个文件展示文件名/文件类型/文件大小/解析状态/上传时间/文件来源，解析状态取值为上传中/解析中/解析成功/解析失败/已删除之一

### Requirement: 答案生成过程与结构
答案生成期间，系统 SHALL 依次展示进度提示：正在理解问题 / 正在检索内容 / 正在分析证据 / 正在生成回答。检索结束 MUST 展示「找到 {N} 篇相关资料，{M} 篇重点参考」与「思考 {S} 秒」；未检索到资料 MUST 提示「未找到相关文献，建议调整提问关键词。」并不输出无依据的诊疗建议。答案 MUST 分点/分章节呈现、保留标题层级、关键结论带引用角标、末尾展示结构化参考资料。每条医学回答 MUST 展示医疗免责声明（§24.7 第一项）。除免责声明外，AIMed 生成内容 MUST 按 c09 security-compliance 的草稿/辅助建议标记契约（owner=c09，本能力为消费方）标记为草稿/辅助建议（§24.7 第二项、§19.2 系统定位为草稿辅助），该标记随答案展示与保存/写回保留，端到端随 c09 验收。

#### Scenario: 未检索到资料的提示
- **WHEN** 检索阶段未命中任何相关资料
- **THEN** 系统提示「未找到相关文献，建议调整提问关键词。」
- **AND** 不输出无文献依据的诊疗建议

#### Scenario: 关键结论带引用角标
- **WHEN** 系统基于检索到的文献生成答案
- **THEN** 答案分点/分章节呈现，关键结论后附引用角标如 [1][2]，末尾列出结构化参考资料
- **AND** 答案底部展示医疗免责声明，并按 c09 草稿/辅助建议标记契约将生成内容标记为草稿/辅助建议（owner=c09，本能力为消费方）

#### Scenario: 检索完成展示统计
- **WHEN** 检索阶段结束
- **THEN** 系统展示「找到 N 篇相关资料，M 篇重点参考」与「思考 S 秒」

### Requirement: AIMed 高风险答案下发前的人工确认（消费 c05 高风险确认链路，subject=message）
AIMed 答案落 c04 所建 `conversations`/`messages` 表（`module=aimed`），是 message 级医学文书，其中通用问答 / 循证证据溯源 / 智能综述生成等模式会直接产出用药 / 诊疗 / 医嘱类高风险结论。c05 高风险确认链路的确认键泛化为 `(subject_type, subject_id)`：AIMed 答案与知识库问答（kb_qa）答案 MUST 取 `subject_type=message`、`subject_id=messages.message_id`（由 c04 所建 `conversations`/`messages` 行提供），医学翻译文书则取 `subject_type=translation_job`、`subject_id=translation_jobs.job_id`（由 c07 以 translation_job 为确认 subject，MUST NOT 写 c04 `conversations` 取 message_id）。因此 c04 `conversations.module` 枚举域保持 `{aimed, kb_qa}`（无需也 MUST NOT 新增 translation 值）。AIMed 答案在下发给用户前，当命中高风险（诊疗、用药、医嘱、临床文书或患者个体信息）时 MUST 接入 c05 ai-writeback-confirmation 的高风险确认链路并以 `subject_type=message`、`subject_id=message_id` 为键，与知识库问答（kb_qa，同为 subject=message）、医学翻译文书（subject=translation_job）复用同一条 `writeback_confirmations` 表（其 subject 列承载 document_id/message_id/translation_job 多态）。`risk_type` 高风险判定与 `confirmed_role` 角色裁决的唯一 owner 为 c05 服务端，本能力 MUST NOT 自建高风险判定或确认记录，仅作为该链路的 message 级生产方前置消费：本能力 SHALL 在 AIMed 答案下发前将待下发内容交由 c05 服务端 `risk_type` 分类器判定。命中高风险时，确认 MUST 按 `confirmed_role∈{doctor,reviewer}` 裁决并以 `(subject_type=message, subject_id=message_id)` 为键落 c05 所建 `writeback_confirmations`，普通用户只能生成草稿或提交审核、MUST NOT 完成最终确认与下发；具备 `doctor` 或 `reviewer` 角色者方可确认下发。确认记录与审计由 c05 owner 写入，本能力仅触发该链路并记录答案行为到 `audit_logs`（§19.2、§24.7 第三项）。

#### Scenario: 高风险 AIMed 答案需医生或审核角色确认后下发
- **WHEN** AIMed 生成的答案被 c05 服务端 `risk_type` 分类器识别为高风险（命中诊疗/用药/医嘱/临床文书/患者个体信息）
- **THEN** 系统 MUST 在答案下发前进入 c05 高风险确认链路并以 `(subject_type=message, subject_id=message_id)` 为键，按 `confirmed_role∈{doctor,reviewer}` 裁决
- **AND** 普通用户 MUST NOT 完成最终确认与下发，仅能生成草稿或提交审核

#### Scenario: 高风险判定与确认记录归 c05、c04 仅前置消费
- **WHEN** AIMed 答案下发前接入高风险确认链路
- **THEN** `risk_type` 判定与 `writeback_confirmations` 确认记录 MUST 由 c05 owner 写入，本能力 MUST NOT 自建判定或确认表
- **AND** 本能力仅触发该链路并将答案行为写入 `audit_logs`，确认键取 `(subject_type=message, subject_id=message_id)`，c07 医学翻译文书改取 `subject_type=translation_job`、不经 c04 `conversations`

#### Scenario: conversations.module 枚举不含翻译取值
- **WHEN** 检视 c04 作为 `conversations.module` 唯一 owner 的枚举域
- **THEN** `module` 枚举 MUST 保持 `{aimed, kb_qa}`，MUST NOT 新增 translation 取值——医学翻译文书的高风险确认以 `subject_type=translation_job` 直接挂 `translation_jobs.job_id`，不向 c04 `conversations`/`messages` 落行取 message_id

### Requirement: 公网模型调用前 PHI/PII 脱敏门禁
AIMed 在调用公网模型或公网 PubMed 前，系统 SHALL 消费 c09 的 redaction-gateway（PHI/PII 识别与脱敏引擎，唯一 owner=c09）对输入内容做识别与脱敏；本 phase 不自行实现 PHI/PII 识别脱敏，仅在公网出口预留并调用该门禁接缝。当识别失败、脱敏置信度不足、识别服务不可用或 redaction-gateway 未接入时，系统 MUST NOT 调用公网模型/公网 PubMed，MUST 切换至 c03 提供的私有化模型与离线 PubMed 缓存路径（本期默认公网关闭、私有化与离线优先，端到端公网脱敏验收随 c09 落地）。脱敏命中与策略由 c09 redaction-gateway 在公网出口统一写入 privacy_redaction_events，本 phase 仅消费门禁判定、不另维护该表字段口径；本能力对应的调用/审计记录写 audit_logs。

#### Scenario: 脱敏后方可调用公网
- **WHEN** 用户问题含潜在 PHI/PII 且公网可用
- **THEN** 系统先经 c09 redaction-gateway 完成识别与脱敏，再以脱敏后内容调用公网模型/PubMed
- **AND** 脱敏事件由该门禁写入 privacy_redaction_events

#### Scenario: 识别服务不可用禁用公网
- **WHEN** PHI/PII 识别服务不可用或脱敏置信度不足
- **THEN** 系统禁止调用公网模型/公网 PubMed
- **AND** 自动切换到私有化模型与离线 PubMed 缓存继续作答

### Requirement: 答案操作栏
答案操作栏 SHALL 提供左侧操作（复制回答、保存为、分享、赞、踩、重新生成、删除）与右侧操作（生成在线 Word、生成 PDF、打开到 ONLYOFFICE）。复制回答 MUST 提示「复制成功」；保存为 MUST 支持保存范围（当前回答/当前对话/全部对话）与格式（在线文档/Word/PDF/Markdown），路径为「我的文档中心 / 应用 / AIMed 学术助手 / 保存内容」，命名规则「yyyymmdd_对话名称」；分享 MUST 支持复制链接、生成图片、下载图片、关闭分享弹窗四个子动作（§8.10.4）；赞 MUST 高亮图标并记录正反馈到 feedbacks（`subject_type=message`、`subject_id=message_id`、`rating=赞`）（§8.10.5）；踩 MUST 弹出反馈原因并写入 feedbacks（`subject_type=message`、`subject_id=message_id`、`rating=踩`，原因枚举按 §8.10.5 原文固定 7 项：不准确 / 引用错误 / 没有回答问题 / 格式不好 / 内容太少 / 内容太长 / 其他）；重新生成 MUST 保留旧版本供对比；删除 MUST 二次确认并同步更新对话内容、最近任务、历史记录。生成在线 Word MUST 创建 docx、保存到文档中心、在 ONLYOFFICE Document Editor 打开后由 c05 medical-ai-panel 拥有的「文档打开后默认展示医疗 AI 面板」触发自动展开医疗 AI 面板；本能力仅作为打开入口引用该触发并传入新生成文档 `document_id`，MUST NOT 自建/重定义该触发，亦不实现面板渲染本体（默认展示触发与面板本体唯一 owner=c05）。生成在线 Word/在线文档 MUST 经 c01 文档中心服务端创建服务（owner=c01）落 `documents` / `document_versions`（落点「我的文档中心/应用/AIMed 学术助手/保存内容」），该首版入库 MUST 由 c01 文档中心创建入口产生 §10.6 `upload_success` `document_event`（产生方=c01、消费方=c03，与 AIMed 本地上传文件复用同一条 `upload_success` 入库事件、对齐 c01 「`upload_success` 唯一产生方=c01」契约），由 c03 消费 `upload_success` 完成解析 → chunk → 索引就绪，再由本能力（rag-retrieval）据索引就绪事件构建检索索引，使该生成文档可被后续 RAG 检索；本能力 MUST NOT 依赖 c02 编辑器内 `createNewDocument(content, templateId)` 的服务端新建变体、亦 MUST NOT 自发明服务端新建/落版契约，仅在 c01 创建落库与 `upload_success` 入库事件就绪后，再经 c02 打开链路把该 `document_id` 在 ONLYOFFICE 打开；本能力自身不直接产生 document_events、仅复用 c01 创建服务触发 `upload_success`。「生成 PPT 大纲」属 §22.2 V1.1，本期 MUST 仅作禁用/占位入口、MUST NOT 实现生成逻辑（与 c05 §22.2 PPT 排除口径一致）。答案操作栏 MUST 额外提供「翻译/保存后翻译」入口（§13.2 列为医学翻译四入口之一，§8.10 原文操作清单未含、本能力按 §13.2 补挂载并负责该入口的渲染与触发）；点击后 MUST 按 §8.12 分流：选区/短文本翻译走 AIMed 学术写作辅助内联处理，整篇/全文翻译 MUST 分流至 c07 医学翻译模块建 translation_job（本能力发起请求、建任务服务由 c07 提供）。

#### Scenario: 生成在线 Word 并打开 ONLYOFFICE
- **WHEN** 用户点击「生成在线 Word」
- **THEN** 系统经 c01 文档中心服务端创建服务将 docx 落为 `documents` / `document_versions` 并获得 `document_id`，由 c01 创建入口产生 §10.6 `upload_success` `document_event`（产生方=c01、消费方=c03），随后作为打开入口经 c02 在 ONLYOFFICE Document Editor 打开该 `document_id`，并引用 c05 拥有的默认展示触发（传入该 `document_id`）由 c05 自动展开医疗 AI 面板，本能力 MUST NOT 自建该触发、MUST NOT 依赖 c02 `createNewDocument` 服务端新建变体
- **AND** c03 消费该 `upload_success` 完成解析 → chunk → 索引就绪，本能力据索引就绪事件构建检索索引使该生成文档可被后续 RAG 检索（生成 → c01 创建落库 → `upload_success` → c03 解析 → 索引就绪 → 可检索链路无孤儿、唯一产生方=c01）

#### Scenario: 分享四子动作
- **WHEN** 用户点击「分享」
- **THEN** 系统弹出分享弹窗，支持复制链接、生成图片、下载图片，并可关闭分享弹窗

#### Scenario: PPT 大纲占位不生成
- **WHEN** 用户在答案操作栏看到「生成 PPT 大纲」入口
- **THEN** 该入口为禁用/占位状态，系统 MUST NOT 在本期执行 PPT 大纲生成逻辑（PPT 生成属 §22.2 V1.1）

#### Scenario: 保存为按规则命名与归档
- **WHEN** 用户在「保存为」中选择保存范围与格式并确认
- **THEN** 系统按「yyyymmdd_对话名称」命名并存入「我的文档中心 / 应用 / AIMed 学术助手 / 保存内容」

#### Scenario: 删除消息二次确认并同步
- **WHEN** 用户点击删除
- **THEN** 系统弹出「是否删除该消息？删除后内容无法恢复。」二次确认，确认后同步更新对话内容、最近任务与历史记录

#### Scenario: 点赞记录正反馈
- **WHEN** 用户点击赞
- **THEN** 系统高亮赞图标并将 `rating=赞` 的正反馈记录写入 feedbacks（`subject_type=message`、`subject_id=message_id`）

#### Scenario: 踩反馈记录原因
- **WHEN** 用户点击踩并从 §8.10.5 固定 7 项原因（不准确 / 引用错误 / 没有回答问题 / 格式不好 / 内容太少 / 内容太长 / 其他）中选择反馈原因
- **THEN** 系统弹出含全部 7 项原因枚举的选择项，并将该负反馈与所选原因记录到 feedbacks（`subject_type=message`、`subject_id=message_id`、`rating=踩`）
- **AND** 原因枚举取值与文案 MUST 与 §8.10.5 原文逐字一致（不准确 / 引用错误 / 没有回答问题 / 格式不好 / 内容太少 / 内容太长 / 其他）

#### Scenario: 答案栏选区/短文本翻译走 AIMed
- **WHEN** 用户在答案操作栏点击「翻译/保存后翻译」且翻译对象为选区/短文本
- **THEN** 系统按 §8.12 在 AIMed 学术写作辅助内联完成翻译，MUST NOT 分流至医学翻译模块

#### Scenario: 答案栏整篇翻译分流 c07 建任务
- **WHEN** 用户在答案操作栏点击「翻译/保存后翻译」且翻译对象为整篇/全文
- **THEN** 系统按 §8.12 将请求分流至 c07 医学翻译模块建 translation_job（本能力发起请求、建任务由 c07 提供），不在 AIMed 内执行全文翻译

### Requirement: feedbacks 多来源反馈表（建表 owner）
作为 §18 单一 `feedbacks` 表的唯一建表 owner，本能力 SHALL 将 `feedbacks` 泛化为可承载多来源反馈的单一表：`subject_type` 枚举 MUST 含 `{message, translation_job}`（区分反馈对象来源），`subject_id` MUST 承载 `message_id`（subject_type=message 时）或 `translation_jobs.job_id`（subject_type=translation_job 时，替代原 `message_id` 非空外键硬约束），`rating` 承载赞/踩或翻译质量评分，`reason` 承载 AIMed 踩原因枚举（§8.10.5 原文 7 项：不准确 / 引用错误 / 没有回答问题 / 格式不好 / 内容太少 / 内容太长 / 其他）∪ 翻译质量反馈维度。本能力（AIMed 赞/踩）按 `subject_type=message`、`subject_id=message_id` 写入；c07 §17.6 翻译质量反馈作为写入侧消费方按 `subject_type=translation_job`、`subject_id=translation_jobs.job_id` 写入本表（c07 仅写入、不建表、不 ALTER 表结构）。所有写入 MUST 按 `tenant_id` 隔离。

#### Scenario: AIMed 反馈按 subject_type=message 写入
- **WHEN** 用户对一条 AIMed 答案点击赞或踩
- **THEN** 系统以 `subject_type=message`、`subject_id=message_id` 写入 `feedbacks`，`rating` 取赞/踩、踩时 `reason` 取 §8.10.5 固定 7 项之一，按 `tenant_id` 隔离

#### Scenario: c07 翻译质量反馈按 subject_type=translation_job 写入
- **WHEN** c07 §17.6 翻译管理后台将一条翻译质量反馈写入本表
- **THEN** 系统接受 `subject_type=translation_job`、`subject_id=translation_jobs.job_id`、`reason` 取翻译质量反馈维度或自由文本 `comment` 的反馈记录，按 `tenant_id` 隔离，写入后可按 tenant 回读核对
- **AND** c07 仅作为写入侧消费方，建表/列契约 owner=c04，c07 MUST NOT 建表或改表结构

### Requirement: AIMed 会话上下文与文档发起
系统 SHALL 支持从在线文档发起 AIMed 会话：接收上层（c05 面板侧 Bridge）传入的文档上下文（当前文档全文/当前选区/文档结构），创建 AIMed 新会话并将该上下文作为会话上下文；当前文档的取数（读取全文/选区/结构）由 c05 面板侧 Bridge 负责，本能力仅负责接收已组装上下文并建会话。学术写作辅助内的短文本/选区翻译归 AIMed 处理，上传完整文档要求翻译全文 MUST 分流至医学翻译模块。AIMed 会话、消息与生成内容 MUST 按 tenant_id/user_id 隔离。AIMed 保存内容（保存为/生成在线 Word）成功后 MUST 在 c01 所建 recent_tasks 写入一条 source=「AIMed 学术助手」（c01 recent-tasks-shell 定义的 §6.4 规范枚举值）、ref_type=conversation、ref_id=conversation_id、按 tenant_id/user_id 隔离、按 (ref_type,ref_id) 幂等的记录；最近任务的展示规则与恢复编排归 c05 recent-tasks，本能力仅负责 AIMed 侧条目写入。

#### Scenario: 当前文档发起 AIMed 会话
- **WHEN** 用户在 ONLYOFFICE 文档内点击医疗 AI 面板的「AIMed 学术助手」
- **THEN** 系统接收 c05 面板侧 Bridge 传入的已组装文档上下文（全文/选区/结构），创建新会话并将该上下文作为会话上下文

#### Scenario: 全文翻译分流至医学翻译模块
- **WHEN** 用户在 AIMed 上传完整文档并要求翻译全文
- **THEN** 系统将该任务分流至医学翻译模块，而非在 AIMed 内执行全文翻译

#### Scenario: 会话按租户与用户隔离
- **WHEN** 系统读写 AIMed 会话与消息
- **THEN** 数据按 tenant_id/user_id 过滤隔离，用户 MUST NOT 访问其它租户或他人会话

### Requirement: 会话基座 module/source 维供 kb_qa 复用
作为 conversations/messages 的唯一建表 owner，本能力 SHALL 在 conversations 上提供 module（枚举域 `{aimed, kb_qa}`）与 source（来源规范值）两个区分维度，使 c06 医疗知识库问答会话可复用同一会话/消息基座写入，而无需另建会话表。AIMed 六大模式会话 module MUST 取值 aimed、source MUST 取「AIMed 学术助手」；c06 知识库问答会话经本接口写入时 module MUST 取值 kb_qa、source MUST 取「医疗知识库问答」。`module` 枚举域 MUST 保持 `{aimed, kb_qa}` 两值、MUST NOT 新增 translation 取值：医学翻译文书不经本会话基座（c07 不向 c04 conversations/messages 落行取 message_id），其高风险确认以 `subject_type=translation_job` 直接挂 `translation_jobs.job_id`、其最近任务恢复经 recent_tasks `ref_type=translation_job` 路由，均不依赖 c04 `conversations.module`。读写 MUST 按 tenant_id/user_id 隔离，c06 据此接口持久化会话并自行向 recent_tasks 写 source=「医疗知识库问答」条目（c06 spec 声明），c05 恢复编排 MUST 能按 module 区分 aimed 与 kb_qa 会话。

#### Scenario: AIMed 会话标记 module=aimed
- **WHEN** 用户创建一次 AIMed 学术助手会话
- **THEN** 系统在 conversations 落 module=aimed、source=「AIMed 学术助手」的会话记录

#### Scenario: kb_qa 会话经本接口写入标记 module=kb_qa
- **WHEN** c06 医疗知识库问答经本会话基座接口写入会话
- **THEN** 系统接受 module=kb_qa、source=「医疗知识库问答」的会话记录，按 tenant_id/user_id 隔离，c05 恢复编排可按 module 区分该会话与 AIMed 会话

#### Scenario: 保存内容写入最近任务
- **WHEN** 用户保存 AIMed 答案（保存为/生成在线 Word）成功
- **THEN** 系统在 recent_tasks 落一条 source=「AIMed 学术助手」、ref_type=conversation、ref_id=conversation_id、按 tenant_id/user_id 隔离、按 (ref_type,ref_id) 幂等的记录
- **AND** 最近任务的展示与恢复由 c05 负责，本能力仅写入条目
