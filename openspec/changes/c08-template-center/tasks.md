# Tasks: c08-template-center 医疗模板中心

> 范围：仅 PRD §22.1 P0 必做。§14.7 高级投放规则、§17.4 完整模板管理后台属 V1.1，本期不实现其规则逻辑。
> 依赖：c02 `onlyoffice-bridge`（ONLYOFFICE 集成、文档中心、对象存储、`documents` / `document_versions` 契约、`openAIPanel`/`closeAIPanel` 右侧面板控制 Bridge）；c05 `medical-ai-panel`（医疗 AI 右侧面板挂载、P0 功能列表与「默认展示」语义的功能本体）；复用 c01 foundation 的多租户隔离与 RBAC。
> 说明：c02 仅提供打开面板壳的机制，「文档打开后默认展示医疗 AI 面板」的触发 owner 与功能本体均为 c05；c08 作为打开模板生成文档的入口仅引用 c05 拥有的默认展示触发，由 c05 经 c02 `openAIPanel` 自动展示并按文档类型渲染默认 P0 功能，c08 不自定义触发、不实现面板本体。
> 验收源：PRD §14、§5.4、§24.5（「副本可用 ONLYOFFICE 打开」），其中「默认打开医疗 AI 面板」回引 §14.6 / §14.8。

## 1. 数据模型与迁移

- [ ] 1.1 新建 `template_categories` 字典表迁移（`id` / `name` / `sort_order`），复用 PRD §18 命名；保证迁移幂等可重复执行（验收：PRD §14.4 六大分类有承载表）
- [ ] 1.2 新建 `templates` 主表迁移，覆盖 PRD §14.8 资产字段与 §14.5 卡片字段：`id` / `tenant_id` / `category_id`(FK) / `name` / `scene_desc` / `file_type` / `storage_key` / `preview_image_key` / `tags` / `commercial_use` / `copyright_source` / `copyright_status` / `copyright_note` / `created_by` / `version` / `status` / `usage_count` / `created_at` / `updated_at`（验收：八项资产字段齐备）
- [ ] 1.3 新建 `template_usage` 流水表迁移（`id` / `template_id` / `user_id` / `tenant_id` / `action`(use/favorite/unfavorite) / `result_document_id` / `created_at`），承载使用次数计数与收藏状态推导（验收：使用次数随使用更新、收藏按用户区分）
- [ ] 1.4 确认本期不创建 `template_delivery_rules` 规则逻辑（按 design D5 / Q3，仅 V1.1 预留），并在迁移注释中标注该决策，避免后续误用（验收：本期可见性不引用投放规则表）
- [ ] 1.5 种子写入 6 行 `template_categories`（医疗服务 / 教学科研 / 人力资源 / 行政办公 / 财务采购 / 运营管理），固定 `sort_order`（验收：PRD §14.4 六分类可枚举）

## 2. 模板资产导入与版权闸门（200 模板交付）

- [ ] 2.1 定义模板交付清单契约（CSV / JSON 清单 + 文件目录结构），声明每个模板的源文件、预览图、版权字段（`copyright_source` / `copyright_status` / `copyright_note` / `commercial_use`）、分类、标签、适用场景、创建人、版本号（验收：PRD §14.8 资产要求可结构化校验）
- [ ] 2.2 实现幂等导入脚本：将模板源文件与预览图写入对象存储（复用 c02 对象存储客户端）、元数据写入 `templates` / 关联 `template_categories`（验收：可重复执行不产生重复数据）
- [ ] 2.3 在导入脚本中实现资产完整性闸门：模板文件 / 预览图 / 版权确认 / 分类标签 / 适用场景说明 / 是否可商用 / 创建人 / 版本号八项任一缺失即拒绝入库并报告缺失字段（验收：缺字段模板不上架，对应 spec「模板资产字段完整性校验」）
- [ ] 2.4 在导入脚本中实现版权闸门：仅 `copyright_status=confirmed` 才置 `status=on_shelf`；`pending` / `restricted` 不上架；标记为未授权稻壳 / 天策来源的条目拒绝入库并记录拒绝原因；重导入导致某模板 `copyright_status` 相对库中现值发生变更时，由导入流程留痕（导入操作人 / 导入时间 / 版权状态前后值），作为 6.3 版权状态留痕的唯一产生方（本期无运行时改写入口）（验收：对应 spec「缺少版权确认不得入库」「禁止引入未授权稻壳/天策模板」「重导入变更版权状态时由导入流程留痕」）
- [ ] 2.5 离线 / 私有化路径：预览图由内容团队用本地工具（ONLYOFFICE 本地实例 / LibreOffice headless）在交付前离线预生成并随种子导入；预览图缺失时按 `file_type` 占位图回退，运行期不依赖在线渲染（验收：design D6，无公网环境可导入与展示）
- [ ] 2.6 灌入 200 个真实可用、版权归属清楚的医疗模板，按 6 分类分布（配额取 design Q1 产品 / 内容团队结论）；至少包含一个 `tenant_id` 非空的指定租户可见样例（design Q4）以演示可见性（验收：PRD §24.5「展示 200 个真实可用医疗模板」）

## 3. 模板中心后端接口

- [ ] 3.1 实现模板列表 / 分类筛选 / 关键词搜索接口：支持 `category_id` 筛选、关键词 over `name`+`tags`+`scene_desc`、分页；返回 PRD §14.5 全部卡片字段（名称 / 分类 / 适用场景 / 文件类型 / 更新时间 / 使用次数 / 收藏状态 / 预览图 / 适用租户 / 标签）（验收：PRD §24.5 支持分类筛选与关键词搜索）
- [ ] 3.2 在列表 / 搜索 / 筛选中统一施加基础可见性过滤：`status=on_shelf` AND `copyright_status=confirmed` AND (`tenant_id IS NULL` OR `tenant_id = :currentTenant`)，叠加 c01 `auth-rbac` 既有模块访问门（有效会话 + 租户内，不新增具名访问权限点）（验收：对应 spec「仅展示对当前用户可见的模板」「跨租户访问被隔离」）
- [ ] 3.3 收藏状态读路径：批量查询当前 `user_id` 在结果集模板上的最新 favorite/unfavorite 记录，合并进卡片 DTO，避免 N+1（验收：对应 spec「收藏状态可切换并按用户区分」）
- [ ] 3.4 实现模板详情 / 预览接口：返回预览图与适用场景、版权状态、是否可商用、创建人、版本号；预览不创建个人文档副本、不改模板资产；对不可见模板拒绝预览并提示无权访问（验收：对应 spec「模板预览」「版权状态可查询」）
- [ ] 3.5 实现收藏 / 取消收藏接口：写 `template_usage(action=favorite/unfavorite)`，按 `user_id` 持久化，仅对该用户生效（验收：对应 spec「收藏状态可切换并按用户区分」）
- [ ] 3.6 搜索边界处理：可见范围内无命中返回空结果并提示未找到，不抛错、不泄露不可见模板存在（验收：对应 spec「搜索无结果的边界」）

## 4. 使用模板核心流程（衔接 c02）

- [ ] 4.1 实现「使用模板」前置校验：模板 `status=on_shelf` 且 `copyright_status=confirmed` 且对当前用户可见，否则拒绝创建副本并提示不可用（验收：对应 spec「对不可见或不可用模板拒绝使用」「版权状态置为不可用时禁止使用」）
- [ ] 4.2 实现对象存储 server-side copy：把 `templates.storage_key` 复制为新 object 得到新 `storage_key`；底层不支持原子 copy 时退化为服务端下载 / 上传，对调用方透明，且不改动模板原始资产（验收：design D4，对应 spec「复制不改动模板原始资产」）
- [ ] 4.3 据 c01 `document-center` 既有 `documents` / `document_versions` 基础契约（owner=c01）写入 `documents`（owner=当前 user、`tenant_id`=当前租户、`file_type` 沿用模板）并创建首个 `document_versions`，`source=template`（复用 PRD §10.5 既有枚举，不新增）；模板复制首版本由 c08 直接写入 `source=template` 并由 c08 同事务自产 `template_created`，**不走 c02 保存回调创建路径、不产生 `save_new_version`**（c02 仅为 `save_new_version` / `ai_writeback` 两类 `document_events` 的唯一产生方）（验收：对应 spec「使用模板复制为个人文档」「个人文档归属当前用户并受 ACL 约束」「模板复制成功产生 template_created 触发事件」）
- [ ] 4.4 同事务写 `template_usage(action=use, result_document_id)` 并 `UPDATE templates SET usage_count = usage_count + 1`，保证流水与冗余计数一致（验收：对应 spec「使用次数随使用行为更新」）
- [ ] 4.5 返回新 `document_id`，前端用 c02 既有路由在 ONLYOFFICE 打开模板生成文档后 **引用 c05 `medical-ai-panel` 拥有的「文档打开后默认展示医疗 AI 面板」触发**（owner=c05，经 c02 `openAIPanel` 机制自动展示并按文档类型渲染默认 P0 功能面板，无需用户点击），本能力不自定义该触发、不实现面板渲染本体（验收：「副本可用 ONLYOFFICE 打开」对应 PRD §24.5；「打开后默认展示医疗 AI 面板」对应 PRD §14.6 / §14.8 / §5.4 —— 触发 owner 与面板渲染本体均归 c05、c02 仅提供 `openAIPanel` 机制、c08 仅引用 c05 触发）
- [ ] 4.8 验证模板生成文档打开后引用 c05 默认展示触发：以集成测试观测模板生成文档在 ONLYOFFICE 打开后，由 c05 `medical-ai-panel` 拥有的默认展示触发经 c02 `openAIPanel` 自动展示面板，c08 仅作为打开入口传入新 `document_id`、不自定义触发逻辑（验收：对应 spec「模板生成文档打开后引用 c05 默认展示触发」；默认展示触发 owner=c05，c08 不重复声明该触发）
- [ ] 4.7 使用模板成功后在同事务（与 4.3 `documents` / 4.4 `template_usage` 写入同事务）向 c01 所建 `recent_tasks` upsert 一条记录：`source=模板生成文档`（对齐 c01 §6.4 source 规范值）、`ref_type=document`、`ref_id=生成文档 document_id`、`title` 取生成文档名（缺省回退模板名）、按 `tenant_id` / `user_id` 隔离、按 `(ref_type, ref_id)` 幂等（验收：对应 spec「使用模板后写入 source=模板生成文档 的最近任务记录」「最近任务记录按租户与用户隔离且幂等」；c05 经 `ref_id=document_id` 可恢复模板生成任务并打开生成文档，对应 PRD §6.4 / §6.6 / §20.3；c05 仅负责恢复编排不变）
- [ ] 4.9 模板复制成功后在同事务（与 4.3 `documents` / `document_versions`、4.4 `template_usage`、4.7 `recent_tasks` 同事务）产生一条 c01 契约形态的 `document_events`（`event_type=template_created`），携带 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)` 稳定字段，`document_id` / `version_id` 指向新生成文档及其首版本（验收：对应 spec「模板复制成功产生 template_created 触发事件」；c08 为 `template_created` 唯一产生方，对齐 c01 §10.6 六类触发源契约与 c03 消费侧重解析/索引；c01 仅以契约形态承载该取值不产生）
- [ ] 4.6 离线 / 私有化路径验证：整条「使用模板 → 复制 → 打开 → 医疗 AI 面板」在本地 MinIO/S3 + 本地 ONLYOFFICE、无公网环境下完整可用，不触发任何公网模型 / 解析调用（验收：design D4 离线降级，闭环可演示）

## 5. 两个入口与前端编排

- [ ] 5.1 入口 1：医疗空间左侧导航「医疗模板库」进入模板库，调用统一后端接口展示对当前用户可见的模板（验收：对应 spec「左侧导航入口进入模板库」）
- [ ] 5.2 入口 2：文档中心 / 新建页「医疗专区」按 6 分类聚合 + 搜索编排，复用同一后端接口；点击「使用模板」复制为个人文档并在 ONLYOFFICE 打开、自动展示医疗 AI 面板（验收：PRD §5.4 / §24.5「支持医疗专区新建页」）
- [ ] 5.3 前端渲染模板卡片：展示 PRD §14.5 全部十项字段，支持收藏切换与「预览」「使用模板」操作（验收：对应 spec「模板卡片展示完整字段」）
- [ ] 5.4 两入口行为一致性：对同一模板从两条路径执行「使用模板」生成等价个人文档并触发相同的 ONLYOFFICE 打开与医疗 AI 面板默认展示行为（验收：对应 spec「两个入口行为一致」）

## 6. 审计、合规与验收

- [ ] 6.1 「使用模板」成功时写 `audit_logs`：操作用户 / `tenant_id` / 模板标识 / 生成文档标识 / 操作时间 / 结果（验收：对应 spec「使用模板生成审计记录」）
- [ ] 6.2 「使用模板」因不可见 / 不可用失败时写一条标记失败的 `audit_logs` 并附拒绝原因（验收：对应 spec「失败操作也留痕」）
- [ ] 6.3 版权状态留痕（经导入流程）：版权状态仅由 2.2 / 2.4 种子 / 导入脚本设定，本期无运行时改写 `copyright_status` 入口（§17.4「版权信息」编辑 UI 属 V1.1，见 design Non-Goals / D7）；2.2 / 2.4 导入脚本在重导入导致 `copyright_status` 相对现值变更时留痕（导入操作人 / 导入时间 / 版权状态前后值）；`copyright_status` 非 `confirmed`（`pending` / `restricted` / 导入设为不可用）的模板在列表与详情不提供「使用模板」入口（复用 3.2 读侧 `copyright_status=confirmed` 过滤），已存在的个人文档副本不受影响（验收：对应 spec「模板版权状态追踪」「重导入变更版权状态时由导入流程留痕」「版权状态非 confirmed 时禁止使用」）
- [ ] 6.4 临床文书确认链路不被绕过：由模板生成的病历 / 知情同意等临床文书，后续经医疗 AI 面板产生诊疗 / 用药 / 医嘱内容时仍视为草稿、仍走既有医生（或授权审核角色）确认链路与写回前确认（验收：对应 spec「模板生成的临床文书草稿不绕过既有确认链路」）
- [ ] 6.5 实现模板上架/下架运行时管理操作（PRD §17.8 POC 后台必做）：按 c01 `auth-rbac` 定义的 `template:manage` 权限点（owner=c01，本能力仅键取、仅授 admin）鉴权，持有 `template:manage` 的管理员可将 `templates.status` 在 `on_shelf` / `off_shelf` 间切换，不具该权限点的普通用户无权操作；下架后该模板在列表 / 搜索 / 筛选 / 预览 / 使用模板对普通用户不可见不可用（复用 3.2 基础可见性 `status=on_shelf` 过滤），重新上架恢复可见可用（验收：对应 spec「管理员下架模板后对用户不可见」「管理员重新上架恢复可见」「非授权用户不能执行上架/下架」）
- [ ] 6.6 上架/下架切换写 `audit_logs`：每次切换记录操作者 / `tenant_id` / 模板标识 / 前后状态 / 操作时间（验收：对应 spec「上架/下架写审计」）
- [ ] 6.7 主验收闭环核验：对照 PRD §24.5 逐项核验 —— 展示 200 模板、每模板字段齐备（文件 / 预览图 / 分类 / 标签 / 适用场景 / 版权状态）、分类筛选、关键词搜索、医疗专区新建、点击创建副本、副本可 ONLYOFFICE 打开；并加一步「真实可用」抽样核验：从 200 模板按 6 分类各抽样若干份执行「使用模板 → 复制 → ONLYOFFICE 打开」，确认生成文档含可识别结构化骨架（标题层级 / 字段占位 / 非空正文）、可编辑保存，且非仅含表头或占位文字的空壳，使「真实可用」（§22.1 / §24.5）落为可勾稽证据（验收：PRD §24.5 全部验收点通过 + 对应 spec「抽样核验模板真实可用而非占位空壳」；§24.5 清单止于「副本可用 ONLYOFFICE 打开」，不含「默认打开医疗 AI 面板」）；「默认打开医疗 AI 面板」另列回引 PRD §14.6 / §14.8（触发 owner 与功能本体均依赖 c05），不计入 §24.5 逐项清单（验收：默认面板项以 §14.6 / §14.8 为依据通过；其中 c08 仅引用 c05 拥有的默认展示触发由 4.8 验证「模板生成文档打开后引用 c05 默认展示触发」，触发与渲染本体均归 c05、c08 不重复声明）
