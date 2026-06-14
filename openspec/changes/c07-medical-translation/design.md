## Context

本 change 为 9 阶段顺序中的第 7 阶段 `medical-translation`，落地 PRD §13 全章与 §22.1 三项 P0 必做：医学翻译文件级异步任务、版式还原、双语对照输出，验收口径见 §24.4。它补齐主验收闭环里「当前文档发起医学翻译」「文档中心 / AIMed / ONLYOFFICE 多入口发起整篇翻译」的能力。

当前状态与约束：

- 上游底座已就绪。c02（`onlyoffice-bridge`）提供四类编辑器、文档 Bridge API（含 `createNewDocument` / `saveDocument` 取当前 `document_id`）与 `save-callback-versioning`（`document_versions.source` 已含 `translation` 取值）；c03（`model-and-parse`）提供 `model-provider-config`（按用途绑定的公网 / 私有化模型路由、优先级与 fallback，含「医学翻译」「术语抽取」用途）、`document-parsing`（文本型解析切分）与 `visual-parsing-service`（扫描 / 图片 / 复杂版式的结构化输出：文本 / 页码 / 段落 / 坐标 / 标题层级 / 表格结构 / 图片位置 / 页眉页脚 / 置信度 / 失败原因 / chunk 定位）。本 change 复用这些能力，不重复实现 provider 抽象与视觉解析。
- 离线优先。部署环境可能无公网，翻译模型与视觉解析都必须有私有化降级路径；c01 不实现 PHI/PII 识别与脱敏（属 c09 security-evidence）。公网调用前的 PHI/PII 识别脱敏引擎 redaction-gateway 及 privacy_detection_rules / privacy_redaction_events 由 c09 唯一实现，本 change 仅在公网出口预留门禁接缝并消费 c09 判定结果。本期口径：redaction-gateway 未接入前不得启用公网 provider，仅私有化 / 离线路径跑通闭环（§16.4 / §24.9），公网脱敏门禁的端到端验收依赖 c09（phase9）。
- POC 稳定性。整篇文档（Word / PPT / PDF / 扫描件 / 图片 / OFD）体量大、耗时长，必须异步化才能在个人机 / 内网稳定演示，不能走同步请求。
- 数据模型复用 PRD §18 命名：`translation_jobs` / `translation_segments` / `term_bases` / `terms` / `corpora` / `documents` / `document_versions` / `document_visual_parse_results` / `model_routes` / `visual_parse_providers` / `privacy_detection_rules` / `privacy_redaction_events` / `audit_logs`。

利益相关方：医生 / 科研人员（翻译文献与院内资料）、医务行政（批量整理资料）、管理员（§17.6 翻译管理：引擎 / 术语库 / 语料库 / 任务队列 / 失败重试 / 质量反馈）。

## Goals / Non-Goals

**Goals:**

- 交付文件级异步医学翻译系统：覆盖 Word（doc/docx）、PPT（ppt/pptx）、文本型 PDF、扫描 PDF、图片（png/jpg）、OFD（转换后），单次最多 10 个文档、单文档 ≤ 50MB、不支持加密文档。
- 提供四类发起入口（左侧导航 / 文档中心右键 / AIMed 答案操作栏 / ONLYOFFICE 医疗 AI 面板）与六类上传来源（本地、我的 / 团队文档中心、当前 ONLYOFFICE 文档），来源按租户 + 文档 ACL 过滤。
- 建立翻译任务状态机（排队中 → 解析中 → 翻译中（带进度）→ 排版中 → 翻译成功 / 翻译失败 / 已取消）与异步队列，支持取消与失败重译。
- 实现版式还原按文件类型分流：Word/PPT 保结构、文本型 PDF 段落级 + 表格 + 图片位置、扫描件 / 图片经 c03 视觉解析还原；三种输出模式（仅译文 / 左右对照 / 逐段对照）。
- 保证同一任务内术语一致性（优先用户所选术语库 / 语料库，支持演示库），达到术语一致性 ≥ 95%、版式结构保留率 ≥ 90% 的验收门槛。
- 译文以新版本写入文档中心，可预览 / 下载 / 打开到 ONLYOFFICE，并与最近任务联动；全链路写 `audit_logs`，译文页展示医疗免责声明。
- 公网翻译 / 公网视觉解析前完成 PHI/PII 脱敏，失败或服务不可用时切私有化路径。

**Non-Goals:**

- 不实现选区级实时翻译的写回交互（属 c05 AI 面板写回确认链路），本 change 只负责文件级整篇翻译、不做行内即时替换。c07 不接收选区 / 短文本翻译、亦不就选区建单段 `translation_job`；选区 / 短文本翻译按 §8.12 分流归 AIMed（c04）/ 医疗 AI 面板（c05）就地处理且不创建 `translation_jobs`，c07 仅接收 c04/c05 判定为整篇 / 全文的请求建任务。
- 不重新实现模型 Provider 抽象、视觉解析服务、文本解析切分（均由 c03 提供）。
- 不实现 OFD → PDF 的转换器本身（由 c02 ofd 转 PDF 预览 / c03 文本抽取提供），本 change 只消费转换结果。
- 不纳入 §22.2/§22.3 项：翻译记忆库自动学习、术语库可视化编辑器、批量术语导入、翻译质量自动评分模型；§17.6「翻译质量反馈」仅保留最小反馈入口，不做评测闭环。
- 不做数字员工编排 / 执行历史。

## Decisions

### D1. 任务与分段数据模型：`translation_jobs` + `translation_segments`

- **决策**：以 `translation_jobs` 为任务聚合根（一条记录对应「待翻译列表」中的一个文档），`translation_segments` 为段落级最小翻译单元。
  - `translation_jobs` 字段（复用 §18 命名）：`job_id`（主键，与 `translation_segments.job_id` 外键、`recent_tasks.ref_id`、c05 回源口径同名，不用 `id`）/ `tenant_id` / `created_by` / `source` 来源（`local` / `my_docs` / `team_docs` / `onlyoffice_current`）/ `source_document_id`（来源文档，可空）/ `source_version_id` / `file_format` / `file_size` / `engine`（绑定 c03 翻译用途路由）/ `lang_from` / `lang_to` / `term_base_id` / `corpus_id` / `output_mode`（`translation_only` / `side_by_side` / `paragraph_interleaved`）/ `layout_style`（译文排版方式，与 `output_mode` 区分的独立设置项，枚举如 `compact` / `loose` / `preserve_layout`，其中 `preserve_layout`=保持原版式风格）/ `output_format`（输出 / 下载格式，枚举如 `docx` / `pdf`）/ 还原开关（`keep_original`=§13.7「是否保留原文」内容开关、`keep_image` / `keep_table` / `bilingual`）/ `status` / `progress` / `failure_code` / `failure_reason` / `result_document_id` / `result_version_id` / 时间戳。
  - **§13.6 待翻译列表三列状态映射**：列表的「上传状态 / 解析状态 / 翻译状态」三列为彼此独立展示列，由 `status` + `progress` 派生映射（不新增三个独立状态机字段）：`queued`→已上传 / 待解析 / 待翻译；`parsing`→已上传 / 解析中 / 待翻译；`translating`→已上传 / 已解析 / 翻译中 `progress`%；`laying_out`/`succeeded`→已上传 / 已解析 / 排版中或翻译成功；`failed`/`cancelled` 按失败 / 取消阶段映射。如此可在「上传完成但解析未开始」等阶段并存展示三列独立态。
  - **§13.7 译文排版方式与输出格式**：`layout_style`（译文排版方式）与 `output_format`（输出 / 下载格式）为 §13.7 与 `output_mode`（输出模式：仅译文 / 左右对照 / 逐段对照）相区分的独立设置项，三者均需持久化并可回读。
  - `translation_segments` 字段：`id` / `job_id` / `seq` / `page_no` / `paragraph_index` / `bbox`（坐标）/ `heading_level` / `block_type`（`paragraph` / `table_cell` / `header` / `footer` / `caption` / `formula` / `footnote`）/ `source_text` / `target_text` / `term_hits`（命中术语条目）/ `status` / `confidence`。
- **为什么**：段落级建模是版式还原（按 `bbox` / `heading_level` / `block_type` 回填）、术语一致性（跨段共享术语决策）、双语对照（按 `seq` 配对原 / 译文）、断点重译（只重译失败 segment）与审计溯源的共同基础；与 c03 `document_visual_parse_results` 的结构化输出字段一一对齐，解析结果可直接映射为 segments。
- **备选与取舍**：
  - 备选 A：只存整篇原文 / 译文两个大字段。被否——无法做段落级进度、术语一致性回查、版式定位与失败段重译，违背 §13.11 / §24.4。
  - 备选 B：直接复用 `document_chunks` 作为翻译单元。被否——chunk 是为 RAG 检索切分（语义块、可跨段合并），与翻译所需的「版式块」粒度和定位语义不同；混用会污染检索索引且坐标 / block_type 信息不全。故 segments 独立建表，必要时引用 `document_visual_parse_results` 的定位字段。

### D2. 异步队列与状态机：作业表驱动 + 阶段化 worker

- **决策**：采用「数据库作业表 + 后台 worker 轮询 / 消费」的异步模型，状态机严格映射 §13.8：`queued`（排队中）→ `parsing`（解析中）→ `translating`（翻译中，带 `progress` 百分比）→ `laying_out`（排版中）→ `succeeded` / `failed` / `cancelled`。每个阶段为独立可重入步骤，状态与进度落 `translation_jobs` 并写 `audit_logs`。`progress` 由「已完成 segment 数 / 总 segment 数」实时计算。
- **为什么**：POC 部署在个人机 / 内网，作业表驱动无需额外引入重量级消息中间件即可满足异步、可取消、可重试、可观测；阶段化让失败精确定位到 `parsing` / `translating` / `laying_out` 并据此给出 §13.11 失败原因。
- **降级 / 容错**：单文档内 segment 翻译失败先按 c03 路由 fallback（公网→私有化）重试；超过阈值则该 segment 标失败但不阻塞整篇，整篇在「排版中」后若失败段比例超阈值则 `failed` 并给出 `failure_code`。取消采用协作式：worker 在阶段边界检查 `cancelled` 标志后停止。
- **备选与取舍**：
  - 备选 A：引入 Kafka / RabbitMQ 等独立消息队列。被否——POC 单机 / 内网环境徒增部署与运维复杂度，违背简单优先；作业表 + worker 已足够，后续 V1.1 规模化再演进。
  - 备选 B：同步翻译（请求内完成）。被否——50MB 文档 + 视觉解析耗时远超 HTTP 超时，无法稳定演示，违背 §13.1「文件级异步」。

### D3. 版式还原按文件类型分流

- **决策**：解析与还原走三条管线，统一产出 segments 后按 `output_mode` 重建目标文件：
  - **`.pdf` 路由判定（文本型 vs 扫描型分流）**：§13.4 中文本型 PDF 与扫描 PDF 同为 `.pdf` 扩展名，无法靠扩展名区分。解析阶段先对 `.pdf` 做文本层探测（计算可提取文本层覆盖率）：覆盖率达阈值走文本型 PDF 管线（c03 `document-parsing`），否则走扫描 PDF 视觉解析管线（c03 `visual-parsing-service`）。混合页 PDF（部分页有文本层、部分页扫描）按页分流；无法可靠按页分流或探测不可用时，保守整篇兜底走视觉解析，避免扫描页误入文本管线得到空译文、或文本 PDF 误走 OCR 引入误差威胁术语一致性 / 保留率。
  - **Word/PPT（doc/docx/ppt/pptx）**：用文档对象模型（DOCX/PPTX 即 OOXML）就地替换文本节点，保留标题层级、表格、图片、页眉页脚、页码、公式与角标（§13.11）；译文文件直接生成同类型文件。
  - **文本型 PDF**：经 c03 `document-parsing` 取页 / 段 / 表格 / 图片位置，做页级预览、段落级翻译、表格识别、图片位置保留；输出可下载译文，下载格式由任务 `output_format` 字段决定（默认 `docx` 便于「打开到 ONLYOFFICE」再编辑，可选 `pdf` 更贴近原貌）。
  - **扫描 PDF / 图片（png/jpg）**：经 c03 `visual-parsing-service` 识别文字 / 版面 / 表格 / 图片位置（结构化输出含坐标与置信度），按坐标回填译文，支持左右对照预览与可下载译文。
  - **OFD**：先由 c02/c03 转 PDF / 文本抽取，再按上述 PDF 管线处理（§9.5）。
- **为什么**：不同格式的版式信息载体不同（OOXML 自带结构 vs PDF 需解析 vs 扫描件需视觉识别），单一管线无法同时满足「结构保留率 ≥ 90%」；分流让每类格式走信息损失最小的路径。
- **降级**：Word/PPT 中无法就地替换的复杂块（嵌入对象、异形公式）降级为「保留原文 + 旁注译文」而非丢弃；PDF / 扫描件版式无法重建时降级为「仅译文」纯文本输出并在任务详情标注降级原因，而非整篇失败。
- **备选与取舍**：
  - 备选 A：所有格式统一转 PDF 再翻译。被否——Word/PPT 转 PDF 会丢失可编辑结构与「打开到 ONLYOFFICE 再编辑」能力，且结构保留率下降，违背 §13.11。
  - 备选 B：所有格式统一走视觉解析（截图式 OCR）。被否——文本型文档本就有结构化文本，走 OCR 反而引入识别误差、降低术语准确率与保留率，且成本高。仅对扫描件 / 图片这类无文本层的格式才用视觉解析。

### D4. 术语库 / 语料库与术语一致性（≥ 95%）

- **决策**：术语库 `term_bases` + `terms`（源词 / 目标词 / 领域 / 优先级），语料库 `corpora`（参考双语句对 / 风格参考）。翻译时分两步保证一致性：
  1. **预处理术语锁定**：翻译前对全任务 segments 做术语匹配，命中 `terms` 的片段以受保护占位注入提示词（约束模型必须用指定译法），命中记入 `segment.term_hits`。
  2. **任务级译法表**：同一源术语在整篇任务内维护唯一目标译法（首次确定后全任务复用），杜绝同词多译。语料库作为风格 / 参考译文注入提示词（few-shot），不强制锁定。
- **一致性度量（≥ 95% 验收）**：统计任务内所有术语出现次数中「译法与该术语任务级译法一致」的占比；低于阈值在任务详情告警，支持重译。
- **为什么**：纯靠大模型无法稳定保证同词同译（§13.11 硬要求），占位锁定 + 任务级译法表是确定性兜底；语料库做软参考兼顾医学文体。
- **降级**：未配置 / 未选术语库时使用演示术语库与演示语料库（§13.11），保证 POC 可演示；私有化模型同样走占位锁定，不依赖公网。
- **备选与取舍**：
  - 备选 A：仅把术语表塞进提示词、不做占位锁定。被否——长文档下模型仍会漂移，难达 95%。
  - 备选 B：译后全局查找替换强制统一。被否——不区分语境会误替换（如缩写在不同上下文含义不同），且破坏语法；故选「译前锁定 + 任务级译法表」而非「译后强替」。

### D5. 与 c03 模型 / 视觉解析路由对接：公网 ↔ 私有化降级

- **决策**：翻译引擎不自管 provider，统一通过 c03 `model_routes`「医学翻译」用途路由获取（公网 / 私有化、优先级、fallback）；视觉解析通过 `visual_parse_providers` 路由。调用公网翻译 / 公网视觉解析前，先经 c09 redaction-gateway 做 PHI/PII 识别与脱敏（c07 不自实现识别脱敏，仅在公网出口预留门禁接缝并消费 c09 判定），事件由门禁写 `privacy_redaction_events`；识别失败 / 置信度不足 / 服务不可用 / c09 判定不可用时禁止走公网（默认拒绝公网），自动切私有化路由；私有化也不可用则该 segment / 任务按失败处理并记原因。本期口径：redaction-gateway 未接入前不得启用公网 provider，仅私有化 / 离线路径可跑通闭环，公网门禁端到端验收依赖 c09（phase9）。所有 fallback / 切换 / 脱敏写 `audit_logs`。
- **为什么**：复用 c03 唯一入口避免重复造路由，并满足医疗红线「公网调用前必脱敏、不可用必降级」；私有化部署可完全不配公网翻译模型仍能跑通整链（离线优先）。
- **备选与取舍**：
  - 备选 A：翻译模块内置一套引擎配置。被否——与 c03 重复、配置分裂、fallback / 健康检查 / 审计无法统一，违背指南「复用 §18 与统一 provider 入口」。

### D6. 与 c02 联动：当前文档发起、译文打开到 ONLYOFFICE、文档中心 / 最近任务联动

- **决策**：
  - **当前文档发起**：ONLYOFFICE 医疗 AI 面板触发时经 Bridge 取当前 `document_id` → 创建 `translation_jobs`（`source=onlyoffice_current`）→ 加入待翻译列表 → 打开医学翻译页（§13.10）。
  - **译文落库**：翻译成功后，译文文件经 c02 `save-callback-versioning` 写入文档中心，生成**新 `document_version`（`source=translation`）而非覆盖原文**，`result_document_id` / `result_version_id` 回填任务。译文新版本成功落库后，c07 作为 PRD §10.6「翻译完成」触发源的唯一产生方（c01 把 `document_events.event_type=translation_done` 指派给 c07），MUST 产生一条 c01 契约形态的 `document_events(event_type=translation_done)`，携带 `(event_type, document_id=result_document_id, version_id=result_version_id, tenant_id, occurred_at, payload)` 稳定字段，供 c03 作为 `document_events` 全部 6 类触发源的唯一重解析 / 索引消费方消费触发对译文新版本的重解析；c03 解析 / 索引就绪后另发「索引就绪」事件，再由 c04 检索侧构建索引、c06 知识库收尾侧消费该「索引就绪」事件刷新 `index_status` 与文档计数（c06 消费的是 c03 下游「索引就绪」事件而非 `translation_done`，c06 不直接消费 `document_events`）。注意区分两个口径：`document_versions.source=translation` 是版本来源字段取值，`document_events.event_type=translation_done` 是 6 类重解析触发事件之一，二者语义不混用；c02 保存回调自身产生的 `save_new_version` 不替代该 `translation_done`，c07 在译文成功落库后单独产生 `translation_done`。
  - **打开到 ONLYOFFICE**：翻译历史「打开到 ONLYOFFICE」按 `result_version_id` 用 c02 编辑器打开（§13.9）。
  - **最近任务联动**：c07 为「医学翻译」来源 `recent_tasks` 记录的写入侧 owner，任务创建 / 到达翻译成功时向 c01 所建 `recent_tasks` upsert 一条 `source=医学翻译`、`ref_type=translation_job`、`ref_id=translation_jobs.job_id`、按 `tenant_id` / `user_id` 隔离、以 `(ref_type, ref_id)` 为幂等键的记录（写法对齐 c04 7.5 范式）；最近任务展示规则（§6.5）与恢复编排（§6.6）归 c05 ai-panel-recent-tasks，c07 仅写入条目并提供按 `job_id` 回源翻译任务的取数接口，支持 c05 从最近任务恢复进入翻译历史 / 译文。
  - **确认归属边界（与 c05 ai-writeback-confirmation 不重叠）**：文件级译文落库 / 打开回写的确认沿用「可确认、可回滚、可审计 + 生成新版本」红线，其确认语义对齐 §9.6 默认写回策略矩阵「翻译结果→生成译文副本」（含医疗免责声明）。面板侧选区 / 全文写回确认归 c05 §9.6 矩阵与其 ai-writeback-confirmation；文件级翻译历史结果落库确认归 c07，c07 仅产出译文产物与 `result_document_id` / `result_version_id`，不另起一套与 c05 重复的面板侧写回确认 UI。
  - **高风险确认（消费 c05，以 `translation_job` 为确认 subject，§19.2 / §24.7）**：医学翻译文书与 AIMed 答案、知识库问答（kb_qa）答案同属高风险确认链路医学文书。c05 ai-writeback-confirmation 高风险确认键已泛化为 `(subject_type, subject_id)` 多态键，三类生产方各以原生标识为确认 subject：AIMed / kb_qa 答案=`subject_type=message`（`subject_id=messages.message_id`）、医学翻译文书=`subject_type=translation_job`（`subject_id=translation_jobs.job_id`）。译文文书确认 subject 直接取 c07 自有 `translation_jobs.job_id`（本 change 建表、主键稳定可回读），c07 MUST NOT 向 c04 `conversations` / `messages` 写入译文文书行，MUST NOT 依赖或重定义 c04 `conversations.module` 取值域（其枚举保持 c04 owner 定义、不含翻译值），确认键稳定不悬空。译文文书下发 / 落库前若被 c05 服务端 `risk_type` 分类器识别为高风险（命中诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息），c07 MUST 以 `(subject_type=translation_job, subject_id=translation_jobs.job_id)` 为键前置消费 c05 ai-writeback-confirmation 确认链路，按 `confirmed_role∈{doctor,reviewer}` 裁决：普通用户仅能生成草稿 / 提交审核、MUST NOT 直接下发，授权角色确认后方可下发。`risk_type` 判定与 `writeback_confirmations`（subject 列承载 `document_id` / `message_id` / `translation_job` 多态）记录唯一 owner=c05，c07 仅触发链路并写 `audit_logs`。该高风险确认与上条文件级落库确认并存、职责正交：前者解决「高风险译文内容是否需医生 / 审核角色裁决方可下发」，后者解决「译文产物落库是否生成新版本 / 不覆盖原文 / 可回滚审计」。
- **为什么**：遵守「可确认、可回滚、可审计 + 生成新版本不覆盖」红线，且复用既有保存 - 版本链路，无需新写盘逻辑。
- **备选与取舍**：
  - 备选 A：译文直接覆盖原文版本。被否——违背可回滚红线，原文不可恢复。
  - 备选 B：译文只存对象存储不进文档中心。被否——无法「打开到 ONLYOFFICE」与最近任务恢复，断闭环。

### D7. 输出模式与双语对照生成

- **决策**：基于 segments 的原 / 译文配对，由同一套生成器按 `output_mode` 渲染：`translation_only`（仅译文，按版式回填 target_text）/ `side_by_side`（左原文右译文，按页 / 段并排）/ `paragraph_interleaved`（逐段上下，原文段后紧跟译文段）。是否保留原文 / 图片 / 表格 / 双语对照由任务开关控制。
- **为什么**：三种模式共享同一 segment 数据，仅渲染布局不同，避免三套翻译逻辑；保留开关直接作用于 segments 过滤与回填。
- **备选与取舍**：备选——为每种输出模式各跑一次翻译。被否——重复翻译浪费算力且可能产生不一致译文，违背简单优先与一致性。

### D8. 术语库 / 语料库管理、翻译管理后台与配置→生效闭环（§17.6 / §17.8 / §24.6 / §0.3）

- **决策**：c07 是 `term_bases` / `terms` / `corpora` 的唯一建表 owner，亦承载术语库 / 语料库的最小后台配置端能力（§17.8 / §24.6「翻译模型、术语库、语料库配置」），与 §17.6 翻译管理最小入口一并落地，使 §0.3 后台配置闭环在翻译支可被验收：
  - **术语库 / 语料库管理（最小形态）**：管理员可创建 / 编辑术语库与术语条目（源词 / 目标词 / 领域 / 优先级）、新增 / 编辑语料库并导入条目、启用 / 停用、绑定到「医学翻译」用途；操作按 `tenant_id` 隔离并写 `audit_logs`。本期为条目级 CRUD + 导入 + 启停 + 绑定，**不做**术语库可视化编辑器与批量术语导入器（属 §22.2/§22.3 延期，与 Non-Goals 一致）。
  - **翻译管理后台（最小）**：§17.6 六项——翻译引擎为只读路由视图（复用 c03 `model_routes`，引擎 provider 的新增 / 编辑归 c03 model-provider-config）、任务队列查看、失败任务重试触发、最小翻译质量反馈入口；**不做**翻译质量评测闭环（属延期）。
  - **配置→生效闭环（§0.3）**：管理员配置翻译引擎（绑定 c03 `model_routes`「医学翻译」用途）+ 选定术语库 / 语料库 + 执行连通性测试（复用 c03 `provider_health_checks`）后，前台翻译任务按该配置生效——输出走该引擎路由、命中所选术语库一致译法、体现所选语料库风格；术语条目更新后对同一源术语再翻译即采用新译法，并可经术语命中溯源核对。连通性测试不通过的公网引擎不置为前台可用（与 redaction-gateway 未接入前公网默认关闭一致）。
- **为什么**：术语库 / 语料库被 PRD 列为独立 P0 条目并三处（§17.6 / §17.8 / §24.6）要求「配置」能力，仅「运行时使用 + 演示种子」无法满足 §0.3「配置→前台生效」闭环的可验收落点；spec 是可验收契约，必须有 Requirement / Scenario 而非仅 tasks 种子。
- **范围裁剪声明**：本期术语库 / 语料库配置取最小形态（条目级编辑 + 导入 + 启停 + 绑定 + 连通性测试 + 前台生效验收），可视化编辑器、批量术语导入器、翻译记忆库自动学习、质量自动评分均延期至 §22.2/§22.3。
- **F37 落点说明**：F37 原 fix 建议在 c09 acceptance-evidence 增补「配置翻译引擎+术语库+语料库连通性测试通过→前台生效」用例；因本轮仅允许改 c07 目录，等价的「配置→连通性→前台生效」可验收 Scenario 已在 c07 spec「翻译引擎 / 术语库 / 语料库配置连通性与前台生效」Requirement 落地，c09 端的跨期证据汇总仍由 c09 acceptance-evidence 在其自身轮次补齐。

## Risks / Trade-offs

- [Word/PPT 复杂版式（嵌套表格 / 文本框 / 异形公式 / 角标）就地替换可能破坏结构，威胁保留率 ≥ 90%] → 对不可安全替换的块降级为「保留原文 + 旁注译文」并在任务详情标注；保留率统计纳入降级块，低于阈值时告警支持重译，不静默丢版式。
- [扫描件 / 图片视觉解析置信度低导致错译或错位] → 透传 c03 视觉解析 `confidence` 与 `failure_reason`；低置信段在左右对照中高亮提示人工核对；解析失败按 §13.11 给明确失败原因并允许重译。译文默认草稿、展示免责声明，不作临床定论。
- [大文档 / 单次 10 个 50MB 文档并发拖垮 POC 单机] → 队列限并发与单租户配额；分阶段流式处理 segments 而非整篇载入内存；进度可见、可取消，避免无响应。
- [公网脱敏服务不可用时若仍调用公网模型会泄露 PHI] → 硬阻断：脱敏失败 / 置信度不足 / 服务不可用一律禁止公网，自动切私有化；私有化也不可用则任务失败而非降级明文外发，事件全程审计。
- [术语一致性受模型漂移影响达不到 95%] → 译前占位锁定 + 任务级译法表做确定性兜底，并以任务级一致性度量门控；未达阈值不静默放行，给出告警与重译入口。
- [OFD 依赖外部转换 / 第三方 SDK，公网或 SDK 不可用] → OFD 仅作「转换后支持」，转换失败按明确失败原因处理；不把 OFD 原生编辑纳入本期（与 c02 V1.0 不支持 OFD 在线编辑一致）。
- [segments 与 document_chunks 概念易混淆导致误用检索索引] → 文档明确二者分表、分语义（翻译版式块 vs RAG 检索块），翻译不写 `document_chunks` / `embeddings`，只读 c03 解析定位字段。

## Migration Plan

- 本 change 为净新增能力，`openspec/specs/` 当前为空，无既有 spec 行为变更，无数据迁移。
- 部署顺序：须在 c01（租户 / 文档 / 对象存储 / 审计 / 脱敏骨架）、c02（编辑器 + Bridge + 保存版本链路）、c03（模型路由 + 文本 / 视觉解析）就绪后上线。
- 上线步骤：建 `translation_jobs` / `translation_segments` 及术语库 / 语料库相关表（`term_bases` / `terms` / `corpora`，含演示库种子数据）→ 部署翻译 worker 并接入 c03 路由与 c01 脱敏前置 → 接入四类入口与 c02 译文落库 / 打开链路 → 灌入 Demo 翻译验收集（§24.4）跑通仅译文 / 左右对照 / 逐段对照与术语一致性 / 保留率验收。
- 回滚策略：能力为独立模块，回滚仅需下线翻译入口与 worker；已生成译文为独立 `document_version`，不影响原文与其它版本，无需数据回退。

## Open Questions

- 术语一致性 ≥ 95% 与版式结构保留率 ≥ 90% 的**自动度量口径**（术语命中分母如何界定、保留率以哪些版式要素加权）需在 tasks / 验收集阶段与 §24.4「术语一致性验收 / 版式结构保留率验收 / 参考译文对比」对齐定稿。
- 文本型 PDF 译文的**可下载格式**已定稿为由 `translation_jobs.output_format` 字段承载（枚举 `docx` / `pdf`，默认 `docx` 便于「打开到 ONLYOFFICE」再编辑，可选 `pdf` 更贴近原貌）；剩余开放点仅为是否按 `output_mode` 设置不同默认值，留待验收集阶段确认。
- Word/PPT 公式 / 角标 / 参考文献格式的**保留深度**（§13.11「尽量保留」）在 POC 的最低验收线需明确，避免过度投入实现。
- 演示术语库 / 演示语料库的**领域覆盖范围与体量**（覆盖哪些科室 / 文献类型）由 Demo 数据集（§22.1 / §24.4）统一定义，本 change 仅约定接入方式。
