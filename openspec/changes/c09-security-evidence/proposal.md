## Why

MedOffice AI 处理医学论文、患者个体信息与院内知识资产，前 8 个 phase（c01–c08）已交付门户、ONLYOFFICE、模型与解析、AIMed RAG、AI 面板、知识库、医学翻译、模板中心等横切与业务能力，但这些能力各自只完成了功能闭环，安全合规红线与可验收证据尚未被统一收口。本 phase 作为 9 阶段的收尾环（依赖 c01–c08），把 PRD §19（安全与合规）、§20（可观测性与评测）、§21（性能）落到一处，确保：医疗 AI 不越界为诊断 / 处方 / 医嘱 / 替代决策、高风险内容进入医生确认链路、公网模型调用前先脱敏并受门禁约束，并提供一套内置 Demo 数据集与验收测试集，让全部 P0 需求可在个人机或内网环境被真实演示与验收。没有本 phase，前面的功能既无法证明合规，也无法证明达标。

## What Changes

- 新增**基础安全收口**：在前序 phase 已具备的租户隔离 / RBAC / 文档级与知识库级 ACL 之上做横切验收收口（底层判定唯一真值仍在 c01 `auth-rbac` / `document-center`、c04 `rag-retrieval`、c06 `kb-search-qa` / `kb-import`，本 phase 不平行重定义），并新增前序未覆盖的增量：统一约束分享链接与下载权限、数据加密存储（对 c01 `object-storage` 已建存储层的横切收口，POC 最小可演示强度），以及访问日志与操作审计，使上传 / 下载 / AI 调用 / 写回 / 删除均进入审计日志。
- 新增**医疗安全边界**：明确系统定位为科研 / 检索 / 知识查询 / 翻译 / 文书草稿 / 办公辅助，禁止自动诊断 / 处方 / 医嘱 / 替代医生决策；涉及诊疗、用药、医嘱、临床文书、患者个体信息的高风险内容必须进入医生（或授权审核角色）确认链路——既覆盖文档写回（`document_id` 为键），也覆盖三类 message 级文书——AIMed 答案（c04）/ 知识库问答 `kb_qa` 答案（c06）/ 医学翻译文书（c07）——在下发前的 message 级输出（`message_id` 为键），普通用户只能查看提示 / 生成草稿 / 提交审核，并留存包含 `confirmed_by` / `confirmed_role` / `confirmed_scope` / `risk_type` / `before_content_hash` / `after_content_hash` / `audit_log_id` 等字段的 confirmation 确认记录（判定与记录 owner=c05，c09 引用消费收口，验收枚举覆盖三类生产方）。
- 新增**医疗免责声明**：所有医学回答与生成文档统一展示 PRD §19.3 规定的免责声明，AI 生成内容标记为草稿 / 辅助建议。
- 新增**隐私 PHI / PII 识别与脱敏**：覆盖姓名 / 身份证号 / 手机号 / 住院号 / 门诊号 / 医保号 / 地址 / 检查号 / 影像号与可配置敏感词；支持识别并提示、脱敏后送模型、阻止上传 / 阻止调用模型、管理员白名单放行，并提供公网 / 私有化两套识别服务配置入口。识别按 §19.4 / §0.3 落在**两个执行落点**：**上传闸**（文档中心 c01 / AIMed 对话文件上传 c04 / 知识库本地·批量上传 c06 / 翻译文件上传 c07 四类入口，持久化入库或送模型前执行，落实「阻止上传」策略）与**出网闸**（c03 公网出网口，落实「阻止调用模型」策略），共用同一 owner 与规则源，共同闭合 §19.4 两类拦截点。
- 新增**公网模型调用前置门禁与策略**：PHI / PII 识别 + 脱敏引擎 `redaction-gateway` 及 `privacy_detection_rules` / `privacy_redaction_events` 的唯一 owner 为本 change（c09），是被 c03–c07 各 phase 前置消费的横切能力，以 c03 公网出网口为统一执行落点与脱敏留痕权威落点（§19.4 四要素由本 phase 统一写入并外键回 `audit_logs`）。调用公网模型前必须先做敏感信息识别与策略判断；**识别失败、脱敏置信度不足（POC 默认阈值 0.9）、识别服务不可用时禁止调用公网模型**（可切换私有化模型），白名单放行须记录原因 / 放行人 / 放行时间 / 适用范围，且每次公网调用记录是否脱敏 / 脱敏策略 / 命中敏感类型 / 审计日志 ID。**相位约束**：`redaction-gateway` 需早于公网放开；接入前凡 `deployment_kind=public` 公网 provider 一律不可启用，本期主验收闭环默认公网关闭、私有化 / 离线优先（§16.4 / §24.9）。
- 新增**验收证据体系**：交付内置 Demo 数据集与验收测试集（用例字段、数据清单），定义 Evals 指标与阈值、可观测性日志与 Metrics、性能门槛，并将每个验收用例关联到对应的 P0 需求；禁用公网模型时主验收闭环仍须可经私有化模型或离线缓存完成。
- **BREAKING（流程约束）**：高风险内容写回与公网模型调用从“可直接执行”收紧为“必须先经确认链路 / 脱敏门禁”，凡未满足门禁的调用一律被阻断；这是对前序 phase 既有 AI 写回与公网调用路径的破坏性收紧。

## Capabilities

### New Capabilities

- `security-compliance`：基础安全（租户隔离 / RBAC / 文档级与知识库级 ACL / 分享与下载权限 / 加密存储 / 访问与操作审计）、医疗安全边界（不得自动诊断 / 处方 / 医嘱 / 替代决策；高风险内容进入医生确认链路并留存 confirmation 记录字段）、医疗免责声明、隐私 PHI / PII 识别与脱敏（识别范围、识别并提示 / 脱敏 / 阻止 / 白名单放行、公网与私有化识别服务）、公网模型调用前置门禁与策略（识别失败 / 置信不足 / 服务不可用禁用公网、白名单放行留痕、调用脱敏与命中类型记录）。
- `acceptance-evidence`：V1.0 POC 内置 Demo 数据集与验收测试集（用例字段、数据清单）、Evals 指标与阈值、可观测性日志与 Metrics、性能门槛，且每个验收用例关联各 P0 需求。

### Modified Capabilities

（无。当前 `openspec/specs/` 为空，本 phase 全部为新增能力，无既有 spec 需求被修改。）

## Impact

- **受影响服务**：安全合规网关 / 脱敏门禁服务、确认链路（高风险内容审核）、审计服务、可观测性（日志与 Metrics 采集）、验收与 Evals 跑批工具，以及内置 Demo 数据集与测试集装载。横向收紧前序 phase 的 AI 写回（c05 AI 面板）、AIMed RAG 问答（c04）、医学翻译（c07）、知识库导入与检索（c06）、模型 Provider 公网 / 私有化路由（c03）等所有会触达公网模型或操作文档的路径。
- **受影响数据表（参考 PRD §18）**：本 phase 为 `privacy_detection_rules` / `privacy_redaction_events`（识别规则与脱敏 / 命中留痕）、`eval_cases` / `eval_results`（验收用例与结果）四张表的唯一**建表 owner**（显式建表，见 design D7）；其余表均消费 / 写入由各自 owner 所建表、c09 不重复建表——`audit_logs`（统一审计落点，建表即含 `role` / `result` / `failure_reason` 列）/ `document_permissions` / `document_events`（ACL 与操作事件）写入由 c01 所建表，`model_providers` / `model_routes` / `provider_health_checks`（公网门禁与 fallback 留痕；模型 fallback 四要素由 owner c03 写入 `audit_logs`，c09 仅消费 / 验收）/ `visual_parse_providers`（解析服务公私网验收）写入由 c03 所建表（`feedbacks` 由 c04 自建自写，本 change 无写入 / 消费动作，不在受影响清单内）；高风险写回 confirmation 记录的唯一建表 owner 为 c05（`writeback_confirmations` 表，落 §19.2 全字段），c09 仅消费 / 关联该表做统一验收与审计收口；§20.4 的 200 个真实可用模板资产（`templates` / `template_categories`）由 c08 交付，c09 仅引用纳入 Demo 清单不重复装载。`digital_agents` / `workflow_definitions` / `workflow_runs` 为数字员工路线图（V1.1 / V1.2）预留，本 phase 不建表、不生成 task。
- **对其它 phase 的依赖**：依赖 c01–c08 全部横切与业务能力已就绪；本 phase 是 9 阶段收尾环，为整体 V1.0 POC 提供合规闭环与验收证据，不被其它 phase 反向依赖。
- **医疗安全 / 合规影响**：确立医疗安全边界与免责声明，杜绝系统被当作自动诊断 / 处方 / 医嘱 / 替代决策工具。
- **人工确认影响**：高风险内容（诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息）强制进入医生（`doctor`）或授权审核（`reviewer`）角色确认链路（角色真值 owner=c01 `auth-rbac`），普通用户不能完成最终确认，并产生不可篡改的 confirmation 记录与审计关联；`risk_type` 风险分类器与 `writeback_confirmations` 记录表的唯一 owner 为 c05 `ai-writeback-confirmation`，c09 仅引用式消费 / 关联做统一验收与审计收口，不平行重定义判定逻辑、不自建独立表。
- **脱敏与审计影响**：公网模型调用受 PHI / PII 前置门禁约束，识别失败 / 置信不足 / 服务不可用一律禁用公网并可降级私有化；上传 / 下载 / AI 调用 / 写回 / 删除、白名单放行、模型 fallback 全量进入审计日志，保证可追溯。
- **范围边界（明确不做）**：高级合规审计、文档水印、电子签章（§22.3 V1.2+）与完整 Agent Evals 平台（§22.2 V1.1）均不在本期；本期仅交付内置验收测试集与 Demo 数据集。
