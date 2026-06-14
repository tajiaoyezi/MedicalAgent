## Context

MedOffice AI V1.0 是一个 POC 演示版，目标是在个人机器或内网服务器上跑通完整的医疗智能办公闭环（PRD §0.2），而非医院商用交付。本 change `c01-foundation` 是 PRD §0.4 规定的 9 阶段中的第 1 阶段，无前置依赖，是其余 8 个 phase 的共同底座。

当前状态：`openspec/specs/` 为空，本项目尚无任何源码，是一个全新工程。foundation 需要先立起后续所有能力的承载层——身份与权限、门户外壳与导航、主题与品牌、文档中心与版本、对象存储抽象、管理后台基础——让 onlyoffice-bridge / model-and-parse / aimed-rag-citation / ai-panel-recent-tasks / knowledge-admin / medical-translation / template-center / security-evidence 有可挂载的入口、可写入的文件空间和可审计的权限边界。

关键约束（来自 PRD 与本 phase 范围纪律）：
- **只覆盖 PRD §22.1 P0 必做中属于底座的部分**：医疗空间门户、蓝白/绿白主题、最近任务（仅列表壳）、文档中心、管理后台（用户与角色管理、审计日志）。不纳入 §22.2/§22.3 与数字员工的创建/运行/编排/执行历史。
- **单租户演示模式**（PRD §17.1）：内置演示租户 + 内置管理员账号 + 内置普通用户账号，不强制接入医院 SSO / LDAP / OA。
- **离线优先**（context）：部署环境可能无公网，任何底座组件都必须能在内网/离线下运行；本 phase 涉及的外部依赖只有对象存储，必须给出私有化部署路径。
- **医疗安全红线**（PRD §1.3、§19.1）：租户隔离 + RBAC + 文档级 ACL 是访问控制基线；下载/分享须经权限控制；登录、用户/角色变更、文档权限变更、文件下载/删除等操作须可审计。
- **数据模型复用 PRD §18 核心表命名**：`tenants` / `users` / `roles` / `permissions`、`documents` / `document_versions` / `document_permissions` / `document_events`、`recent_tasks`、`audit_logs`。

本 phase 不直接调用任何 AI 模型，因此 PHI/PII 识别与脱敏不在本期实现（属 security-evidence），但需为后续留出可标识来源的字段（如 `document_versions.source = ai_writeback`）。

## Goals / Non-Goals

**Goals:**
- 立起单租户演示下的身份与会话、角色与 RBAC 权限模型，提供内置管理员/普通用户演示账号，作为全部 phase 的访问控制基线。
- 立起医疗空间门户外壳：左侧固定导航（含数字员工"规划中"入口）、默认进入 AIMed、各模块前端壳与路由组织，让后续 phase 把真实能力挂进既有路由槽位。
- 落地蓝白（默认）/绿白两套主题与租户级品牌配置，并明确其生效范围（含 ONLYOFFICE 原生 UI 不承诺跟随主题）。
- 落地文档中心：文件空间、文件操作、6 级文档权限分级、文档版本（含 `file_hash` / `source` 语义）、以及触发重新解析/索引的事件契约（后续 phase 消费）。
- 落地对象存储抽象：MinIO/S3 文件落盘、`file_hash` 计算、按权限的下载/访问控制，给文档版本与各类生成产物提供统一存储层。
- 落地管理后台基础：单租户/演示租户视图、用户与角色管理、操作审计入口与 `audit_logs`。
- 落地最近任务的**最小数据模型与列表壳**：`recent_tasks` 表 + 列表/分组/筛选展示，能记录来源与标题；完整的会话/文档恢复留给 c05。

**Non-Goals:**
- 不实现真实多租户开通/计费/SSO/LDAP/OA 接入（POC 单租户演示，PRD §17.1）。
- 不实现 ONLYOFFICE 集成、保存回调与文档实际打开（属 c02 onlyoffice-bridge）；本期文档中心的"打开"仅作路由占位。
- 不实现文档解析、视觉解析、向量/全文索引的**消费方**；本期只定义并产生 `document_events` 事件（解析/索引由 c03/c06 消费）。
- 不实现模型/知识库/模板/翻译/视觉解析等后续 phase 专属的后台配置页（PRD §24.6 中除"用户与角色管理""系统审计日志"外的项均不在本期）。
- 不实现 PHI/PII 识别与脱敏、免责声明（属 c09 security-evidence）。
- 不实现最近任务的完整恢复（各来源恢复字段映射属 c05 ai-panel-recent-tasks）。
- 不实现数字员工的创建/运行/编排/执行历史（仅导航"规划中"入口）。

## Decisions

### D1. 门户与各模块前端壳与路由组织：单 SPA + 模块路由槽位

**决策**：医疗空间门户采用单页应用（SPA）+ 固定左侧导航的外壳布局，主工作区由路由驱动。本期为 PRD §3 / §6.2 的每个模块预留**路由槽位与占位页（feature shell）**：`/aimed`、`/knowledge`、`/digital-staff`、`/translation`、`/templates`、`/documents`、`/recent`、`/admin`。默认路由重定向到 `/aimed`（PRD §6.3）。数字员工 `/digital-staff` 渲染"规划中"占位页，不挂任何子路由。后续 phase 只在对应槽位内填充真实组件，不改动外壳与导航结构。

各模块壳约定一个统一的"宿主页面契约"：导航、主题变量注入点、面包屑、模块级工具条插槽。ONLYOFFICE 编辑器作为独立的全屏/抽屉式宿主页面（c02 接入），与门户壳通过路由参数（document_id）解耦，不内嵌进门户主工作区的常规布局。

**备选方案与取舍**：
- *Micro-frontend（每模块独立部署）*：隔离性强，但 POC 阶段 9 个 phase 由同一团队顺序推进，模块间共享主题/鉴权/导航状态，微前端的运行时集成与构建成本远超收益。取舍：选单 SPA，保留按目录分模块（feature-based 目录结构）以便将来需要时再拆。
- *服务端多页应用（MPA）*：每模块独立页面刷新，但门户首页加载 ≤2 秒、最近任务恢复 ≤2 秒（PRD §21）和模块间无刷新切换的体验更适合 SPA。取舍：选 SPA。
- *把 ONLYOFFICE 内嵌进门户主工作区*：会让主题/布局与编辑器原生 UI 强耦合，而 PRD §7.3 明确 ONLYOFFICE 原生 UI 不承诺跟随主题。取舍：编辑器走独立宿主页，门户壳只做入口。

### D2. 单租户演示鉴权与 RBAC 模型：会话令牌 + 角色绑定权限 + 全局 tenant_id 过滤

**决策**：
- **鉴权**：账号密码登录，签发会话令牌（HTTP-only cookie 承载的服务端会话或 JWT，二选一；POC 推荐服务端会话以便即时失效）。内置演示租户 1 个、内置管理员账号 + 普通用户账号（PRD §17.1）。不接入 SSO/LDAP/OA。
- **RBAC**：采用 `roles` ↔ `permissions` 的角色绑定权限模型，`roles` 表为全平台 RBAC 角色的唯一真值，c05/c09 引用不平行重定义。本期角色集合 = {`admin`（平台/医院管理员）、`user`（普通用户）、`dept`（科室，承载 PRD §2.2 科室主任/科室归属语义）、`doctor`（医生）、`reviewer`（授权审核）}。其中 `doctor` / `reviewer` 对应 PRD §19.2「高风险内容确认人需具备医生或授权审核角色」，二者通过权限点 `highrisk:confirm` 表达「可完成高风险内容最终确认」，供 c05 写回确认链路与 c09 安全收口键取 `confirmed_role`（取值枚举 `doctor` / `reviewer`）；`user` 不持 `highrisk:confirm`、不能完成最终确认。`dept`（科室主任语义）本期不承载授权审核资格，授权审核资格统一由 `reviewer` 角色 + `highrisk:confirm` 权限点表达，避免与确认链路歧义。权限为细粒度动作串（如 `document:read`、`document:share`、`user:manage`、`audit:view`、`admin:console`、`highrisk:confirm`）。用户通过 `users.role` 或 `user_roles` 关联获得角色；管理后台仅 `admin` 可见（PRD §6.2）。
- **租户隔离**：所有业务表带 `tenant_id`；所有数据访问在服务层强制注入 `tenant_id = currentTenant` 过滤，作为后续 RAG/知识库 `tenant_id`/`kb_id`/`user_id`/`role`/`acl` 过滤（context 红线）的前置基线。POC 单租户下该过滤恒为演示租户，但代码路径必须存在，避免 c06 再回填。
- **授权落点**：在服务层（API 网关/控制器入口）做 `requirePermission(...)` 检查；文档级细粒度授权下沉到 D4 的 `document_permissions` 解析。

**备选方案与取舍**：
- *直接用枚举角色硬编码权限（不建 permissions 表）*：实现快，但 PRD §18 列了 `permissions` 表且后续 phase（知识库 ACL、模板交付规则）需要可扩展的权限点。取舍：建 `roles`/`permissions` 表，POC 内置一组种子权限，结构可扩展但不过度设计。
- *ABAC/策略引擎（如 OPA）*：表达力强，但 POC 阶段权限点少、规则简单，引入策略引擎是过度工程。取舍：选 RBAC + 文档级 ACL 的两层模型。
- *无状态 JWT vs 服务端会话*：JWT 无需会话存储但难即时吊销（禁用用户需立即生效，PRD §17.2"禁用用户"）。取舍：POC 选服务端会话（或短期 JWT + 服务端黑名单），保证禁用用户即时失效。

### D3. 文档中心存储模型与对象存储抽象（MinIO/S3）

**决策**：采用**元数据与二进制分离**的双层存储：
- **元数据层（PostgreSQL）**：`documents`（逻辑文档，含 tenant_id、owner、name、space、状态、当前版本指针）、`document_versions`（每次保存一条版本）。文件空间（PRD §10.2：我的文档/团队文档/应用文档/AIMed/医学翻译/模板生成/数字员工输出/知识库文档/回收站）用 `documents.space` 枚举 + 软删除标志（回收站 = `is_deleted=true` 视图，而非物理删除），保证删除可恢复且不破坏版本链。
- **二进制层（对象存储抽象）**：定义 `ObjectStorage` 接口（`put/get/delete/presignedUrl/headObject`），实现适配 MinIO 与 AWS S3（二者 API 兼容，同一 S3 SDK 即可）。对象 key 以 `tenant_id/document_id/version_id` 组织，确保隔离与可定位。
- **`file_hash`**：上传/保存时对二进制内容计算内容哈希（如 SHA-256），写入 `document_versions.file_hash`。用途：版本去重判断、完整性校验、以及 c02 保存回调链路的幂等基线。
- **下载/访问控制**：下载不直接暴露对象存储公网地址；由服务层校验 `document_permissions` 后签发**短时效 presigned URL** 或服务端代理流式下载（PRD §19.1 下载权限控制）。

**备选方案与取舍**：
- *文件直接存数据库 BLOB / 本地文件系统*：BLOB 不适合大文件与版本爆炸；本地 FS 缺乏多副本/签名下载/水平扩展，且与 PRD §3"对象存储"底座不符。取舍：选对象存储抽象。
- *直接绑死 MinIO SDK*：会让将来换 S3/OSS 困难。取舍：抽 `ObjectStorage` 接口，MinIO 为 POC 默认实现。
- *版本存"差量"而非整文件*：Office 二进制差量复杂且收益有限。取舍：每版本存整文件 + `file_hash` 去重，简单可靠。

**离线/私有化降级路径**：对象存储默认实现就是 **MinIO（可完全离线/内网私有化部署）**，无需公网。S3 适配仅作为有公网/云环境时的可选实现，二者通过同一接口切换。因此底座在无公网环境下天然可用，不存在外部 SaaS 依赖。

### D4. 文档权限（document_acl）与版本表设计

**决策**：
- **文档级 ACL**：用 `document_permissions` 表承载 PRD §10.4 的 6 级权限——`owner` / `manage`（可管理）/ `edit`（可编辑）/ `comment`（可评论）/ `view`（可查看）/ `none`（无权限）。表结构 `(tenant_id, document_id, principal_type[user|role|dept], principal_id, permission_level)`，支持按用户、按角色、按科室授权。`owner` 由 `documents.owner_id` 表达，ACL 表承载其余授权。
- **权限判定**：取某用户对某文档的**有效权限** = 自身 owner / 直授 / 角色授 / 科室授 中的最高级别。级别序为 manage > edit > comment > view，服务层用单一 `resolveEffectivePermission(user, document)` 函数解析出有效级别，所有文档操作（下载/分享/删除/AI 写回）前调用。**注意 §10.4 能力矩阵存在唯一非单调项**：下载是一条统一的「下载能力位」——`owner` / `manage`（可管理）/ `edit`（可编辑）/ `view`（可查看）四级均含下载（owner=全部操作隐含；manage / edit 为高于 view 的功能级别按高功能级别隐含包含下载；view 级别显式声明下载），唯一例外是 `comment`（可评论）级别在级别序上虽高于 `view`、但 §10.4 仅列「评论/查看/复制文本」未含下载——`comment` 是整张矩阵下载能力位上的唯一非单调缺口。因此 `resolveEffectivePermission` 返回有效级别后，**「下载」判定 MUST 按 §10.4 能力位（capability bit / 能力查表：owner / manage / edit / view = 含下载，仅 comment = 不含）而非「级别 ≥ 可查看」做简单级别序推断**，既要把 `comment` 排除出可下载集合、也要保证 `manage` / `edit` 落在可下载集合内；spec「文档权限分级」据此显式声明「owner / manage / edit / view 均具备下载，仅 `comment` 不具备」。这与 D2 的 RBAC 是两层：RBAC 管"能否进入文档中心/能否进管理后台"等功能权限，document_acl 管"对某具体文档能做什么"。
- **版本表**：`document_versions` 字段对齐 PRD §10.5——`version_id` / `document_id` / `file_hash` / `saved_by` / `saved_at` / `source`，并额外补一个 `document_version`（对 §10.5 的补充字段：人读版本序号/版本号，非 §10.5 原清单项，用于展示与排序，版本唯一标识仍为 `version_id`）。`source` 枚举 = `user_edit / ai_writeback / translation / import / template`，本期上传走 `import`、文档中心新建走对应来源；`ai_writeback` / `translation` / `template` 的实际产生方在后续 phase，但字段语义本期定义，以支撑 c05 的"原文/修改后/说明/影响范围"可确认可回滚链路。
- **事件契约**：`document_events` 表记录 PRD §10.6 的 6 类触发源（上传成功 / 保存新版本 / AI 写回 / 翻译完成 / 模板创建 / 手动重建索引），且**仅承载这 6 类 `event_type`**——文档打开等访问类操作与解析作业生命周期审计一律写 `audit_logs`、不写 `document_events`。每类 `event_type` 有唯一产生方：`upload_success`→c01（本期）；`save_new_version` 与 `ai_writeback`→c02 保存回调（带 `writebackSource` 时落 `ai_writeback`）；`translation_done`→c07；`template_created`→c08；`manual_reindex`→c06（c03 消费侧触发重解析）。本期**只定义表 + 在上传成功时产生 `upload_success` 事件**；其余事件由各自唯一产生方在其相位产生，事件消费（解析/视觉解析/向量/全文索引）属 c03/c06。事件契约固定为 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)`，作为跨 phase 的稳定接口。

**备选方案与取舍**：
- *只按用户授权（不支持角色/科室授权）*：实现简单，但 PRD §2.2 有科室归属、科室主任审核语义，团队文档需按角色/科室共享。取舍：principal_type 支持 user/role/dept，POC 可只用到 user/owner，结构预留 role/dept。
- *把权限级别拆成独立布尔列（can_edit/can_share...）*：易出现非法组合。取舍：用有序的 `permission_level` 枚举 + 包含关系，单点判定，避免不一致。
- *版本物理覆盖（不留历史）*：与 PRD §10.5"每次保存生成版本"和 c05 回滚冲突。取舍：每次保存追加版本，`documents` 持当前版本指针。

### D5. 主题/品牌的租户级配置落地

**决策**：
- **两套内置主题**（PRD §7.1）：蓝白（默认）、绿白，以 **CSS 变量（design token）** 形式定义（主色、辅助色、导航样式、按钮圆角、字体大小等，对齐 PRD §7.2 可配置项）。
- **租户级品牌配置**：存 `tenant_id` 维度的品牌配置（Logo、主色、辅助色、登录页背景、导航栏样式、按钮圆角、字体大小、默认主题）。POC 可挂在 `tenants` 表的 JSON 配置列或单独 `tenant_branding` 配置（优先复用 `tenants`，避免新表过度设计）。前端在门户壳启动时拉取并把 token 注入 `:root` CSS 变量，运行时切换无需刷新。
- **生效范围**（PRD §7.3）：门户页、AIMed、知识库、数字员工、医学翻译、模板库、文档中心、医疗 AI 面板、管理后台——即所有**自建宿主页面**。
- **ONLYOFFICE 例外**（PRD §7.3 末句 / proposal Impact）：**明确不承诺 ONLYOFFICE 编辑器原生 UI 跟随主题**，只对外部宿主页面、插件入口、医疗 AI 面板、顶部自定义入口做主题适配。本期文档把该边界写入主题契约，避免 c02/c05 误把主题强加到编辑器内部。

**备选方案与取舍**：
- *为每套主题写独立 CSS 文件/SCSS 编译*：换主题要重新构建，无法运行时切换租户品牌。取舍：选 CSS 变量 + 运行时注入。
- *主题配置存前端常量*：无法做租户级品牌定制。取舍：配置存后端按 tenant_id 下发。
- *尝试让 ONLYOFFICE 原生 UI 完全跟随主题*：ONLYOFFICE 的主题定制能力有限且非本期目标，强行适配成本高、收益低且与 PRD 明确边界冲突。取舍：仅适配外部宿主与面板入口，原生 UI 不承诺。

### D6. 最近任务的本期最小数据模型

**决策**：本期只落 `recent_tasks` 表 + 门户最近任务列表壳。
- **建表归属**：`recent_tasks` 的**唯一建表 owner 为本 change（c01）**。c05 ai-panel-recent-tasks 不重复建表，只能在本表上 ALTER 补列（如 `title_preview` / `status` / `created_at` / `related_document_id` + `(ref_type, ref_id)` 唯一约束 + `updated_at` 排序索引），其迁移须在 c01 之后执行。
- **表字段**：`(task_id, tenant_id, user_id, source, title, ref_type, ref_id, updated_at, deleted_at)`。`source` 枚举规范值由 c01 在 recent-tasks-shell spec 定义，取 PRD §6.4 六类来源名：`AIMed 学术助手` / `医疗知识库问答` / `医疗数字员工` / `医学翻译` / `在线文档 AI 操作` / `模板生成文档`。各写入方（c04 AIMed、c06 医疗知识库问答、c07 医学翻译、c08 模板生成文档、c05 在线文档 AI 操作，数字员工占位不写）MUST 一律使用上述规范值。`ref_type/ref_id` 是指向回源对象的弱引用，`ref_type` 取值规范集唯一对应回源表（`conversation`→`conversations` / `document`→`documents`（仅 documents 行主键）/ `translation_job`→`translation_jobs` / `writeback_confirmation`→`writeback_confirmations`，c05 doc_ai 写回确认来源用 `writeback_confirmation` 而非 `document`），消费方仅凭 `ref_type` 回源；本期可为空或仅文档来源填充。
- **本期实现（有 spec：recent-tasks-shell）**：列表壳展示规则（PRD §6.5：标题前 10 字、悬浮全标题、按 `updated_at` 倒序、今天/7天/30天/1年/全部分组、按模块多选筛选、查看/重命名/删除/批量删除/二次确认）。删除规则（PRD §6.7：二次确认、不删关联文件除非勾选"同时删除关联文档"）。c01 与 c05 的最近任务 Requirement 使用互不相同的名称（c01 用「最近任务列表壳展示/列表壳删除/最小数据模型」，c05 用「展示规则/恢复内容/删除与二次确认」），避免归档冲突。
- **本期不实现**：各来源的**恢复内容映射**（PRD §6.6 的 Agent 状态/选区/引用段落/翻译进度等，即「六类来源恢复」§6.6），这些字段的写入与恢复链路属 c05 ai-panel-recent-tasks，本期只保证表结构能承载、列表壳能展示。数字员工来源在最近任务中仅作占位/显示「规划中」、不提供恢复（其执行历史恢复属 §22.2 V1.1）。

**备选方案与取舍**：
- *每个来源各建一张历史表，最近任务做 UNION 视图*：查询复杂、排序分页困难。取舍：用单一 `recent_tasks` 聚合表 + `source` 区分，符合 PRD §18 命名与 §6.4"混合展示"语义。
- *本期就把恢复字段全建齐*：会侵入 c05 的设计且本期无数据写入。取舍：本期只建最小可承载字段，恢复细节留 c05，遵守 phase 范围纪律。

### D7. 管理后台基础范围裁剪

**决策**：本期管理后台只实现 PRD §24.6 中**属于底座**的两项——"用户与角色管理"和"系统审计日志"，外加单租户/演示租户视图（PRD §17.1/§17.2）。模型配置、知识库管理、模板管理、视觉解析配置、翻译/术语库/语料库配置等均**不在本期**，由 c03/c06/c07/c08 在 `/admin` 槽位下增量挂载子页面。`audit_logs` 在本期落地（owner=c01），**建表即含 `role` / `result`（成功·失败枚举）/ `failure_reason` 列**（连同操作者/tenant_id/操作类型/对象/时间），记录登录、用户/角色变更、文档权限变更、文件下载/删除等操作，作为后续各 phase 审计的统一落点；c09 安全收口仅引用消费本表（含上述三列）而不 ALTER 加列。

**备选方案与取舍**：
- *本期就把后台框架按 §24.6 全量建出占位页*：会与各 phase 的专属配置设计抢先决策且易返工。取舍：本期只建后台壳 + 用户角色管理 + 审计日志，其余留空槽位由对应 phase 填充。

## Risks / Trade-offs

- **[单租户演示路径与未来多租户不一致]** POC 恒为演示租户，若服务层不强制注入 `tenant_id` 过滤，c06 知识库 ACL 时需大面积回填。→ 缓解：D2 要求所有业务表带 `tenant_id` 且服务层从第一天就走 `tenant_id` 过滤代码路径（即使值恒定）。
- **[RBAC 与 document_acl 两层权限判定不一致]** 功能权限（RBAC）与文档级 ACL 若分散判断，易出现"能进文档中心但越权操作某文档"或反之。→ 缓解：D4 收敛为单一 `resolveEffectivePermission`，所有文档操作前统一调用；RBAC 只管功能入口。
- **[禁用用户未即时失效]** 若用无状态 JWT，禁用用户（PRD §17.2）在令牌过期前仍可访问。→ 缓解：D2 选服务端会话或短期 JWT + 服务端黑名单，保证即时吊销。
- **[下载越过权限控制]** 若前端拿到对象存储直链，可绕过 ACL 下载。→ 缓解：D3 下载只走"服务层校验后的短时效 presigned URL 或服务端代理流"，绝不暴露长效公网直链。
- **[主题被误施加到 ONLYOFFICE 原生 UI]** 后续 phase 可能误以为主题应覆盖编辑器内部。→ 缓解：D5 在主题契约中明确写入 ONLYOFFICE 原生 UI 例外边界。
- **[document_events 事件契约后期被改]** c03/c06 是事件消费方，若本期契约不稳定会连带返工。→ 缓解：D4 固定 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)` 为稳定接口，本期只产生不消费。
- **[最近任务恢复字段在 c05 才补，本期表结构不够]** → 缓解：D6 预留 `ref_type/ref_id/source` 弱引用，恢复细节由 c05 扩展，不在本期锁死。
- **[范围蔓延]** 底座易被顺手做成"把后台全量建出来"。→ 缓解：D7 明确裁剪，非底座配置一律留空槽位。
- **[性能目标]** 门户首页加载 ≤2 秒、最近任务恢复 ≤2 秒（PRD §21）。→ 缓解：SPA 外壳轻量化、最近任务列表分页 + `updated_at` 索引；本期不引入重组件。

## Migration Plan

本项目无既有系统、无既有 spec（`openspec/specs/` 为空），不涉及数据迁移或破坏性变更，因此是**从零初始化**而非迁移：

1. 初始化数据库 schema：建 `tenants` / `users` / `roles` / `permissions`（及 `user_roles`）、`documents` / `document_versions` / `document_permissions` / `document_events`、`recent_tasks`、`audit_logs`。→ 验证：迁移脚本可重复执行、表与 PRD §18 命名一致。
2. 写入种子数据：演示租户 1 个、内置 `admin` / `user` 账号、角色与种子权限、默认蓝白主题。→ 验证：用内置账号能登录。
3. 部署对象存储：默认 MinIO（离线/内网），配置 bucket 与凭据；S3 适配仅在有云环境时切换。→ 验证：上传文件能落盘并算出 `file_hash`，下载走 presigned URL。
4. 起门户外壳与各模块路由槽位 + 主题注入。→ 验证：登录后默认进入 `/aimed`，导航可切换到各占位页，数字员工显示"规划中"，切换蓝白/绿白主题生效。
5. 起文档中心 + 管理后台基础（用户角色管理、审计日志）。→ 验证：文件空间可见、文档权限可设、版本可生成、审计记录可查。

**回滚策略**：POC 全新工程，回滚 = 重置数据库 schema + 清空对象存储 bucket；无生产数据，无需向后兼容迁移。各步骤独立，可单独回退到上一步。

## Open Questions

- 会话机制最终选**服务端会话**还是**短期 JWT + 黑名单**？两者都能满足禁用用户即时失效，POC 倾向服务端会话（更简单），待 specs/tasks 阶段确认实现栈后定。
- 租户品牌配置落在 `tenants` 表的 JSON 列还是独立 `tenant_branding` 表？倾向复用 `tenants`（避免新表），若品牌字段过多再拆。
- `document_versions.file_hash` 的哈希算法（SHA-256 vs 其它）与是否做跨文档去重，本期是否需要去重，还是仅做完整性校验？倾向 SHA-256 + 仅完整性校验，去重留待需要时。
- 文档级 ACL 的 principal_type 本期是否需要真正用到 `role`/`dept`，还是仅 `user`/`owner` 即可满足 POC 演示？结构预留三种，实现范围待 specs 阶段按演示场景裁定。
