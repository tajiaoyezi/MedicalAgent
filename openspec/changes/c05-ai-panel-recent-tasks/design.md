## Context

本 change（9 阶段中的第 5 阶段 `c05-ai-panel-recent-tasks`）承接 c02（ONLYOFFICE 集成与文档 Bridge API）与 c04（AIMed/RAG/引用），把「人在文档内驱动 AI → AI 安全写回文档 → 全平台 AI 任务沉淀为可恢复历史」这一段闭环补齐。它是主验收闭环（PRD §24.2）的关键一环，也是医疗安全红线「AI 操作文档必须可确认、可回滚、可审计」的落地点。

当前状态与约束：

- 上游已交付（只消费、不改契约）：
  - c01：`users`/`tenants`/`roles`/`permissions`、文档中心、对象存储、文档元数据与 ACL（`documents`/`document_permissions`）。
  - c02：ONLYOFFICE 私有化部署、四类编辑器路由、三段式 Bridge（插件 + 受控 postMessage + 宿主桥 SDK）、读取/写回/面板控制方法契约、保存回调-版本链路（`document_versions`，`source ∈ {user_edit, ai_writeback, translation, import, template}`）、乐观并发（读取返回 `revision`，写回带 `expectedRevision`）、原文出口标注与脱敏挂载点、ACL/审计落点。c02 明确把「AI 写回确认弹窗 UI 与具体 AI 技能」划归本 phase。
  - c04：AIMed 会话与带引用回答（`conversations`/`messages`/`citations`，由 c04 建表）。
  - c09：PHI/PII 识别与脱敏门禁 redaction-gateway（及 `privacy_detection_rules`/`privacy_redaction_events`）唯一 owner=c09；本 phase 仅在公网出口预留门禁接缝、消费其判定结果，不实现识别脱敏引擎（端到端随 c09 phase 9 落地）。
- 协作（本 phase 仅发起、不实现）：c07 医学翻译任务系统（`translation_jobs` 由 c07 落库）。
- 范围口径：V1.0 POC，可部署在个人机或内网（可能无公网）。仅覆盖 PRD §22.1 P0 中本期项：医疗 AI 右侧面板、当前文档发起 AIMed、当前文档发起医学翻译、最近任务、AI 写回确认机制。论文转 PPT / AI 文档脑图 / 文档生成 PPT 属 §22.2，数字员工的创建/运行/编排/执行历史属 §22.2–22.3，均不纳入。
- 安全红线（context rules）：写回前必须展示「原文/修改后/修改说明/影响范围」并提供「应用到文档/生成副本/取消」；以新版本/副本/批注为默认策略避免覆盖原文；高风险写回进医生/授权角色确认链路；面板触发的公网模型调用必须先经 PHI/PII 脱敏门禁；确认动作写确认记录与 `audit_logs`。
- 利益相关方：医生/科研/医务行政（文档内 AI 操作与确认）、授权审核角色（高风险确认）、平台运维（面板与历史服务部署）、下游 phase（c06/c07/c08 向最近任务投递历史）。

本期净新增：`openspec/specs/` 当前为空，无既有 spec 行为被修改。proposal 定义三个新能力：`medical-ai-panel`、`ai-writeback-confirmation`、`recent-tasks`。

## Goals / Non-Goals

**Goals:**

- 在 ONLYOFFICE 编辑器右侧落地医疗 AI 面板，提供三类入口（右侧固定图标 / 顶部医疗空间按钮 / 选区浮层：润色·翻译·解释·补引用），按文档类型（docx/pdf/ofd）渲染对应 P0 功能集，并提供「从当前文档发起 AIMed / 发起医学翻译」入口（PRD §9.3–9.5、§24.2）。
- 把面板内每类 AI 操作经 c02 Bridge 安全地接到编辑器：读取走读取类方法，写回统一前置「写回确认机制」，再映射到具体写回方法（PRD §9.4、§9.6）。
- 落地 AI 写回确认机制：写回前四要素展示 + 三按钮，默认写回策略矩阵到 Bridge 方法的映射，写回时的冲突/权限处理，高风险确认链路，确认记录与审计（PRD §9.6、§9.9、§19.2、§24.2）。
- 落地最近任务：六类来源（AIMed/知识库/数字员工占位/翻译/在线文档 AI/模板）统一数据模型与各自恢复内容，展示规则（标题/前 10 字/悬浮/倒序/分组/多选筛选）、删除二次确认与「是否同时删除关联文档」（PRD §6.4–6.7）。
- 给出公网不可用时的离线/私有化降级路径（私有化模型、私有化解析/识别、本地面板与历史能力）。

**Non-Goals（明确排除）:**

- Bridge 读写/面板控制/保存回调链路本体——归 c02，本期复用其契约不改其结构。
- AIMed 模式本体与 RAG/引用实现、公网调用前脱敏的实际执行——归 c04/c03；面板只发起并约束「调用前必经脱敏门禁」。
- 医学翻译任务编排与 `translation_jobs` 落库——归 c07；面板只发起并传上下文。
- 论文转 PPT / AI 文档脑图 / 文档生成 PPT（§22.2）；OFD 原生在线编辑与依赖第三方 SDK 的 OFD 批注（§9.5，V1.0 不支持）。
- 数字员工创建/运行/编排/执行历史（§22.2–22.3）——最近任务对其仅保留「来源占位」，不生成其历史写入与恢复实现。
- 校对/润色/排版底层算法实现（参考 §15）——作为面板技能被调用，本期不实现模型侧逻辑。

## Decisions

### D1. 面板与 c02 Bridge 的协作边界：面板只调宿主桥 SDK，写回统一经确认网关

**决策**：医疗 AI 面板作为 c02 宿主页内的右侧 UI，全程只调用 c02 宿主桥 SDK（`bridge.*` Promise 方法），不直接接触 ONLYOFFICE 插件 / postMessage / 文档 DOM。读写分两条路径：

- **读取路径**：面板技能取上下文统一走 c02 读取类方法（`getDocumentType`/`getDocumentId`/`getDocumentTitle`/`getFullText`/`getSelectedText`/`getCurrentParagraph`/`getDocumentOutline`/`getReferences` 等）。读取返回体携带 c02 的 `revision`（用于写回乐观校验）与 `redaction_hook` 原文出口标注（用于脱敏门禁）。面板不缓存原文超过单次操作生命周期。
- **写回路径**：面板技能产出的任何「会改文档语义内容」的结果，MUST 先进入本期 `ai-writeback-confirmation` 确认网关，由网关在用户确认后才调用 c02 写回类方法（`replaceSelection`/`insertText`/`appendSection`/`insertComment`/`insertCitation`/`applyStyle`/`createNewDocument`/`saveDocument`），并填入 c02 写回入参 `expectedRevision` 与 `writebackSource`。纯排版类编辑器操作（目录/分页/段落/页眉页脚）不改语义内容，走 c02 编辑器操作直接执行，不经确认网关。
- **面板控制**：面板的打开/关闭/技能调度/流式回流走 c02 面板控制类（`openAIPanel`/`closeAIPanel`/`runAIPanelSkill`/`streamContentToEditor`）。

**备选与取舍**：

- 备选 A：面板自带一套 Bridge，直接 postMessage 进插件。被否——重复实现 c02 已交付的安全边界（origin 白名单、请求-响应配对、写回前 ACL/审计），且两套 Bridge 易产生权限校验不一致的越权缺口。
- 备选 B：面板技能在服务端直接改 OOXML 再 reload 编辑器。被否——拿不到选区/当前段落/光标级上下文，破坏「所见即所得 + 用户确认」体验，且绕过 c02 协同与乐观并发。
- 选「只调宿主桥 SDK + 写回经确认网关」：复用 c02 的安全/审计/冲突机制，面板与编辑器解耦，确认网关成为「AI 改文档」的唯一收口，天然满足可确认/可回滚/可审计红线。

**离线/私有化降级**：宿主桥 SDK、面板 UI、确认网关前端均随前端本地打包，无外呼；无公网时面板挂载、读取、确认、写回、最近任务展示均可用，仅 AI 推理本身依赖下游模型 phase 的私有化降级（D5）。

### D2. 写回确认网关：四要素 diff 呈现 + 三按钮 + 单一收口

**决策**：所有面板 AI 写回经一个统一的「写回确认网关」，是 AI 改文档的唯一入口。网关职责与呈现：

- **四要素呈现**：原文 / 修改后 / 修改说明 / 影响范围（PRD §9.6）。diff 呈现按操作粒度自适应：
  - 选区类（选区润色/选区翻译）：行内字符级 diff（删除标红、新增标绿），原文=选区文本，影响范围=该选区。
  - 全文类（全文润色）：分段并排 diff（左原文/右修改后），影响范围=全文，默认策略「生成新文档」，故 diff 仅作预览不就地改原文。
  - 批注类（校对/补引用/插入标注）：以「建议列表」呈现（位置定位 + 原片段 + 建议内容 + 理由），影响范围=各定位点；不在正文做就地替换。
  - 排版类经 AI 的（AI 论文排版）：呈现「结构/样式变更摘要」+ 关键页预览，影响范围=全文版式，默认「生成新版本」。
- **三按钮**：应用到文档（按默认策略写回）/ 生成副本（写入原文档副本，原文不变）/ 取消（丢弃，不建版本/副本）。
- **免责声明**：网关底部固定展示 §19.3 医疗免责声明，提示结果为草稿/辅助建议。
- **未确认零写回**：结果生成后、用户点确认前，MUST NOT 调用任何 c02 写回方法。

**备选与取舍**：

- 备选 A：每个技能各自实现确认弹窗。被否——四要素/按钮/免责声明/审计落点会漂移，且无法保证「所有写回都过确认」这条红线；单一收口才能在网关层强制前置 ACL/冲突/脱敏/高风险判定。
- 备选 B：用 ONLYOFFICE 自带「修订/审阅」模式替代确认面板。被否——审阅模式只覆盖正文改动接受/拒绝，表达不了「修改说明/影响范围/生成副本/高风险审核」语义，也无法承载非正文（翻译副本/新文档）策略。
- 选「统一网关 + 自适应 diff」：一个收口保证红线一致；diff 粒度按策略自适应，避免「全文级 diff 看不清/选区级 diff 太碎」。

**离线降级**：diff 计算与确认 UI 纯前端/本地，无外部依赖。

### D3. 默认写回策略矩阵 → 具体 Bridge 写回方法的映射

**决策**：按 PRD §9.6/§9.4 落地策略矩阵，并显式映射到 c02 写回方法与 `document_versions.source` 取值。默认值不可被技能静默覆盖；用户可在确认网关点「生成副本」改走副本路径。

| 操作类别（面板技能） | 默认写回策略（§9.6/§9.4） | c02 写回方法 | 版本/产物 | `writebackSource` / `source` |
|---|---|---|---|---|
| 选区润色 / 选区翻译 | 替换选区 | `replaceSelection(text)`（带 `expectedRevision`） | 原文档新版本（保存回调时） | `ai_writeback` |
| 全文润色 | 生成新文档，不覆盖原文 | `createNewDocument(content, templateId)` | 新文档 | `ai_writeback` |
| 校对 | 批注 / 建议 | `insertComment(range, comment)` 批量 | 原文档新版本 | `ai_writeback` |
| 补引用 / 插入标注 | 写回指定位置 | `insertCitation(position, citation)` / `insertComment` | 原文档新版本 | `ai_writeback` |
| AI 论文排版 | 生成新版本 | `applyStyle`/`appendSection` 组合 → `saveDocument` | 原文档新版本 | `ai_writeback` |
| 医学翻译（文件级，发起 c07） | 生成译文副本（产出与落库确认归 c07，本网关不二次确认） | 由 c07 产出译文文件并执行文件级落库确认 → 落副本 | 译文副本文档 | `translation` |
| 辅助显示 | 不写回 | 无 | 无 | — |
| 目录/更新目录/目录级别/分页/页眉页脚/段落 | 编辑器操作 | `getDocumentOutline`/`applyStyle` 等直执行 | 协同保存时落版本 | `user_edit`（非 AI 语义改动） |

- 「应用到文档」与「生成副本」共用同一映射，差异仅在目标文档：副本路径先 `createNewDocument` 复制原文档再写入。
- 任何被接受的写回最终经 c02 保存回调链路落为独立 `document_versions`（含 `source`/`file_hash`），实现可回滚（D6）。

**备选与取舍**：

- 备选：全部统一为「生成新版本」一种策略。被否——选区润色就地替换体验更顺、校对天然是批注、翻译需双文件对照，单一策略丢失语义且违反 §9.4 逐项写回方式。
- 备选：默认就地覆盖正文（仅靠版本回滚兜底）。被否——违反「以新版本/副本/批注为默认避免覆盖」红线；覆盖后即便可回滚也增大医疗误改风险。
- 选「矩阵化逐项映射 + 默认不覆盖」：贴合 §9.4 写回方式，且每条都落版本保证可回滚。

### D4. 最近任务统一数据模型与六类来源恢复

**决策**：以 PRD §18 `recent_tasks` 为唯一聚合表，承载六类来源的「展示元数据 + 恢复指针」，不复制各来源的业务数据本体（恢复时回各自来源表取详情）。`recent_tasks` 唯一建表 owner=c01（c01 建最小表并负责 §6.5 展示规则壳 / §6.7 删除规则壳），本 phase 绝不重复建表，仅对其 ALTER 补列以承载 §6.6 六类来源恢复编排所需字段；ALTER 迁移须排在 c01 建表迁移之后执行。

- **统一字段**：`task_id`、`tenant_id`、`user_id`、`source`（取 c01 recent-tasks-shell 定义的 §6.4 中文规范值：`AIMed 学术助手` / `医疗知识库问答` / `医疗数字员工` / `医学翻译` / `在线文档 AI 操作` / `模板生成文档`，MUST NOT 写入 `aimed`/`kb_qa`/`doc_ai` 等机器短码——机器短码仅为本文档对来源类别的简写/对应 c04 的 `module` 维，非 `source` 列取值）、`title`（按任务最初用户原始问题生成）、`ref_type` + `ref_id`（指向来源实体，见下表）、`updated_at`（排序键）、`deleted_at`（软删，以上由 c01 建表时落）；本 phase ALTER 补列 `title_preview`（前 10 字，悬浮展示完整 `title`）、`status`、`created_at`、`related_document_id`（关联文档，用于删除时「是否同时删除关联文档」），并补 (`ref_type`,`ref_id`) 唯一约束与 `updated_at` 排序索引。
- **排序/分组/筛选**：按 `updated_at` 倒序；分组「今天 / 7 天内 / 30 天内 / 1 年内 / 全部」按 `updated_at` 落桶；筛选按 `source` 多选。这些均为对 `recent_tasks` 的查询逻辑，不引新表。
- **写入方式**：各来源在产生/更新任务时投递一条 upsert 到 `recent_tasks`（按 `ref_type`+`ref_id` 幂等键、`source`/`ref_type`/`ref_id`、按 `tenant_id`/`user_id` 隔离）。本 phase 负责 `doc_ai`（在线文档 AI 操作）来源的写入与聚合服务及统一展示（写入侧 spec 落点 = 本 change recent-tasks「在线文档 AI 操作写入最近任务」Requirement）；其余来源由**产生来源的对应 phase 负责写入自己的 recent_tasks 条目**（写入侧 owner=来源 change），本 phase 只定义统一契约与读取/恢复编排，不替它们实现业务，也不重复定义其展示/删除 Scenario（归 c01 列表壳）。
- **`source` 枚举规范值**：由 c01 recent-tasks-shell spec 定义为唯一真值，取 PRD §6.4 六类来源名（AIMed 学术助手 / 医疗知识库问答 / 医疗数字员工 / 医学翻译 / 在线文档 AI 操作 / 模板生成文档），各写入方一律使用规范值。
- **各来源写入侧 spec 落点（投递契约修正）**（下表 source 列括号内 `aimed`/`kb_qa`/`doc_ai` 等机器短码仅为来源类别简写 / 对应 c04 `module` 维，**非 `recent_tasks.source` 列取值**；`source` 列实际写入一律为括号前的 §6.4 中文规范值）：

  | source（中文规范值） | 写入侧 owner change | 写入侧 spec Requirement 落点 |
  |---|---|---|
  | AIMed 学术助手（aimed） | c04 | aimed-assistant「AIMed 会话进入最近任务」（source=AIMed 学术助手） |
  | 医疗知识库问答（kb_qa） | **c06**（非 c04） | kb-search-qa「知识库问答会话持久化 + 写入最近任务」（source=医疗知识库问答，复用 c04 conversations/messages 经 module/source 维区分） |
  | 医学翻译（translation） | c07 | medical-translation「翻译任务进入最近任务」（source=医学翻译，ref_type=translation_job、ref_id=translation_jobs.job_id） |
  | 模板生成文档（template） | c08 | template-center「模板生成文档进入最近任务」（source=模板生成文档，ref_type=document、ref_id=result_document_id，回源 `documents`） |
  | 在线文档 AI 操作（doc_ai） | c05（本 change） | recent-tasks「在线文档 AI 操作写入最近任务」（source=在线文档 AI 操作，ref_type=writeback_confirmation、ref_id=writeback_ref，回源 `writeback_confirmations`） |
  | 医疗数字员工（digital_agent） | 占位不写 | —（V1.0 仅来源占位，不写入条目） |

**`ref_type` 收口（唯一对应回源表）**：`ref_type` 取值 MUST 与 `ref_id` 实际回源表唯一对应，恢复分发器仅凭 `ref_type` 即可判定回源表，MUST NOT 出现同一 `ref_type` 指向两张不同源表的过载。取值集合：`conversation`→`conversations`（aimed / kb_qa）、`document`→`documents`（template，c08 产物个人文档行主键）、`translation_job`→`translation_jobs`（translation）、`writeback_confirmation`→`writeback_confirmations`（doc_ai，单次写回确认记录）。doc_ai 取 `ref_type=writeback_confirmation`（而非 `document`）以避免与 c08 模板来源的 `ref_type=document` 过载；任何消费方 MUST NOT 按 `ref_type=document` 直推 `ref_id` 为 `document_id` 去读 doc_ai 来源，doc_ai 的关联文档一律经本能力解析 `related_document_id`。

**六类来源 `ref_type` 与恢复内容映射（PRD §6.6）**：

| source | ref_type → ref_id 指向 | 恢复内容 | 恢复动作 |
|---|---|---|---|
| AIMed | `conversation_id`（c04 `conversations`/`messages`） | 问答记录、模式、上传文件、知识库选择、引用资料、Agent 状态 | 打开 AIMed 会话续聊 |
| 医疗知识库问答 | `conversation_id`（c06 kb 问答会话，复用 c04 conversations/messages 经 module/source 维区分） | 问答记录、知识库选择、检索源、引用段落 | 打开 KB 问答会话 |
| 数字员工 | 占位（不落 `ref_id` 实体） | —（V1.0 仅来源占位，不实现恢复） | 显示「规划中」，不可恢复 |
| 医学翻译 | `ref_type`=translation_job、`ref_id`=`translation_jobs.job_id`（c07 `translation_jobs` 主键列 job_id） | 原文文件、译文文件、语言方向、术语库、翻译进度、历史版本 | 打开翻译任务详情 |
| 在线文档 AI | `ref_type`=`writeback_confirmation`、`ref_id`=`writeback_ref`（指向 `writeback_confirmations` 的单次操作记录、非裸 `document_id`，按操作粒度区分；恢复时由确认记录非哈希字段还原） | 文档 ID（`document_id`）、选区（`confirmed_scope`）、AI 操作类型（`operation_type`）、输出结果（`output_version_id`→`document_versions`）、写回记录（确认记录本体） | 打开文档定位选区/回看写回 |
| 模板生成 | `ref_type`=`document`、`ref_id`=`document_id`（c08 产物，回源 `documents` 行主键） | 模板 ID、生成文档、使用时间、编辑状态 | 打开生成文档 |

**恢复编排归属补记**：六类来源的恢复编排（拿 `ref_id` 回源表取详情并展示）由本 phase owns，但详情数据由各来源 owner 保证：AIMed 恢复编排消费 **c04** 的 `conversation_id`，kb_qa 恢复编排消费 **c06** 的会话（c06 复用 c04 conversations/messages、经 module/source 维区分 kb_qa 与 AIMed，由 c06 写入并在其 spec 声明），translation 消费 c07 的 `job_id`，doc_ai 消费本期写回确认记录，template 消费 c08 的 `document_id`；数字员工不恢复（仅占位）。本 phase 对全部可恢复来源（AIMed/kb_qa/translation/doc_ai/template）均提供恢复 Scenario 与编排 task。kb_qa 的源 owner 统一为 c06，不与 c04（AIMed）混淆。

**删除规则（PRD §6.7）**：§6.7 删除规则（二次确认 / 默认不删关联文件 / 仅勾选「同时删除关联文档」才删 / 单条与批量 / 删除审计）的唯一真值与可验收 Scenario 归 c01 recent-tasks-shell 列表壳，本 phase 不重复定义其 Scenario。本 phase 仅在「同时删除关联文档」时补充按各来源 `related_document_id` 解析其关联文档对象的差异（doc_ai→写回生成文档/副本、translation→译文副本、template→生成文档；会话来源与数字员工占位无独立关联文档），解析得到的文档删除仍交 c01 删除规则执行（经 ACL 校验、软删进回收站、可经 `document_versions` 追溯）；`ref_id` 为空时关联文档解析为空操作。

**备选与取舍**：

- 备选 A：六类来源各建一张历史表，前端聚合。被否——前端做跨源排序/分组/分页代价高且不一致，且新增来源要改前端聚合；统一聚合表让排序/分组/筛选退化为单表查询。
- 备选 B：把各来源业务数据全冗余进 `recent_tasks`。被否——数据双写易腐化（如翻译进度变化要同步两处），且违反 §18 既有表的归属；用 `ref_type`+`ref_id` 指针只存「恢复入口」，详情回源表取最新。
- 选「统一 `recent_tasks` + 指针恢复」：复用 §18 命名，新增来源只需约定 `source`/`ref_type`，恢复内容由源表保证最新。

### D5. 选区浮层技能（润色 / 翻译 / 解释 / 补引用）的就地处理与分流

**决策**：选中文本时在选区附近浮层展示四个快捷动作，统一以 `getSelectedText` 取选区为上下文，按动作分流：

- **润色**：选区润色技能 → 结果经确认网关 → `replaceSelection`（默认替换选区）。
- **翻译**：按 §8.12「选中文档中的一段文字翻译」分流到医疗 AI 面板就地处理（短文本，不创建文件级 `translation_jobs`）→ 确认网关 → `replaceSelection` 或就地展示；区别于「文档内点击医学翻译/翻译全文」走 c07 文件级任务（D 见 panel spec）。
- **解释**：调 c04 AIMed 就地问答（"这段是什么意思"语义），结果只在浮层/面板展示，MUST NOT 写回（对应 §8.12「深度文献伴读中问这段是什么意思」就近就地）。
- **补引用**：调 c04 检索带引用来源 → 确认网关 → `insertCitation` 写回光标/选区指定位置；引用须可溯源、可点击。

四个动作中需公网模型的（润色/翻译/解释/补引用）在调用前 MUST 经面板脱敏门禁（D7）。

**备选与取舍**：

- 备选：浮层动作直接改文档不经确认。被否——润色/翻译/补引用都改语义内容，必须过确认网关；仅「解释」是只读，天然不写回。
- 备选：选区翻译也走 c07 文件级任务。被否——§8.12 明确「选中一段文字翻译」属面板/AIMed，文件级任务用于「翻译全文/文档内点击医学翻译」，错配会给短文本翻译套上异步任务的重流程。
- 选「按 §8.12 分流 + 写类过网关、读类就地」：贴合分流矩阵，体验轻快且不破红线。

### D6. 写回冲突、权限与高风险确认链路

**决策**（对齐 PRD §9.9、§19.2，复用 c02 D2/D6 机制）：

- **冲突（乐观并发）**：读取上下文时记录 c02 返回的 `revision`/内容哈希；用户点「应用到文档」时由确认网关比对当前文档 `revision`，不一致则阻止写回并提示「文档已变更，请重新读取上下文」，MUST NOT 用过期结果覆盖；保存回调失败由 c02 自动重试，失败告警（成功率 ≥ 99%）。
- **权限**：写回前网关校验文档存在性与编辑权限（`document_permissions`/ACL，按 `tenant_id`/`user_id`/`role`/`acl` 过滤）。文档被删 → 禁写回提示「文档不存在」；无编辑权限 → 禁「应用到文档」，仅允许「生成副本」或查看。
- **高风险确认链路**：写回内容先经 `risk_type` 判定（命中诊疗/用药/医嘱/临床文书/患者个体信息为高风险）。高风险且当前用户为普通角色 → 仅能「生成草稿/提交审核」，MUST NOT 最终确认；具备医生（`doctor`）/授权审核（`reviewer`）角色者方可最终确认。`confirmed_role` 取值可枚举为 `doctor` / `reviewer`；这两类角色由 c01 auth-rbac 作为 RBAC 唯一真值定义（或等价 `highrisk:confirm` 权限点），本期引用其确定角色名，不在 c05 平行重定义角色集。该确认链路的确认 subject 泛化为 (`subject_type`, `subject_id`) 多态键，覆盖三类回源：文档写回（`subject_type=document`、`subject_id=document_id`，回源 `documents`）、AIMed 答案 / 知识库问答（kb_qa）答案（`subject_type=message`、`subject_id=messages.message_id`，回源 c04 所建 `conversations`/`messages`）、医学翻译文书（`subject_type=translation_job`、`subject_id=translation_jobs.job_id`，回源 c07 `translation_jobs`）。下发前确认 MUST 显式承认三类产生方：AIMed 答案（c04）、知识库问答（kb_qa，c06）、医学翻译文书（c07）——AIMed/kb_qa 本就落 c04 会话/消息表、确认键直取其 `message_id`；c07 译文文书以 `translation_job` 为 subject、MUST NOT 为取确认键而向 c04 `conversations`/`messages` 写 message 行（本能力消费 c07 稳定的 `translation_jobs.job_id`）。三者复用同一 `writeback_confirmations` 表与同一角色裁决；普通用户对高风险输出只能提交审核、不能直接下发，授权角色确认后方可下发。c09 引用式收口对三类产生方与 document 级确认统一验收 / 审计（c07 按 `subject_type=translation_job` 核对）。c04 `conversations.module` 枚举保持 {aimed, kb_qa}（翻译不写 `conversations`，以 `translation_job` 作 subject）。
- **risk_type 判定与 confirmation 记录唯一 owner=c05（边界收口）**：`writeback_confirmations` 表与 `risk_type` 分类器（高风险角色裁决）唯一 owner=c05，本能力服务端实现判定与建表落记录、不可篡改保证落在本侧；c09 安全合规为**引用式收口**——仅消费/关联本能力的 `risk_type` 判定与 `writeback_confirmations` 记录做统一验收与审计，不新造独立 confirmation 表、不重复实现风险分类拦截器。此处与 redaction-gateway（唯一 owner=c09，c05 消费）方向相反但同为单点收口：脱敏归 c09，高风险写回确认归 c05。
- **确认记录与审计**：每次确认生成确认记录（§19.2 全字段：`confirmation_id`/subject（多态键，由 `subject_type` ∈ {document, message, translation_job} 与 `subject_id` 承载 `document_id`/`message_id`/`translation_jobs.job_id`，与 §19.2「document_id / message_id」对齐并泛化覆盖译文文书）/`confirmed_by`/`confirmed_role`/`confirmed_at`/`confirmed_scope`/`risk_type`/`before_content_hash`/`after_content_hash`/`confirmation_action`/`audit_log_id`，并额外承载 doc_ai §6.6 恢复载体列 `operation_type`/`output_version_id`，使 `recent_tasks.ref_id`=`writeback_ref` 指向本操作记录可还原选区/AI 操作类型/输出结果/写回记录），写入 `audit_logs` 并以 `audit_log_id` 关联；写回经 c02 落 `document_events`。可回滚基于 `document_versions`，确认/审计记录不随回滚删除。

**备选与取舍**：

- 备选：高风险判定放前端。被否——前端不可信，`risk_type` 与角色裁决必须服务端做，否则普通用户可绕过审核确认。
- 备选：写回时悲观锁文档。被否——与 c02 多人协同冲突；沿用 c02 乐观并发，冲突即重读，低并发 POC 下成本可接受。
- 选「服务端裁决 + 乐观并发 + 全字段确认记录」：满足可确认/可审计红线，与 c02 机制一致不重复造轮。

### D7. 面板触发公网模型调用的脱敏门禁与离线/私有化降级

**决策**：面板任何需公网模型的操作（润色/校对/翻译/辅助显示/解释/补引用/发起 AIMed，覆盖全部可能含 PHI/PII 的面板动作）在出网前 MUST 经 c09 redaction-gateway 的 PHI/PII 识别与脱敏判定（PHI/PII 识别脱敏引擎及 `privacy_detection_rules`/`privacy_redaction_events` 唯一 owner=c09，本期只「在公网出口预留门禁接缝 + 强制前置消费 c09 判定 + 不绕过」，不实现识别脱敏引擎，端到端真实接入随 c09（phase 9）落地）：

- 识别成功且脱敏置信度达标 → 以脱敏后文本调公网模型，对该次调用写 `audit_logs`（含 `privacy_redaction_events` 关联，命名复用 §18，由 c09 落表）。
- 识别失败 / 置信度不足 / 识别服务（redaction-gateway）不可用 → MUST 禁止公网调用，提示改用私有化模型或取消，MUST NOT 外发任何未脱敏文本。
- **相位约束（公网默认关闭）**：本期对「公网 provider」口径为：redaction-gateway 接入前不得启用公网，仅私有化/离线路径跑通闭环（§16.4/§24.9）。redaction-gateway 判定不可用时，公网 provider 按「识别服务不可用」保守处理，默认拒绝公网。
- **离线/私有化降级**：无公网或脱敏不可用时，面板技能经模型 Provider 抽象层（c03）路由到私有化模型；OFD 转 PDF/文本抽取、视觉解析等走私有化解析/识别服务（c03）。面板挂载、读取、确认网关、最近任务、删除/恢复等非推理能力本地可用，不依赖公网。OFD 转换/抽取失败时提示「暂不可处理」且不进公网模型（与 panel spec 一致）。

**备选与取舍**：

- 备选：在面板内自己做脱敏。被否——脱敏规则/识别服务是平台级横切能力（`privacy_detection_rules`/`privacy_redaction_events` 归 c09 redaction-gateway），面板内重复实现会与平台规则漂移；面板只做「门禁前置接缝」，消费 c09 既有编排。
- 备选：脱敏失败时静默回退明文调用。被否——直接违反「识别失败禁公网」红线。
- 选「强制前置门禁 + 失败禁公网 + 私有化降级」：既不重复实现脱敏，又守住红线，且离线可用。

## Risks / Trade-offs

- [面板自建 Bridge 造成与 c02 权限/审计不一致的越权] → D1 面板只调 c02 宿主桥 SDK，写回唯一经确认网关，复用 c02 的 ACL/冲突/审计落点，不另起 Bridge。
- [某条 AI 写回绕过确认直接落盘（违反医疗安全红线）] → D2 单一确认网关收口，未确认零写回；纯排版类（不改语义）才直执行；c02 侧本就不自动落盘。
- [全文级 diff 看不清 / 选区级 diff 太碎，用户误确认] → D2 diff 粒度按策略自适应（选区行内、全文分段并排、批注列表、排版摘要+关键页预览）。
- [默认覆盖原文导致医疗误改难恢复] → D3 默认策略不覆盖（替换选区外均走新文档/批注/新版本/副本），任何写回落独立 `document_versions` 可回滚。
- [最近任务跨源数据双写腐化（如翻译进度不同步）] → D4 `recent_tasks` 只存展示元数据 + `ref_type`/`ref_id` 指针，详情恢复时回源表取最新；upsert 按指针幂等。
- [删除最近任务误删用户文件] → D4 删除二次确认，默认不删文件，仅勾选「同时删除关联文档」才软删（经 ACL，进回收站，版本可追溯）。
- [选区翻译被错误路由到 c07 文件级任务（重流程）] → D5 严格按 §8.12 分流：选中一段文字→面板就地；翻译全文/文档内点击医学翻译→c07。
- [普通用户对高风险内容绕过审核完成确认] → D6 `risk_type` 与角色由服务端裁决，普通用户仅能生成草稿/提交审核。
- [AI 写回与他人协同改动冲突覆盖他人内容] → D6 乐观并发，写回带 `expectedRevision`，`revision` 不一致即阻止并要求重读上下文。
- [面板把 PHI 原文出网（脱敏不可用时）] → D7 强制前置脱敏门禁，识别失败/不可用即禁公网、切私有化模型，MUST NOT 外发未脱敏文本。
- [数字员工来源在最近任务中产生空恢复/误导] → D4 数字员工仅保留来源占位、显示「规划中」、不可恢复，不生成其历史写入/恢复实现（§22.2–22.3）。

## Migration Plan

净新增、无破坏性变更（`openspec/specs/` 为空，依赖 c01/c02/c04 已交付）。部署步骤：

0. 数据迁移：对 c01 所建最小 `recent_tasks` 表执行 ALTER 补列（`title_preview`/`status`/`created_at`/`related_document_id` + (`ref_type`,`ref_id`) 唯一约束 + `updated_at` 排序索引），并新建 `writeback_confirmations` 表；本期不重复建 `recent_tasks`，ALTER 须在 c01 建表迁移之后执行 → 验证：迁移正向/回滚均通过，本期新建表仅 `writeback_confirmations`。
1. 落地写回确认网关（四要素自适应 diff + 三按钮 + 免责声明 + 未确认零写回）→ 验证：任一面板写回操作均先弹确认面板，取消不建版本/副本。
2. 落地默认写回策略矩阵到 c02 写回方法的映射 → 验证：选区→`replaceSelection`、全文→`createNewDocument`、校对→`insertComment`、排版→新版本、翻译→译文副本，且均落 `document_versions`（对齐 §9.4/§9.6）。
3. 落地冲突/权限/高风险确认链路 + 确认记录与审计 → 验证：文档已变更阻止写回、被删/无权限禁写回、普通用户高风险仅能提交审核、确认记录含 §19.2 全字段并关联 `audit_logs`。
4. 落地医疗 AI 面板挂载与三类入口、Word/PDF/OFD 功能集、发起 AIMed/医学翻译、脱敏门禁 → 验证：右侧图标/顶部按钮/选区浮层可用，按文档类型渲染对应功能，发起 AIMed/翻译携带正确上下文，脱敏失败禁公网（对齐 §24.2）。
5. 落地最近任务统一服务（六类来源聚合、展示/分组/筛选、各来源恢复、删除二次确认与关联文档处理）→ 验证：标题/前 10 字/悬浮/倒序/分组/多选筛选正确，各来源恢复内容对齐 §6.6，删除按 §6.7。
6. 端到端：润色/校对/翻译选区 → 确认写回 → 保存新版本 → 最近任务恢复（对齐主验收闭环 §24.2）。

回滚策略：本期为独立 phase，回滚=下线面板入口、确认网关与最近任务聚合服务；c02 编辑器与 Bridge、c04 会话不受影响。已写入的 `recent_tasks` 与确认/审计记录为追加数据，回滚不删历史；已生成的 `document_versions` 保留可追溯。

## Open Questions

- ~~`risk_type` 判定来源~~（已收口）：`risk_type` 分类器与 `writeback_confirmations` 记录唯一 owner=c05（本能力服务端实现），c09 引用式消费做统一验收/审计、不重复实现分类拦截器；可复用医疗高风险词表/分类器，识别脱敏的 `privacy_detection_rules` 仍归 c09 redaction-gateway，两者职责分离。
- ~~「授权审核角色」的具体角色名与授予流程~~（已收口）：确认人角色固定为 `doctor`（医生）/ `reviewer`（授权审核），由 c01 auth-rbac 作为 RBAC 唯一真值定义并入种子（或等价 `highrisk:confirm` 权限点），本能力 spec/task 引用确定角色名，`confirmed_role` 取值可枚举为 {doctor, reviewer}。
- AI 论文排版「生成新版本」与「生成新文档」的边界：排版是否可能产出结构差异过大而更适合新文档？待结合 c08 模板排版能力定稿默认策略。
- 选区翻译就地结果是否也允许「生成副本」入口，还是仅 `replaceSelection`/展示——待与 c07 分流口径联调确认。
- ~~最近任务「在线文档 AI」来源的 `writeback_ref` 与确认记录的关联粒度（一次写回一条 vs 一次会话一条）~~（已收口）：doc_ai 的 `ref_id` 取 `writeback_ref`（单次写回确认记录 id），按操作粒度「一次写回一条」，幂等键 (`ref_type`,`ref_id`) 以 `writeback_ref` 区分同一文档的多次不同操作 / 选区，对齐 §6.6 在线文档 AI 按单次操作恢复选区 / AI 操作类型 / 输出结果 / 写回记录（见 D4 恢复内容映射表）。
