## Why

医学文献、报告、论文、指南与院内资料的翻译是医院 / 医生 / 科研人员的高频刚需，但通用机翻无法保证医学术语一致、版式还原与可溯源，且整篇文档（Word / PPT / PDF / 扫描件 / 图片）翻译必须异步处理才能在 POC 环境稳定演示。本 change 落地 PRD §13 与 §22.1「医学翻译文件级异步任务 / 版式还原 / 双语对照输出」三项 P0 必做，交付一套文件级异步医学翻译系统，补齐主验收闭环中「医学翻译选区 / 整篇翻译」与「文档中心 / ONLYOFFICE / AIMed 多入口发起翻译」的能力。

在 9 阶段顺序中，本阶段为第 7 阶段 `medical-translation`，位于 `knowledge-admin` 之后、`template-center` 之前，依赖 c02（`onlyoffice-bridge`，用于从当前 ONLYOFFICE 文档发起翻译并回写打开）与 c03（`model-and-parse`，复用文件上传解析与文档视觉解析服务、模型 Provider 抽象与脱敏链路）。

## What Changes

- 新增文件级异步医学翻译系统：支持 Word（doc/docx）、PPT（ppt/pptx）、文本型 PDF、扫描 PDF、图片（png/jpg）以及 OFD（转换后）的整篇翻译；单次最多 10 个文档、单文档 ≤ 50MB、暂不支持加密文档。
- 新增多入口与上传来源：左侧导航、文档中心右键、AIMed 答案操作栏、ONLYOFFICE 医疗 AI 面板均可发起；上传来源覆盖本地文件、我的 / 团队文档中心文件、从当前 ONLYOFFICE 文档发起（面板侧取数与跳转归 c05，c07 接收传入 document_id 创建任务并返回翻译页路由）。
- 新增待翻译列表（文档名称 / 格式 / 大小 / 上传状态 / 解析状态 / 翻译状态 / 操作）与添加、移除、预览原文操作。
- 新增翻译设置与三种输出模式：翻译引擎、语言方向、术语库 / 语料库、是否保留原文 / 图片 / 表格 / 双语对照；输出模式为「仅译文 / 左右对照 / 逐段（上下）对照」。
- 新增翻译任务状态机：排队中 → 解析中 → 翻译中（带进度）→ 排版中 → 翻译成功 / 翻译失败 / 已取消。
- 新增版式还原：Word/PPT 保留标题层级、表格、图片、页眉页脚、页码、公式与角标；文本型 PDF 支持页级预览、段落级翻译、表格识别、图片位置保留；扫描 PDF / 图片走文档视觉解析服务识别文字、版面、表格与图片位置后还原。
- 新增术语一致性：同一任务内术语翻译保持一致，优先使用用户所选术语库，支持演示术语库 / 演示语料库。
- 新增术语库 / 语料库最小后台配置与翻译管理最小入口（§17.6 / §17.8 / §24.6）：管理员创建 / 编辑术语库与术语条目、新增 / 导入语料库、启停与绑定翻译用途（按 tenant_id 隔离 + 写 audit_logs），并提供翻译引擎只读路由视图 / 任务队列查看 / 失败重试 / 最小质量反馈入口；落实 §0.3「配置→连通性测试→前台按配置生效」闭环。本期为最小形态，不做术语库可视化编辑器、批量术语导入器与质量评测闭环（属 §22.2/§22.3 延期）。
- 新增翻译历史与操作：预览、下载、删除、重新翻译、打开到 ONLYOFFICE、查看原文 / 任务详情 / 失败原因。
- 新增失败原因与重译：文件加密 / 损坏 / 无法解析 / 视觉解析失败 / 版式无法重建时展示明确失败原因，并允许失败任务重新提交。

破坏性变更：无（全部为新增能力，不修改既有 spec 行为）。

## Capabilities

### New Capabilities
- `medical-translation`: 文件级异步医学翻译系统，覆盖入口与上传（格式 / 限制 / 来源）、待翻译列表、翻译设置与输出模式（仅译文 / 左右对照 / 逐段对照）、任务状态机、版式还原（Word/PPT、文本型 PDF、扫描 PDF / 图片走视觉解析）、术语一致性（术语库 / 语料库）、翻译历史与操作、从当前 ONLYOFFICE 文档发起翻译、失败原因与重译。

### Modified Capabilities
（无：`openspec/specs/` 当前为空，本 change 不修改任何既有能力的需求。）

## Impact

受影响的服务与数据表（参考 PRD §18 命名）：
- 翻译核心（建表归 c07）：`translation_jobs`（任务与状态机 / 失败原因 / `output_mode` / `layout_style` / `output_format`）、`translation_segments`（段落级原文 / 译文与版式定位）、`term_bases` / `terms` / `corpora`（术语库 / 语料库与术语一致性）由本 change 建表。
- 文档与解析（建表归 c01 / c03，c07 消费）：`documents` / `document_versions`（上传来源、生成译文文件、打开到 ONLYOFFICE，建表归 c01，c07 写入新版本）、`document_parse_jobs` / `document_visual_parse_results`（文本解析与扫描件 / 图片视觉解析结果，建表归 c03，c07 只读复用）。
- 模型与脱敏：`model_providers` / `model_routes` / `visual_parse_providers`（翻译引擎与视觉解析的公网 / 私有化路由与 fallback，建表归 c03，c07 只读消费）、`privacy_detection_rules` / `privacy_redaction_events`（PHI/PII 识别脱敏引擎 redaction-gateway 与表建表归 c09；c07 不实现识别脱敏，仅在公网出口预留门禁接缝并消费 c09 判定，事件由门禁写入）、`audit_logs`（任务全链路审计，由 c01 建表，c07 写入）。
- 反馈（建表归 c04，c07 写入）：`feedbacks`（§17.6 翻译质量反馈，建表 owner=c04 aimed-rag-citation 所泛化的多来源反馈表，承载 `subject_type` ∈ {message, translation_job} + `subject_id`；c07 仅作为写入侧消费方落最小反馈记录，按 `subject_type=translation_job` + `subject_id=translation_jobs.job_id` 关联、按 `tenant_id` 隔离，不建表不改表结构）。
- 术语库 / 语料库（建表归 c07）：`term_bases` / `terms` / `corpora` 由本 change 建表并承载术语一致性运行时使用与最小后台配置（创建 / 编辑 / 导入 / 启停 / 绑定）。

对其它 phase 的依赖：依赖 c02 `onlyoffice-bridge`（从当前 ONLYOFFICE 文档发起翻译、译文打开回 ONLYOFFICE）与 c03 `model-and-parse`（文件上传解析、文档视觉解析服务、模型 Provider 抽象层与脱敏链路）；为后续 `security-evidence` 阶段提供翻译任务的审计与验收数据。

医疗安全 / 合规 / 人工确认 / 脱敏与审计影响：
- 译文为机器辅助产物，默认作为草稿 / 辅助，需人工确认；翻译结果页须展示医疗免责声明，不作为临床定论。
- PHI/PII 识别按 §19.4 / §0.3 发生在两个时点，均由 c09 redaction-gateway 唯一实现、c07 仅前置消费、不自实现识别脱敏：(1) 上传时（上传闸）——翻译文件在持久化入库或送模型前 MUST 经 c09「上传时 PHI / PII 识别与『阻止上传』策略执行」契约识别并按策略处理（识别并提示 / 脱敏后送模型 / 阻止上传），策略=阻止上传命中则拒绝入库；(2) 调用模型时（出网闸）——调用公网翻译模型或公网视觉解析前先经脱敏门禁做 PHI/PII 识别与脱敏。出网闸：识别失败、脱敏置信度不足、服务不可用或 c09 判定不可用时禁止调用公网模型（默认拒绝公网），须切换私有化模型 / 私有化解析。本期口径：redaction-gateway 未接入前不得启用公网 provider，仅私有化 / 离线路径跑通闭环（§16.4 / §24.9）。
- 多租户隔离与 ACL：上传来源（我的 / 团队文档中心、当前 ONLYOFFICE 文档）须按 tenant_id / 文档级 ACL 过滤，禁止越权翻译。
- 译文打开回 ONLYOFFICE 或写入文档中心遵循「可确认、可回滚、可审计」原则，生成新版本而非覆盖原文。
- 全链路（创建 / 解析 / 翻译 / 排版 / 失败 / 重译 / 下载 / 打开）写入 `audit_logs`，失败原因可追溯。
