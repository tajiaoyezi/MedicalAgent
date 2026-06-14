## 1. 工程脚手架与运行底座

- [x] 1.1 初始化单 SPA 前端 + 后端服务工程骨架（feature-based 目录），约定门户壳与各模块路由槽位的目录组织，确保可本地启动空壳并通过健康检查
- [x] 1.2 引入 PostgreSQL 连接与可重复执行的 schema 迁移框架，约定 `tenant_id` 全局过滤的服务层入口，迁移脚本可幂等重跑（对应 design Migration Plan 步骤 1 验证）
- [x] 1.3 建立环境配置与「公网 / 私有化」双入口约定（数据库、对象存储后端、会话密钥），确保无公网环境下仅靠内网配置即可启动全部底座组件

## 2. 数据库 schema 与种子数据

- [x] 2.1 建 `tenants`（含租户级品牌配置列：Logo / 主色 / 辅助色 / 登录页背景 / 导航栏样式 / 按钮圆角 / 字体大小 / 默认主题）表，命名对齐 PRD §18
- [x] 2.2 建 `users` / `roles` / `permissions`（及角色权限关联），字段含 `tenant_id`、`role`、科室归属、启用状态、口令哈希列，命名对齐 PRD §18
- [x] 2.3 建 `documents`（含 `tenant_id` / `owner_id` / `name` / `space` 枚举 / `is_deleted` / 当前版本指针）与 `document_versions`（PRD §10.5 字段 `version_id` / `document_id` / `file_hash` / `saved_by` / `saved_at` / `source` + 补充人读序号 `document_version`，版本唯一标识仍为 `version_id`）表，`source` 枚举限定 `user_edit / ai_writeback / translation / import / template`（对应 spec「文档版本」，document_version 为 §10.5 清单外补充字段）
- [x] 2.4 建 `document_permissions`（`tenant_id` / `document_id` / `principal_type[user|role|dept]` / `principal_id` / `permission_level`，6 级：owner / manage / edit / comment / view / none）与 `document_events`（稳定契约 `event_type` / `document_id` / `version_id` / `tenant_id` / `occurred_at` / `payload`）表
- [x] 2.5 建 `recent_tasks`（`task_id` / `tenant_id` / `user_id` / `source` / `title` / `ref_type` / `ref_id` / `updated_at` / `deleted_at`，`source` 枚举对齐 PRD §6.4 规范值）与 `audit_logs`（操作者 / `role` / `tenant_id` / 操作类型 / 对象 / 时间 / `result`（成功·失败枚举）/ `failure_reason`）表；`audit_logs` 建表即含 `role` / `result` / `failure_reason` 列，c09 仅引用消费不 ALTER（对应 spec「操作审计入口」、design D7）
- [x] 2.6 写入种子数据：1 个内置演示租户、内置管理员账号与普通用户账号（口令以哈希存储且归属演示租户）、`admin` / `user` / `dept` / `doctor`（医生）/ `reviewer`（授权审核）五类角色与种子权限点（含 `highrisk:confirm` 权限点，授予 `doctor` / `reviewer`、不授予 `user`，对应 §19.2 高风险确认人资格，供 c05/c09 确认链路键取；含 `template:manage` 权限点，仅授予 `admin`，对应 §17.8 模板上架/下架管理，供 c08 引用；含 `kb:create` 权限点，仅授予 `admin`，对应 §11.5/§17.3 创建知识库，供 c06 判定创建授权引用）、默认蓝白主题，验证用内置账号可登录（对应 spec「内置演示租户与演示账号」「角色模型（管理员/普通用户/科室/医生/授权审核）」「RBAC 权限模型与租户隔离」种子权限点）

## 3. 对象存储抽象（MinIO / S3）

- [x] 3.1 定义 `ObjectStorage` 接口（`put` / `get` / `delete` / `presignedUrl` / `headObject`），对象 key 以 `tenant_id/document_id/version_id` 组织，屏蔽后端差异（对应 spec「对象存储抽象层」）
- [x] 3.2 实现落盘时按内容确定性计算 `file_hash`（SHA-256）并写入 `document_versions`，相同内容得一致哈希；读取时校验哈希不一致即报异常、不返回损坏内容（对应 spec「文件落盘与 file_hash」三个场景）
- [x] 3.3 实现下载只走服务层校验后的短时效 presigned URL 或服务端代理流，禁止暴露可绕过权限的公网直链；成功下载写审计（对应 spec「下载与访问控制」三个场景）
- [x] 3.4 离线 / 私有化降级演示：以本地 / 内网 MinIO 为默认后端，验证无公网下文件落盘与读取正常（对应 spec「私有化后端可用」「默认 MinIO 真实落盘全链路」、design D3 离线降级路径）
- [x] 3.5 真实接入连通性测试：对默认 MinIO 后端真实跑通 put / headObject / get / presignedUrl / delete 全链路冒烟（非 mock），并验证 S3 兼容后端可通过同一接口切换后同组操作仍通过（对应 spec「默认 MinIO 真实落盘全链路」「同接口切换 S3 兼容后端」、design D3 备选实现）

## 4. 身份认证、会话与 RBAC

- [x] 4.1 实现用户名 / 口令登录与会话签发（服务端会话或短期 JWT + 黑名单），登录成功创建会话并默认进入 `/aimed`，失败返回明确错误且不创建会话（对应 spec「用户名口令登录与会话」前两个场景）
- [x] 4.2 实现禁用账号即时失效：被禁用用户无法登录、已签发会话即时吊销；无有效会话访问受控资源被拦截要求登录（对应 spec「被禁用账号拒绝登录」「未登录访问受控资源被拦截」）
- [x] 4.3 实现登录尝试（成功 / 失败）写入 `audit_logs`（对应 spec「用户名口令登录与会话」审计要求）
- [x] 4.4 实现角色模型与 `roles` 关联：用户至少一个角色、普通用户可分配科室归属，角色判定限定在所属租户内（对应 spec「角色模型」）
- [x] 4.5 实现 RBAC 授权落点 `requirePermission(...)`：所有授权判定先校验 `tenant_id` 一致再校验角色与权限，越权操作与跨租户访问被拒绝且不产生数据变更 / 不泄露目标租户数据（对应 spec「RBAC 权限模型与租户隔离」）
- [x] 4.6 暴露租户 / 角色判定结果供下游（文档中心 / 知识库 / RAG）按 `tenant_id` / `user_id` / `role` 过滤复用（对应 spec「授权判定可被下游消费」）

## 5. 医疗空间门户外壳、导航与主题品牌

- [x] 5.1 实现门户「左侧固定导航 + 右侧主工作区」外壳，导航含 AIMed / 医疗知识库 / 医疗数字员工 / 医学翻译 / 医疗模板库 / 文档中心 / 最近任务 / 管理后台（仅管理员可见）；切换导航切换主工作区、导航位置固定（对应 spec「医疗空间门户布局与左侧固定导航」）
- [x] 5.2 为每个模块预留路由槽位与占位页（`/aimed` `/knowledge` `/digital-staff` `/translation` `/templates` `/documents` `/recent` `/admin`），约定统一宿主页面契约（主题变量注入点 / 面包屑 / 模块工具条插槽），ONLYOFFICE 走独立宿主页不内嵌主工作区（对应 design D1）
- [x] 5.3 实现默认进入 AIMed：登录后未选模块时主工作区默认展示 AIMed（对应 spec「默认进入 AIMed」）
- [x] 5.4 实现数字员工「规划中」入口：导航保留入口、渲染规划中说明页，不提供任何创建 / 运行 / 编排 / 执行历史（对应 spec「数字员工『规划中』入口」）
- [x] 5.5 实现蓝白（默认）/ 绿白两套主题为 CSS 变量 design token，门户壳启动拉取租户品牌并注入 `:root`，运行时切换无需刷新（对应 spec「蓝白/绿白内置主题」、design D5）
- [x] 5.6 实现租户级品牌配置（Logo / 主色 / 辅助色 / 登录页背景 / 导航栏样式 / 按钮圆角 / 字体大小 / 默认主题）仅管理员可改、保存后对该租户生效、变更写审计；普通用户改品牌被拒（对应 spec「租户级品牌配置」）
- [x] 5.7 落实主题生效范围契约：门户页 / AIMed / 知识库 / 数字员工 / 医学翻译 / 模板库 / 文档中心 / 医疗 AI 面板 / 管理后台统一跟随；明确写入「ONLYOFFICE 编辑器原生 UI 不承诺跟随主题」边界（对应 spec「主题与品牌生效范围」、design D5 例外）
- [x] 5.8 验证门户首页加载 ≤ 2 秒（PRD §21），外壳轻量化、不引入重组件（对应 spec 加载性能场景）

## 6. 文档中心：文件空间、文件操作、权限分级、版本与事件

- [x] 6.1 实现文件空间视图：我的文档 / 团队文档 / 应用文档（含 AIMed / 医学翻译 / 模板生成 / 数字员工输出 / 知识库文档子来源）/ 回收站，按 `documents.space` 枚举 + 软删除视图组织（对应 spec「文档中心文件空间」按空间分类场景）
- [x] 6.2 实现文档级 ACL 与单点判定 `resolveEffectivePermission(user, document)`（owner / 直授 / 角色授 / 科室授取最高级，权限级别含包含关系），所有文档操作前统一调用（对应 design D4、spec「文档权限分级」）
- [x] 6.3 实现文件列表按当前用户在所属租户内的有效权限过滤，无权限文件不在列表可见（对应 spec「按权限过滤文件列表」）
- [x] 6.4 实现文件操作集：上传 / 新建 / 打开（本期仅路由占位）/ 重命名 / 复制 / 移动 / 删除 / 下载 / 分享 / 收藏 / 版本历史 / 权限管理 / 加入知识库（本期仅入口路由占位 + 权限分级校验，落库归 c06）/ 发起 AIMed（本期仅入口路由占位 + 权限分级校验，触发归 c04）/ 发起翻译（本期仅入口路由占位 + 权限分级校验，触发归 c07）/ 用模板新建（本期仅入口路由占位 + 权限分级校验，触发归 c08），每个操作先校验权限分级、权限不足即拒绝；上述占位类操作本期不真正发起下游能力（对应 spec「文件操作」）
- [x] 6.4a 文档中心上传入口在内容持久化入 `documents` / `document_versions` 与对象存储前前置消费 c09 上传闸：经 c09 `redaction-gateway` 做 PHI / PII 识别并按「识别并提示 / 脱敏后送模型 / 阻止上传」策略处理；上传闸接缝设计为「可插拔、缺省放行」——当 c09 尚未接入 / 不可用时按 PRD §19.4 POC 默认策略「识别并提示 + 脱敏后送模型」放行入库并写 `result=成功` 审计（使 9.1/9.5 在 phase 1 即可独立验收），仅当 c09 已接入且策略=阻止上传且命中时拒绝入库并写 `result=失败`+`failure_reason` 非空审计；识别脱敏与 `privacy_redaction_events` 留痕归 c09，c01 仅前置消费（对应 spec「文件操作」上传闸 Scenario 与「c09 上传闸未接入时按默认策略放行」Scenario、c09 Contract 2 上传时 PHI/PII 门禁、PRD §19.4/§0.3/§0.4）
- [x] 6.4b 实现文档中心服务端创建服务（创建 API）：作为「无既存目标文档 / 无 ONLYOFFICE 编辑器会话」的服务端净生成入口，按目标文档空间「新建 / 创建」能力校验调用者（`templateId` 在 AIMed 净生成等场景可缺省），将净生成内容落入 `documents` / `document_versions` 首版（`source` 取对应来源），落库成功后由 c01 文档中心产生一条 `event_type=upload_success` 的 `document_events`（c01 为 `upload_success` 唯一产生方）；AIMed「生成在线 Word / 在线文档」经本服务落库并产生 `upload_success`、再经 c02 打开 ONLYOFFICE，本服务 MUST NOT 依赖 c02 编辑器内 `createNewDocument` 变体（对应 spec「文档中心服务端创建服务」两个场景、跨 change 契约 D1）
- [x] 6.5 实现删除入回收站（软删除而非物理清除、可恢复、不破坏版本链），并写删除审计（对应 spec「删除文件进入回收站」、design D3 软删除）
- [x] 6.6 实现下载 / 分享 / 权限变更受权限控制：下载能力位按 §10.4 统一口径判定（`owner` / `manage` / `edit` / `view` 四级含下载、仅 `comment` 不含下载，为唯一非单调缺口），可管理 / 可编辑用户下载成功并审计、可评论或更低被拒并记被拒审计、无「可管理」及以上对文件分享被拒（对应 spec「文档权限分级」「文件操作」下载 / 分享场景）
- [x] 6.7 实现文档权限分级能力矩阵：可编辑者可触发保存 / AI 写回生成新版本、可查看者不可编辑 / 写回、权限授予与变更仅 owner / manage 可执行并写审计（对应 spec「文档权限分级」三个场景）
- [x] 6.8 实现文档版本：每次保存追加新版本（既有版本不可篡改），记录 `document_version` / `file_hash` / `saved_by` / `saved_at` / `source`；AI 写回版本 `source=ai_writeback` 可被识别；历史版本按时间倒序可查（对应 spec「文档版本」三个场景）
- [x] 6.9 实现 `document_events` 稳定契约：`event_type` 覆盖 6 类触发源（`upload_success` / `save_new_version` / `ai_writeback` / `translation_done` / `template_created` / `manual_reindex`），每条携带 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)`；表 MUST 仅承载这 6 类，文档打开等访问类与解析作业生命周期审计一律写 `audit_logs`、不写 `document_events`；本期在「上传成功」与「服务端创建服务净生成文档首版入库成功」（见 6.4b）两条入库路径产生 `upload_success` 事件（c01 为唯一产生方，不消费），`save_new_version` / `ai_writeback`（c02 保存回调，`ai_writeback` 在回调带 `writebackSource` 时产生）/ `translation_done`（c07）/ `template_created`（c08）/ `manual_reindex`（c06，c03 消费侧触发重解析）仅以契约形态承载、本期不产生，供 c03 / c06 消费（对应 spec「触发重新解析与索引的事件」、design D4 事件契约）

## 7. 管理后台基础与审计

- [x] 7.1 实现管理后台访问控制：仅管理员可进入，普通用户 / 科室角色直接访问 `/admin` 路由被拒；限定在当前演示租户范围、不暴露跨租户管理（对应 spec「管理后台访问控制」）
- [x] 7.2 实现单租户 / 演示租户视图：展示医院名称 / 机构类型 / Logo / 主题 / 用户数 / 存储空间 / 启用模块；不提供新建或切换多租户入口（对应 spec「单租户/演示租户视图」）
- [x] 7.3 实现用户与角色管理：新增用户、禁用用户、角色分配、科室归属、文档权限查看、知识库权限查看，均仅管理员可执行并写审计；禁用用户即时无法登录（对应 spec「用户与角色管理」三个场景）
- [x] 7.4 实现操作审计入口与统一落点 `audit_logs`：记录登录（成功 / 失败）、用户 / 角色变更、文档权限变更、文件上传（成功 / 失败）、文件下载 / 删除、品牌 / 主题配置变更，每条含操作者 / `role` / `tenant_id` / 操作类型 / 对象 / 时间 / `result`（成功·失败）/ 失败时 `failure_reason`；文件上传审计由 c01 上传入口产生（成功记 `result=成功`、被门禁阻断记 `result=失败`+`failure_reason`），c09 仅核对完备性；审计仅管理员可查且按 `tenant_id` 过滤、不暴露其它租户记录（对应 spec「操作审计入口」前两个场景、PRD §24.7、design D7）
- [x] 7.5 裁剪本期后台范围：模型 / 知识库 / 模板 / 翻译 / 视觉解析等后续 phase 专属配置入口本期不提供，仅保留用户 / 角色管理与审计入口（对应 spec「不含后续 phase 专属配置」、design D7）

## 8. 最近任务最小数据模型与列表壳

- [x] 8.1 实现最近任务列表壳：标题前 10 字显示 + 悬浮全标题、按 `updated_at` 倒序、今天 / 7 天 / 30 天 / 1 年 / 全部分组、按模块多选筛选（来源对齐 PRD §6.4 规范值：AIMed 学术助手 / 医疗知识库问答 / 医疗数字员工 / 医学翻译 / 在线文档 AI 操作 / 模板生成文档）（对应 spec「最近任务列表壳展示」全部场景、design D6、PRD §6.5）
- [x] 8.2 实现最近任务查看 / 重命名 / 删除 / 批量删除 + 二次确认；删除即对 `recent_tasks` 记录置 `deleted_at` 软删（PRD §6.7「同步更新历史记录」本期收敛为对 recent_tasks 自身软删，跨来源历史同步依赖 c05/源表、`ref_id` 为空时为空操作）；删除不删关联文件除非勾选「同时删除关联文档」，删除关联文档前校验删除权限并写审计（对应 spec「最近任务列表壳删除」全部场景、PRD §6.7、design D6）
- [x] 8.3 建 `recent_tasks` 最小表（本 change 唯一建表 owner）并仅落最小可承载字段（`task_id` / `tenant_id` / `user_id` / `source` / `title` / `ref_type` / `ref_id` 弱引用 / `updated_at` / `deleted_at`），按 `tenant_id` / `user_id` 隔离；本 change 作为 `ref_type` 弱引用契约 owner 登记 4 类取值唯一对应回源表（`conversation`→`conversations` / `document`→`documents`（仅 documents 行主键）/ `translation_job`→`translation_jobs` / `writeback_confirmation`→`writeback_confirmations`，c05 doc_ai 来源用 `writeback_confirmation` 而非 `document`），消费方 MUST 仅凭 `ref_type` 回源、MUST NOT 按 `ref_type=document` 直推非 documents 语义；各来源恢复内容映射（§6.6）明确不在本期实现、留 c05 在本表 ALTER 扩展（对应 spec「最近任务最小数据模型」全部场景与「ref_type 唯一对应回源表」Scenario、design D6 范围纪律）

## 9. 验收闭环联调与底座交付校验

- [x] 9.1 串联底座主线冒烟：内置账号登录 → 默认进入 AIMed → 切换蓝白 / 绿白主题生效 → 上传文件落盘并算出 `file_hash` → 文档中心按权限可见 → 设置文档权限 → 生成版本 → 经 presigned URL / 代理下载 → 删除入回收站（对应 design Migration Plan 步骤 3–5）
- [x] 9.2 验证审计闭环：上述登录、权限变更、下载、删除、品牌 / 主题变更均在 `audit_logs` 留痕，管理员可在审计入口按租户查到（对应 spec「关键操作产生审计记录」）
- [x] 9.3 验证 PRD §24.6 底座验收子集：管理后台「用户与角色管理」与「系统审计日志」可用，其余后台配置项确认本期不提供（对应 PRD §24.6、spec admin-console-base）
- [x] 9.4 验证性能基线：门户首页加载 ≤ 2 秒、最近任务列表展示 ≤ 2 秒（PRD §21）
- [x] 9.5 验证离线 / 私有化整体可用：在无公网环境用内网 MinIO + 内置账号跑通 9.1 主线，确认底座不依赖任何外部 SaaS（对应 design D3 离线降级、context 离线优先）
