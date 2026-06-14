## Why

MedOffice AI V1.0 POC 的主验收闭环（登录 → 医疗空间门户 → 文档中心 → ONLYOFFICE → AIMed/翻译/知识库/模板 → 最近任务恢复）依赖一套统一的身份、权限、门户外壳、文档与文件存储底座。在没有这层底座之前，后续 8 个 phase 既无法登录受控访问，也无法把生成/翻译/解析后的文件落盘、分版本、按权限隔离。

本 change 是 9 阶段中的第 1 个 phase（foundation），无前置依赖，是其余所有 phase（onlyoffice-bridge / model-and-parse / aimed-rag-citation / ai-panel-recent-tasks / knowledge-admin / medical-translation / template-center / security-evidence）的共同基础：先把单租户演示账号、RBAC、医疗空间门户与导航、主题与品牌、文档中心与版本、对象存储、以及管理后台基础跑通，让后续能力有可挂载的入口、可写入的文件空间和可审计的权限边界。

## What Changes

- 新增单租户演示下的身份与权限底座：内置演示租户、内置管理员/普通用户演示账号、登录、角色（管理员/普通用户/科室/医生/授权审核）与 RBAC 权限模型（含 `highrisk:confirm` 权限点，供 c05/c09 高风险确认链路键取），作为所有 phase 的访问控制基线与 RBAC 角色唯一真值。
- 新增医疗空间门户外壳：左侧固定导航（含 AIMed、医疗知识库、医学翻译、医疗模板库、文档中心、最近任务、管理后台、以及数字员工“规划中”入口），默认进入 AIMed；提供蓝白（默认）/绿白两套主题与租户级品牌配置，并约定其在门户及各模块宿主页面的生效范围。
- 新增文档中心：文件空间（我的文档/团队文档/应用文档/各来源文档/回收站）、文件操作、文档权限分级（所有者/可管理/可编辑/可评论/可查看/无权限）、文档版本（对齐 PRD §10.5 `version_id`/`file_hash`/`saved_by`/`saved_at`/`source`，并补充人读 `document_version` 序号字段）、以及触发重新解析与索引的事件契约（后续 phase 消费）。
- 新增对象存储抽象：MinIO/S3 文件落盘、`file_hash` 计算、下载与按权限的访问控制，为文档版本与各类生成产物提供统一存储层。
- 新增管理后台基础：单租户/演示租户视图、用户与角色管理、操作审计入口；不含模型/知识库/模板/翻译/视觉解析等后续 phase 专属配置。
- 数字员工在本期仅在导航显示“规划中”入口，不实现其创建/运行/编排/执行历史。
- 无 **BREAKING** 变更（项目首个 phase，全部为新增能力，`openspec/specs/` 当前为空）。

## Capabilities

### New Capabilities
- `auth-rbac`：单租户演示下的账号、登录、角色（管理员/普通用户/科室/医生/授权审核）与 RBAC 权限模型（含 `highrisk:confirm` 权限点）、内置演示账号（管理员 + 普通用户），作为全平台 RBAC 角色唯一真值。
- `portal-shell`：医疗空间门户布局与左侧固定导航（含数字员工“规划中”入口）、默认进入 AIMed、蓝白/绿白主题与租户级品牌配置及其生效范围。
- `document-center`：文档中心文件空间、文件操作、文档权限分级、文档版本（PRD §10.5 字段 + 补充 `document_version` 人读序号）、触发重新解析与索引的事件。
- `object-storage`：对象存储（MinIO/S3）抽象、文件落盘与 `file_hash`、下载与访问控制。
- `admin-console-base`：管理后台基础——单租户/演示租户、用户与角色管理、操作审计入口（不含模型/知识库/模板/翻译等后续 phase 专属配置）。
- `recent-tasks-shell`：最近任务的唯一聚合表 `recent_tasks`（本 change 为该表唯一建表 owner）+ 门户最近任务列表壳（§6.5 展示规则、§6.4 六来源多选筛选、§6.7 删除规则与二次确认）；§6.6 各来源恢复内容映射不在本期，留 c05。

### Modified Capabilities
（无。`openspec/specs/` 当前为空，本 change 全部为新增能力，无既有 spec 的需求被修改。）

## Impact

- 受影响服务：身份认证与会话、RBAC 授权、医疗空间门户前端外壳与主题/品牌服务、文档中心服务、对象存储服务（MinIO/S3 网关）、管理后台基础服务。ONLYOFFICE 编辑器内部原生 UI 不承诺完全跟随主题，仅适配外部宿主页面与面板入口。
- 受影响数据表（参考 PRD 第 18 章命名）：`tenants`、`users`、`roles`、`permissions`；`documents`、`document_versions`、`document_permissions`、`document_events`；`recent_tasks`（最小表）；`audit_logs`。以上表均由本 change 唯一建表，`recent_tasks` 的唯一建表 owner 为本 change，c05 仅在本表上 ALTER 补列（如 title_preview / status / created_at / related_document_id）与补充约束，绝不重复建表。其中 `document_events` 仅定义并产生“触发重新解析/索引”事件，事件的消费方（解析、视觉解析、向量/全文索引）属于 model-and-parse、knowledge-admin 等后续 phase。
- 对其它 phase 的依赖关系：本 phase 是全部 8 个后续 phase 的前置依赖。onlyoffice-bridge 依赖文档中心与版本链路、对象存储落盘；model-and-parse / knowledge-admin 依赖 `document_events` 的解析/索引触发事件；ai-panel-recent-tasks / medical-translation / template-center 依赖文档中心文件空间、权限分级与门户导航入口；所有 phase 依赖 `auth-rbac` 的登录与角色、`admin-console-base` 的审计入口。
- 医疗安全 / 合规影响：建立租户隔离与 RBAC + 文档级 ACL 的访问控制基线，为后续 RAG/知识库按 `tenant_id`/`kb_id`/`user_id`/`role`/`document_acl`/`chunk_acl` 六维过滤、以及高风险内容的角色化确认链路提供前置约束；其中 `document_acl` 维由本 change 所建 `document_permissions` 表派生（文档级过滤维度，非 chunk 列），由 c06 注入预计算的可见 document 集合或等价条件执行，`chunk_acl` 为 c03 `document_chunks` 上的 chunk 级 ACL 列；下载与分享须经权限控制。
- 人工确认 / 脱敏影响：本 phase 不直接调用模型，故不在本期实现 PHI/PII 识别与脱敏（属 security-evidence），但文档版本的 `source` 字段需可标识 `ai_writeback` 等来源，以支撑后续 ai-panel-recent-tasks 的“原文/修改后/说明/影响范围”可确认、可回滚链路。
- 审计影响：管理后台“操作审计入口”与 `audit_logs` 表在本期落地，登录、用户/角色变更、文档权限变更、文件下载/删除等操作需可被记录，作为后续各 phase 审计能力的统一落点。
