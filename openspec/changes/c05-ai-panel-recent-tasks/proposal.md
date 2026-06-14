## Why

主验收闭环要求用户在 ONLYOFFICE 中打开生成的在线 Word 后，通过右侧医疗 AI 面板完成润色 / 校对 / 翻译选区，并在用户确认后写回、保存新版本、再由最近任务恢复（PRD §24.2、§22.1）。前序 phase 已经打通 ONLYOFFICE 集成与 Bridge 读写（c02）、AIMed/RAG/引用（c04），但「人怎么在文档里驱动 AI、AI 又怎么安全地把结果还给文档、以及全平台 AI 任务如何沉淀为可恢复的历史」这一段尚未建立。本 phase（9 阶段中的第 5 阶段 ai-panel-recent-tasks）正是承接 c02/c04、把文档内 AI 交互闭环补齐的关键一环：它是医疗安全红线「AI 操作文档必须可确认、可回滚、可审计」的落地点，也是整个产品的统一历史入口。

## What Changes

- 新增**医疗 AI 右侧面板**：在 ONLYOFFICE 编辑器右侧提供医疗 AI 面板，含三类入口（右侧固定图标 / 顶部医疗空间按钮 / 选区浮层：润色·翻译·解释·补引用）；面向 Word 文档提供 P0 文档 AI 功能（全文润色、选区润色、校对、AI 论文排版、目录 / 更新目录 / 目录级别、分页、页眉页脚、段落、插入标注、辅助显示），面向 PDF/OFD 提供其 P0 子集（医学翻译、AIMed、批注 / 预览）；支持从当前文档发起 AIMed 与发起医学翻译。
- 新增**AI 写回确认机制**：任何 AI 对文档的修改在写回前必须先展示「原文 / 修改后 / 修改说明 / 影响范围」并提供「应用到文档 / 生成副本 / 取消」；落地默认写回策略矩阵（选区→替换选区、全文→生成新文档、校对→批注 / 建议、翻译→生成译文副本、排版→生成新版本）；写回时处理文档冲突与权限（文档已变更须提示重新读取上下文、文档被删除 / 无编辑权限禁止写回）。
- 新增**最近任务**完整能力：混合六类来源（AIMed 学术助手、医疗知识库问答、医疗数字员工、医学翻译、在线文档 AI 操作、模板生成文档）的历史；落地展示规则（标题按原始问题生成、首页前 10 字、悬浮显示完整标题、按最近更新时间倒序、今天 / 7 天内 / 30 天内 / 1 年内 / 全部分组、按模块多选筛选）、六类来源各自的恢复内容、删除二次确认与「是否同时删除关联文档」处理。
- 范围内但仅「发起」不「实现」：面板的「发起 AIMed」复用 c04 模式本体、「发起医学翻译」复用 c07 翻译任务系统，本 phase 只负责面板侧的发起与上下文传递，不实现模式本体与翻译任务。
- 无破坏性变更：openspec/specs/ 当前为空，本 phase 全部为新增能力，不修改任何既有能力契约。

## Capabilities

### New Capabilities

- `medical-ai-panel`：ONLYOFFICE 右侧医疗 AI 面板。覆盖面板入口（右侧固定图标 / 顶部医疗空间按钮 / 选区浮层）、Word 文档 AI 功能（润色 / 选区润色 / 校对 / 排版 / 目录 / 段落 / 插入标注 / 辅助显示等 P0 功能）、PDF/OFD AI 功能（其 P0 子集），以及从当前文档发起 AIMed 与发起医学翻译的入口与上下文传递。
- `ai-writeback-confirmation`：AI 写回确认机制。覆盖写回前展示原文 / 修改后 / 修改说明 / 影响范围 / 操作按钮，默认写回策略矩阵（选区替换 / 全文生成副本 / 校对批注 / 翻译副本 / 排版新版本），以及写回时的文档冲突与权限处理。
- `recent-tasks`：最近任务完整能力。覆盖六类来源混合，展示规则（标题 / 前 10 字 / 悬浮 / 排序 / 分组 / 筛选），六类来源各自的恢复内容，删除二次确认与关联文档处理。

### Modified Capabilities

（无：openspec/specs/ 当前为空，本 phase 全部为新增能力。）

## Impact

- **受影响服务**
  - ONLYOFFICE 编辑器与文档 Bridge API（c02 产物）：面板挂载与选区读取 / 写回均经由 Bridge，本 phase 复用其读写通道，不改其契约。
  - AIMed / RAG 服务（c04 产物）：面板「发起 AIMed」「辅助显示·补引用」按上下文调用其会话与引用能力。
  - 医学翻译模块（c07）：面板「发起医学翻译」按 §8.12 分流规则将「文档内点击医学翻译 / 翻译全文」路由至翻译任务系统；本 phase 仅发起，不实现翻译任务。
  - 校对 / 润色 / 排版的文档处理能力（参考 §15）作为面板技能被调用。
- **受影响数据表**（PRD §18）
  - `recent_tasks`（建表 owner=c01）：本 phase **仅 ALTER 补列**（`title_preview` / `status` / `created_at` / `related_document_id` + (`ref_type`,`ref_id`) 唯一约束 + `updated_at` 排序索引），绝不重复建表；用于承载六类来源恢复编排、混合展示、排序 / 分组 / 筛选、删除与历史同步。
  - `writeback_confirmations`（本 phase **新建**，与 `risk_type` 分类器唯一 owner=c05；c09 引用式消费做统一验收/审计、不新造独立 confirmation 表）：写回确认记录，落 §19.2 全字段（confirmation_id / subject（多态键，`subject_type` ∈ {document, message, translation_job}、`subject_id` 承载 document_id / message_id / translation_jobs.job_id，泛化覆盖文档写回与三类下发前确认）/ confirmed_by / confirmed_role（取值 ∈ {doctor, reviewer}）/ confirmed_at / confirmed_scope / risk_type / before_content_hash / after_content_hash / confirmation_action / audit_log_id / tenant_id）。
  - `documents` / `document_versions` / `document_permissions`（建表归 c01）：写回生成新版本 / 副本、权限校验，本 phase 仅消费 / 写入（写回经 c02 写回入参链路落库）。
  - `document_events`（建表归 c01）：写回事件留痕，本 phase **不产生、不消费 `document_events`**，仅经把 `writebackSource` 填入 c02 写回入参，间接触发 c02 **唯一产生** `ai_writeback` 事件（该事件的**唯一消费方为 c03**，做 RAG 重索引）。c05 既非 `ai_writeback` 的产生方也非其消费方，MUST NOT 自行写 `document_events`、不产生任何 `event_type`、不读取消费任何 `event_type`（与 design「写回经 c02 落 document_events」、c02 save-callback-versioning「ai_writeback 唯一产生方=c02」、c03 document-parsing「ai_writeback 唯一消费方=c03」及 c01 document_events 6 类 event_type 唯一产生方 / 消费方契约一致）。
  - `conversations` / `messages` / `citations`（建表归 c04）：面板发起 AIMed 与问答历史的恢复来源，本 phase 仅消费。
  - `translation_jobs`（建表归 c07）：从面板发起医学翻译的任务来源，本 phase 仅触发，不落库。
  - `audit_logs`（建表归 c01）/ `privacy_redaction_events`（建表归 c09）：写回确认链路与公网脱敏门禁的审计 / 留痕落点，本 phase 仅写入 / 关联。
- **对其它 phase 的依赖**
  - 依赖 c02（onlyoffice-bridge）：编辑器集成与读写选区 / 写回的 Bridge 能力。
  - 依赖 c04（aimed-rag-citation）：面板发起 AIMed 与引用 / 辅助显示的会话能力。
  - 与 c07（medical-translation）协作：本 phase 提供发起入口，翻译任务系统由 c07 实现。
- **医疗安全 / 合规 / 人工确认 / 脱敏 / 审计**
  - 人工确认：所有 AI 写回默认是草稿 / 辅助建议，必须经写回确认机制方可应用；涉及诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息的高风险写回须进入医生（`doctor`）或授权审核（`reviewer`）角色的确认链路（§19.2，角色定义为 c01 auth-rbac 唯一真值），普通用户只能生成草稿 / 提交审核。高风险确认链路的确认 subject 泛化为 (`subject_type`, `subject_id`) 多态键，覆盖文档写回（`subject_type=document`、`subject_id=document_id`）与三类下发前确认——AIMed 答案（c04）/ 知识库问答（kb_qa，c06）取 `subject_type=message`、`subject_id=messages.message_id`（回源 c04 `conversations`/`messages`），医学翻译文书（c07）取 `subject_type=translation_job`、`subject_id=translation_jobs.job_id`（回源 c07 `translation_jobs`、MUST NOT 为取键向 c04 写 message 行）；三类产生方复用同一 `writeback_confirmations` 表与同一确认链路并按 `subject_type` 区分；c09 引用式收口做统一验收 / 审计（收口枚举覆盖该三类、c07 按 `subject_type=translation_job` 核对）。
  - 可回滚 / 可审计：写回以新版本 / 副本 / 批注为默认策略避免直接覆盖原文，结合 `document_versions` 实现可回滚；确认动作写入确认记录与 `audit_logs`，实现可审计。
  - 脱敏：面板触发的、需调用公网模型的 AI 操作（润色 / 校对 / 翻译 / 辅助显示 / 解释 / 补引用 / 发起 AIMed）须遵循公网调用前 PHI/PII 识别与脱敏红线（§19.4）；识别失败 / 置信度不足 / 识别服务不可用时禁止调用公网模型，可降级私有化模型。PHI/PII 识别脱敏引擎 redaction-gateway 唯一 owner=c09，本 phase 不实现，仅在公网出口预留门禁接缝、强制前置消费 c09 判定。本期相位约束：redaction-gateway 接入前默认关闭公网 provider，仅私有化 / 离线路径跑通闭环（§16.4 / §24.9）。
  - 免责声明：面板内的医学回答与生成 / 写回内容须展示 §19.3 医疗免责声明。
