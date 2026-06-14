## ADDED Requirements

### Requirement: 写回前确认面板四要素展示
任何 AI 对文档的修改在写回前，系统 MUST 先展示确认面板，包含四要素：原文、修改后、修改说明、影响范围，并提供操作按钮「应用到文档 / 生成副本 / 取消」（§9.6）。在用户未点击确认操作前，系统 MUST NOT 对文档执行任何写回。确认面板 MUST 展示 §19.3 医疗免责声明，提示 AI 生成内容为草稿 / 辅助建议。

#### Scenario: 写回前展示四要素与操作按钮
- **WHEN** 任一面板 AI 操作产生待写回结果
- **THEN** 系统展示确认面板，逐项呈现原文、修改后、修改说明、影响范围，并提供「应用到文档 / 生成副本 / 取消」三个按钮
- **AND** 确认面板底部 MUST 展示医疗免责声明

#### Scenario: 用户取消不改动文档
- **WHEN** 用户在确认面板点击「取消」
- **THEN** 系统丢弃待写回结果，文档内容保持不变，且不创建版本或副本

#### Scenario: 未确认前禁止写回
- **WHEN** AI 结果已生成但用户尚未点击「应用到文档」或「生成副本」
- **THEN** 系统 MUST NOT 调用任何 Bridge 写回能力修改文档

### Requirement: 默认写回策略矩阵
系统 SHALL 按 §9.6 默认写回策略矩阵决定每类操作的写回方式：选区修改→可替换选区（replaceSelection）；全文修改→生成新文档（createNewDocument）；校对建议→以批注 / 建议形式插入（insertComment）；排版结果→生成新版本（document_versions）；PPT 生成不在 V1.0 范围内。该矩阵 MUST 作为默认值，避免直接覆盖原文以保证可回滚。**翻译结果（文件级译文副本）的产出与落库确认归 c07**（`source=translation`）：本能力 §9.6 矩阵仅覆盖面板侧选区 / 全文 / 校对 / 排版 / 补引用等写回确认，文件级翻译副本由 c07 medical-translation 产出 `result_document_id` / `result_version_id` 并执行文件级落库确认（对齐 c07「译文打开回 ONLYOFFICE 与写入文档中心可确认可审计」Requirement），本能力 MUST NOT 对文件级译文副本二次确认、亦不另起一套文件级翻译落库确认 UI；面板侧选区短文本翻译（走 replaceSelection）仍归本矩阵。

#### Scenario: 选区修改替换选区
- **WHEN** 用户对选区润色 / 翻译结果点击「应用到文档」
- **THEN** 系统通过 replaceSelection 将结果写回原选区

#### Scenario: 全文修改生成新文档
- **WHEN** 用户对全文润色结果点击「应用到文档」
- **THEN** 系统通过 createNewDocument 生成新文档，MUST NOT 覆盖原文档

#### Scenario: 校对结果以批注插入
- **WHEN** 用户对校对结果点击「应用到文档」
- **THEN** 系统通过 insertComment 以批注 / 建议形式插入，不直接改写正文

#### Scenario: 排版结果生成新版本
- **WHEN** 用户对 AI 论文排版结果点击「应用到文档」
- **THEN** 系统生成新的 document_version，保留旧版本以支持回滚

#### Scenario: 用户改选生成副本
- **WHEN** 用户对任一结果点击「生成副本」
- **THEN** 系统生成原文档的副本并将结果写入副本，原文档保持不变

#### Scenario: 文件级译文副本不经本网关二次确认
- **WHEN** 用户在文档内发起文件级医学翻译（翻译全文 / 文档内点击医学翻译）并由 c07 产出译文副本
- **THEN** 文件级译文副本（`source=translation`）的产出与落库确认归 c07，本能力 §9.6 矩阵 MUST NOT 对其二次确认，亦不另起文件级翻译落库确认 UI
- **AND** 仅面板侧选区短文本翻译走本矩阵的 replaceSelection 确认路径

### Requirement: 写回时文档冲突处理
写回时若文档自上下文读取后已发生变更，系统 MUST 提示用户重新读取上下文，禁止基于过期上下文盲写（§9.9）。保存回调失败时 MUST 自动重试，失败后告警；ONLYOFFICE 保存回调成功率 MUST ≥ 99%。多人同时编辑时 MUST 复用 ONLYOFFICE 协作能力并由服务端保留版本。

#### Scenario: 文档已变更提示重新读取上下文
- **WHEN** 用户点击「应用到文档」时检测到文档内容自 AI 读取上下文后已被他人或自身修改（如版本号 / 内容哈希不一致）
- **THEN** 系统 MUST 阻止本次写回并提示用户重新读取上下文后再生成
- **AND** 系统 MUST NOT 用过期结果覆盖已变更的文档

#### Scenario: 保存回调失败自动重试
- **WHEN** 写回后触发的保存回调失败
- **THEN** 系统自动重试保存，重试仍失败则告警并保留待保存内容，不丢失用户已确认的结果

### Requirement: 写回权限校验
写回前系统 MUST 校验文档存在性与当前用户对该文档的编辑权限（document_permissions / ACL，按 tenant_id / user_id / role / acl 过滤）。文档被删除时 MUST 禁止写回并提示文档不存在；无编辑权限时 MUST 禁止写回，仅允许查看或复制（生成副本）（§9.9）。

#### Scenario: 文档被删除禁止写回
- **WHEN** 用户点击「应用到文档」时文档已被删除
- **THEN** 系统 MUST 禁止写回并提示「文档不存在」

#### Scenario: 无编辑权限仅允许复制
- **WHEN** 当前用户对目标文档无编辑权限
- **THEN** 系统 MUST 禁止「应用到文档」，仅允许「生成副本」或查看
- **AND** 权限校验 MUST 按 tenant_id / user_id / role / acl 过滤，越权写回 MUST 被拒绝

### Requirement: 高风险写回的人工确认链路
涉及诊疗、用药、医嘱、临床文书或患者个体信息的高风险写回，系统 MUST 进入医生（`doctor`）或授权审核（`reviewer`）角色的确认链路（§19.2）。`risk_type` 高风险判定与确认角色裁决的唯一 owner 为本能力（c05）服务端，c09 仅引用消费本能力的判定与 `writeback_confirmations` 记录做统一验收与审计。确认人 MUST 具备 `doctor` 或 `reviewer` 角色（角色定义为 c01 auth-rbac 唯一真值，本能力引用其确定角色名）；普通用户只能生成草稿或提交审核，MUST NOT 完成最终确认。`confirmed_role` 取值 MUST 可枚举为 `doctor` / `reviewer`。

#### Scenario: 普通用户高风险写回仅能提交审核
- **WHEN** 普通用户对本能力服务端识别为高风险（risk_type 命中诊疗 / 用药 / 医嘱 / 临床文书 / 患者信息）的内容点击「应用到文档」
- **THEN** 系统 MUST 阻止其直接确认，仅允许生成草稿或提交 `doctor` / `reviewer` 角色审核

#### Scenario: 授权角色完成最终确认
- **WHEN** 具备 `doctor` 或 `reviewer` 角色的用户对高风险写回点击确认
- **THEN** 系统允许写回，并记录 confirmed_by / confirmed_role（取值 ∈ {doctor, reviewer}）/ risk_type 至确认记录

### Requirement: AIMed 答案 / 知识库问答 / 医学翻译文书下发前的高风险确认（(subject_type, subject_id) 泛化键）
高风险确认链路 MUST NOT 仅覆盖文档写回（document 路径），还 MUST 覆盖非文档型 message / 文书级内容的下发前确认。为消除单一 `message_id` 键对各类产生方稳定标识来源的强耦合，本能力确认 subject 泛化为 (`subject_type`, `subject_id`) 多态键，`writeback_confirmations` 的 subject 列据此承载三类取值，三者取值与回源表 MUST 唯一对应：

- **文档写回**：`subject_type=document`、`subject_id`=`document_id`（回源 `documents`）。
- **AIMed 答案（c04）/ 知识库问答（kb_qa，c06）**：`subject_type=message`、`subject_id`=`messages.message_id`（回源 c04 所建 `conversations` / `messages`；这两类本就落 c04 会话 / 消息表，确认键直取其行主键 `message_id`）。
- **医学翻译文书（c07）**：`subject_type=translation_job`、`subject_id`=`translation_jobs.job_id`（回源 c07 `translation_jobs`）。c07 译文文书 MUST 以 `translation_job` 为确认 subject、MUST NOT 为取确认键而向 c04 `conversations` / `messages` 写一条 message 行；本能力消费 c07 稳定的 `translation_jobs.job_id` 作为 `subject_id`、MUST NOT 自造标识。

本能力（c05）确认链路 MUST 显式承认三类产生方 / 消费方：**AIMed 答案（c04）、知识库问答（kb_qa，c06）、医学翻译文书（c07）**——均为下发前命中高风险时 MUST 前置消费本链路的医学文书（本能力为确认 owner，c04 / c06 / c07 为生产方挂载、各自前置消费本链路）。当本能力服务端 `risk_type` 分类器将待下发的 AIMed 答案 / 知识库问答（kb_qa）答案 / 医学翻译文书识别为高风险（命中诊疗、用药、医嘱、临床文书或患者个体信息）时，系统 MUST 在内容下发前进入与文档写回同一条确认链路，按 `confirmed_role` 裁决（取值 MUST 可枚举为 `doctor` / `reviewer`）：普通用户只能生成草稿或提交审核，MUST NOT 完成最终确认与下发；具备 `doctor` 或 `reviewer` 角色者方可确认下发。确认记录 MUST 以 (`subject_type`, `subject_id`) 为键落 `writeback_confirmations`（含 `confirmed_by` / `confirmed_role` / `risk_type` / `audit_log_id`），使 `writeback_confirmations` 的 document / message / translation_job 多态 subject 路径均有可验收落点。`risk_type` 判定与 `writeback_confirmations` 记录唯一 owner 为本能力（c05），c09 仅引用消费做统一验收与审计（三类产生方在 c09 收口验收枚举中口径一致，c07 译文确认按 `subject_type=translation_job` 核对）（§19.2、§19.4）。

#### Scenario: 普通用户高风险 message 级输出仅能提交审核
- **WHEN** 普通用户对本能力服务端识别为高风险的 AIMed 答案、知识库问答（kb_qa）答案或医学翻译文书请求下发
- **THEN** 系统 MUST 阻止其直接下发，仅允许生成草稿或提交 `doctor` / `reviewer` 角色审核，MUST NOT 在未经授权角色确认前将高风险内容下发给用户

#### Scenario: 授权角色确认后下发并落 subject 多态确认记录
- **WHEN** 具备 `doctor` 或 `reviewer` 角色的用户对高风险 AIMed 答案 / 知识库问答（kb_qa）答案（`subject_type=message`）或医学翻译文书（`subject_type=translation_job`）点击确认下发
- **THEN** 系统允许下发，并以 (`subject_type`, `subject_id`) 为键向 `writeback_confirmations` 生成确认记录，含 confirmed_by / confirmed_role（取值 ∈ {doctor, reviewer}）/ risk_type / audit_log_id
- **AND** 该确认动作 MUST 写入 audit_logs 并以 audit_log_id 关联确认记录

#### Scenario: 三类产生方复用同一确认链路并按 subject_type 区分
- **WHEN** AIMed 答案（c04，subject_type=message）、知识库问答（kb_qa，c06，subject_type=message）、医学翻译文书（c07，subject_type=translation_job）中任一在下发前被本能力 `risk_type` 分类器识别为高风险
- **THEN** 三者 MUST 进入本能力同一条确认链路、复用同一 `writeback_confirmations` 表、以各自 (`subject_type`, `subject_id`) 为键落确认记录
- **AND** c07 译文文书 MUST 以 `subject_type=translation_job`、`subject_id=translation_jobs.job_id` 落键，MUST NOT 向 c04 `conversations` / `messages` 写 message 行取键
- **AND** 本能力对三类产生方一视同仁裁决（confirmed_role ∈ {doctor, reviewer}），c09 收口验收 MUST 覆盖该三类产生方

### Requirement: 写回确认记录与审计留痕
每次写回确认，系统 MUST 生成确认记录并写入 audit_logs（§19.2、§19.4）。确认记录 MUST 包含字段：confirmation_id、subject（多态键，由 `subject_type` ∈ {document, message, translation_job} 与 `subject_id` 承载 document_id / message_id / translation_jobs.job_id，与 §19.2「document_id / message_id」对齐并泛化覆盖译文文书）、confirmed_by、confirmed_role、confirmed_at、confirmed_scope、risk_type、before_content_hash、after_content_hash、confirmation_action、audit_log_id，并 MUST 额外承载在线文档 AI（doc_ai）操作的 §6.6 恢复载体字段：`operation_type`（AI 操作类型，枚举：全文润色 / 选区润色 / 校对 / 选区翻译 / 补引用 / 插入标注 / AI 论文排版）与 `output_version_id`（指向本次写回所落 `document_versions` 的新版本 / 副本，承载「输出结果」）。`confirmed_scope` 承载本次操作的「选区」定位、`operation_type` 承载「AI 操作类型」、`output_version_id` 承载「输出结果」、确认记录本体即「写回记录」，使该确认记录可作为 `recent_tasks.ref_id`（=`writeback_ref`，指向本操作记录而非裸 `document_id`）的回源载体、无需依赖内容哈希即可还原 §6.6 doc_ai 恢复内容。document 级写回确认以 `subject_type=document`、subject_id=`document_id` 为键，message 级文书下发确认以 `subject_type=message`、subject_id=`message_id` 为键，医学翻译文书以 `subject_type=translation_job`、subject_id=`translation_jobs.job_id` 为键，三者复用同一 `writeback_confirmations` 表与同一确认链路。写回以新版本 / 副本 / 批注为默认策略，结合 document_versions 实现可回滚。

#### Scenario: 确认写回生成完整确认记录
- **WHEN** 用户完成一次写回确认（应用到文档 / 生成副本）
- **THEN** 系统生成确认记录，包含上述全部字段，其中 before_content_hash 与 after_content_hash MUST 分别对应写回前后内容哈希
- **AND** 确认记录 MUST 记录本次的 `operation_type`（AI 操作类型）、`confirmed_scope`（选区）与 `output_version_id`（指向写回所落 `document_versions`），使后续 doc_ai 恢复可由非哈希字段还原选区 / AI 操作类型 / 输出结果 / 写回记录
- **AND** 系统将该确认动作写入 audit_logs，并以 audit_log_id 关联确认记录

#### Scenario: 写回可回滚
- **WHEN** 用户需要撤销一次已应用的写回
- **THEN** 系统可基于 document_versions 回滚到写回前版本，确认记录与审计日志保留不被删除
