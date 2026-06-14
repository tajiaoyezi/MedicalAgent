# Save Callback Versioning Specification

## Purpose

ONLYOFFICE 保存回调链路：callbackUrl → Document Service 下载 → 写对象存储 → 创建 document_version → 更新元数据 → 产生 `document_events` 触发 c03 异步解析；版本 source/file_hash 溯源、失败重试、协作保存与 AI 写回冲突处理。

## Requirements

### Requirement: 保存回调链路下载并落库为新版本
系统 SHALL 在 ONLYOFFICE 编辑器触发 callbackUrl 后，由 Document Service 下载最新文件、写入对象存储（MinIO/S3）、创建 document_version、更新文档元数据，并产生 `document_events` 以触发下游重解析与索引。整条链路 MUST 按顺序执行，且 MUST 在普通 docx 上端到端 ≤ 10 秒完成。系统 MUST NOT 在文件成功落对象存储前就对外宣告保存成功。重解析与索引由唯一消费方 c03 消费 c02 产生的事件后异步发起，c02 不在 c03 之外另立第二条解析触发路径。

#### Scenario: 编辑保存生成新版本
- **WHEN** 用户编辑 docx 后 ONLYOFFICE 通过 callbackUrl 通知文档已就绪（status 表示需保存）
- **THEN** 系统下载最新文件、写入对象存储、创建一条新的 document_version 并更新文档元数据（最新版本指针、更新时间）

#### Scenario: 落库后触发异步解析与索引
- **WHEN** 新版本成功写入对象存储与数据库
- **THEN** 系统产生对应 `document_events`（`save_new_version` 或 `ai_writeback`）供唯一消费方 c03 消费后异步重建 RAG 索引，且保存响应不被解析耗时阻塞

#### Scenario: 落盘失败不报成功
- **WHEN** 文件下载或写入对象存储失败
- **THEN** 系统不创建 document_version、不对外返回保存成功，并进入失败重试流程

### Requirement: 版本记录文件 hash、保存人、保存时间与来源
系统 SHALL 为每个 document_version 记录 version_id、document_id、file_hash、saved_by、saved_at 以及 source。source MUST 取值于 {user_edit, ai_writeback, translation, import, template}。系统 MUST 用 file_hash 标识版本内容，以支持去重判断与回滚。

#### Scenario: 用户编辑产生 user_edit 版本
- **WHEN** 保存来源为用户在编辑器中的直接编辑
- **THEN** 新版本 source=user_edit，并记录 file_hash、saved_by、saved_at

#### Scenario: AI 写回产生 ai_writeback 版本
- **WHEN** 保存由经用户确认的 AI 写回触发
- **THEN** 新版本 source=ai_writeback，可据该版本回滚到写回前内容

#### Scenario: 不同来源版本可区分
- **WHEN** 同一文档先后经历翻译副本、模板创建与导入
- **THEN** 对应版本 source 分别记录为 translation / template / import，便于审计与追溯

### Requirement: 重新解析与索引触发事件
c02 保存回调链路 SHALL 是 PRD §10.6 中 `save_new_version` 与 `ai_writeback` 两类 `document_events` 的唯一产生方：每当保存回调成功创建一条新的 `document_version` 时，c02 MUST 按回调上下文产生一条 c01 契约形态的 `document_events`，其 `event_type` 取自 {`save_new_version`, `ai_writeback`}——当回调上下文不含 `writebackSource` 时为 `save_new_version`，当回调上下文带 `writebackSource`（AI 写回）时为 `ai_writeback`；事件 MUST 携带 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)` 稳定契约字段（owner=c01）。产生事件即为 c02 的重解析/索引触发职责终点：该事件由唯一消费方 c03 消费后异步发起重解析与索引，c02 MUST NOT 在 c03 之外另立第二条重解析触发路径。事件产生与保存交互 MUST 解耦、MUST 不阻塞用户的保存/编辑交互；由 c03 消费触发的解析状态刷新 MUST ≤ 3 秒可见。c03 对同一新版本的重解析作业 MUST 以 `(document_id, version_id)` 幂等去重，杜绝重复或漏触发。

PRD §10.6 列出的其余触发事件不在 c02 产生，由各自 owner 产生：上传成功 `upload_success`（c01）、翻译完成 `translation_done`（c07）、模板创建 `template_created`（c08）、手动重建索引 `manual_reindex`（产生方=c06、c03 消费）；c02 仅产生 `save_new_version` / `ai_writeback` 两类并保证其可验收。上述全部 6 类 `document_events`（含 c02 产生的 `save_new_version` / `ai_writeback`）的唯一重解析/索引消费方为 c03：c02 产生事件即完成职责，由 c03 消费该事件并以 `(document_id, version_id)` 幂等去重后异步发起重解析与索引，避免对同一新版本重复触发或漏触发；c02 MUST NOT 自称 c03 之外的第二重解析触发路径。

#### Scenario: 保存新版本产生 save_new_version 事件并触发重新索引
- **WHEN** 保存回调成功创建一条新的 `document_version` 且回调上下文不含 `writebackSource`
- **THEN** 系统产生一条 `event_type=save_new_version` 的 `document_events`（含 `document_id`、`version_id`、`tenant_id`、`occurred_at`、`payload`），该事件由唯一消费方 c03 消费后按 `(document_id, version_id)` 幂等发起一次重解析与索引、将解析状态置为进行中，c02 不另行直接触发解析

#### Scenario: AI 写回保存产生 ai_writeback 事件并触发重新索引
- **WHEN** 保存回调成功创建一条新的 `document_version` 且回调上下文带 `writebackSource`（AI 写回）
- **THEN** 系统产生一条 `event_type=ai_writeback` 的 `document_events`（含 `document_id`、`version_id`、`tenant_id`、`occurred_at`、`payload`），该事件由唯一消费方 c03 消费后按 `(document_id, version_id)` 幂等发起一次重解析与索引，c02 不另行直接触发解析

### Requirement: 保存回调失败自动重试与告警
系统 SHALL 在保存回调处理失败时自动重试，重试仍失败后触发告警并记录失败事件。系统 MUST 暴露 ONLYOFFICE 保存回调成功率指标且目标 ≥ 99%。重试 MUST 是幂等的，MUST NOT 因重复回调而产生重复的 document_version（以 file_hash 去重）。

#### Scenario: 回调失败后自动重试成功
- **WHEN** 一次保存回调因下载或落库瞬时失败
- **THEN** 系统按重试策略自动重试，成功后正常生成单一版本

#### Scenario: 重试耗尽后告警
- **WHEN** 保存回调在达到最大重试次数后仍失败
- **THEN** 系统触发告警、记录失败事件到审计，并向用户提示保存未成功

#### Scenario: 重复回调不产生重复版本
- **WHEN** 同一保存内容的回调被重复投递（相同 file_hash）
- **THEN** 系统识别为同一内容，仅保留一条 document_version，不重复落库

### Requirement: 多人协作与服务端版本保留
系统 SHALL 沿用 ONLYOFFICE 协作能力处理多人同时编辑，并由服务端在保存时保留版本，确保协作产生的内容变更被纳入版本历史。系统 MUST 保证协作保存同样写入 document_version 与审计。

#### Scenario: 多人协作后保存保留版本
- **WHEN** 多名用户在同一文档中协作编辑后触发保存
- **THEN** 系统通过 ONLYOFFICE 协作合并后生成新的 document_version，并记录保存人与时间

### Requirement: AI 写回时文档冲突处理
系统 SHALL 在执行 AI 写回前校验文档是否自上下文读取后已发生变更（基于版本/file_hash 或编辑会话状态）。若文档已变更，系统 MUST 提示用户重新读取上下文并阻止基于陈旧上下文的写回，以避免覆盖他人改动。

#### Scenario: 文档已变更时提示重新读取上下文
- **WHEN** AI 写回所依据的上下文版本与文档当前版本不一致
- **THEN** 系统拒绝该次写回并提示用户「文档已更新，请重新读取上下文」

#### Scenario: 上下文一致时正常写回
- **WHEN** AI 写回所依据的版本与文档当前版本一致且用户已确认
- **THEN** 系统执行写回并生成新的 ai_writeback 版本

### Requirement: 文档被删除或无编辑权限时禁止改正文写回
系统 SHALL 在保存与改正文/新建类写回前校验文档存在性与调用者编辑权限：文档已被删除时 MUST 禁止写回并提示文档不存在；调用者无「可编辑」权限时 MUST 禁止改正文/新建类写回（replaceSelection / insertText / appendSection / insertCitation / applyStyle / createNewDocument / createPresentation / saveDocument）。被拒后的兜底动作按 §10.4 区分：「可评论」用户仍可查看与复制文本，「可查看」用户仅允许查看（含受权限控制的下载）、MUST NOT 复制文本（复制文本属「可评论」及以上专属）。说明：批注类 insertComment 最低权限为「可评论」，不属本要求禁止集合（其放行规则见 document-bridge-api「Bridge 写回类方法定义文档修改入口」）。被拒事件 MUST 写入审计。

#### Scenario: 文档已删除禁止写回
- **WHEN** 写回目标文档已被删除
- **THEN** 系统拒绝写回并提示「文档不存在」，不创建任何版本，并记录审计

#### Scenario: 无编辑权限禁止改正文写回
- **WHEN** 调用者仅有「可查看」权限，或仅有「可评论」权限并触发改正文/新建类写回
- **THEN** 系统禁止该改正文/新建类写回并记录一条审计日志；被拒后「可查看」用户仅允许查看（含受权限控制的下载）不含复制文本，「可评论」用户仍可查看、复制文本并经 insertComment 插入批注
