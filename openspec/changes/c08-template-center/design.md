## Context

医疗模板中心（`template-center`）是 MedOffice AI V1.0 POC 9 阶段中的第 8 阶段，依赖第 2 阶段 `onlyoffice-bridge`（c02）。它为医疗空间补全「从医疗模板创建文档」这条高频内容生产入口，与既有的「上传文档」「新建空白文档」并列。

**当前状态与约束（来自 PRD §14、§5.4、§17.4、§22.1、§24.5）**

- V1.0 POC 必须**一次性交付 200 个真实可用、版权归属清楚的医疗模板**，不复用未授权的 WPS 稻壳 / 天策资产，且模板版权状态必须可追踪（PRD §14.2、§24.5）。
- 模板按 **6 大分类**组织：医疗服务 / 教学科研 / 人力资源 / 行政办公 / 财务采购 / 运营管理（PRD §14.4）。
- 模板卡片需展示完整字段：名称、分类、适用场景、文件类型、更新时间、使用次数、收藏状态、预览图、适用租户、标签（PRD §14.5）。
- 必须提供搜索、分类筛选与预览，以及「使用模板 → 复制为个人文档 → ONLYOFFICE 打开 → 默认展示医疗 AI 面板」核心流程（PRD §14.6、§24.5）。
- 两个入口：入口 1「左侧导航 → 医疗模板库」，入口 2「文档中心 / 新建页 → 医疗专区 → 选择模板」（PRD §14.3、§5.4）。
- `documents` / `document_versions` 与文档中心、文档级 ACL、多租户隔离与 RBAC、对象存储的基础数据契约由 foundation（c01）拥有；c02 仅拥有 ONLYOFFICE Bridge 与保存回调链路（`openAIPanel` / `closeAIPanel`、保存回调产生 `save_new_version` / `ai_writeback`）。本期复用 c01 的 `documents` / `document_versions` 基础契约与 c02 的 ONLYOFFICE Bridge / 打开机制，不改动二者契约。

**范围纪律**：本 phase 仅覆盖 PRD §22.1 P0 必做与 §17.8「V1.0 POC 后台必做清单」。PRD §14.7 的「按租户类型 / 医院 / 科室 / 角色 / 标签的高级投放规则」与 §17.4 模板管理后台中确属 V1.1 的部分（在线上传 UI / 投放规则编辑 / 版权信息编辑 UI）不在本期；但 §17.8 明列为 POC 必做的「模板分类、标签、预览图、上架 / 下架」属本期范围，其中**上架/下架运行时管理操作（`templates.status` 在 `on_shelf`/`off_shelf` 切换 + 状态机 + 审计）纳入本期**（见 D9），仅其在线上传 / 投放规则编辑等 §17.4 增量延后。

**利益相关方**：医生、医务行政、科研人员（模板消费者）；平台运营 / 内容团队（200 个模板的生产与版权确认）；合规（版权状态追踪）。

## Goals / Non-Goals

**Goals:**

- 交付可演示、可验收的医疗模板中心：浏览 / 搜索 / 分类筛选 / 预览 / 收藏 / 使用模板全链路。
- 定义模板资产与元数据模型（`templates` / `template_categories` / `template_usage`），承载 PRD §14.5 卡片字段与 §14.8 资产要求，并落地版权状态追踪。
- 定义「使用模板」的复制语义：模板文件复制为当前用户个人文档，据 c01 `documents` / `document_versions` 基础契约（owner=c01）落入文档中心，由 c02 ONLYOFFICE Bridge 打开并默认展示医疗 AI 面板（触发 owner=c05）；模板首版本由 c08 自产 `source=template` + `template_created`，不走 c02 保存回调。
- 定义新建页「医疗专区」结构（按 6 分类聚合 + 搜索）与两个入口的统一后端契约。
- 提供基础可见性（哪些模板对当前用户可见）的最小过滤，遵循 `tenant_id` / role / ACL。
- 给出 200 个模板来源与组织方式（自建 / AI 辅助 / 授权）的交付计划，作为可执行的内容生产约束。

**Non-Goals:**

- 不实现 PRD §14.7 高级投放规则引擎（按医院 / 科室 / 角色 / 标签精细投放）——V1.1。
- 不实现 PRD §17.4 完整模板管理后台中确属 V1.1 的部分（运营在线上传 UI / 投放规则编辑 / 版权信息编辑 UI）——V1.1；本期模板与版权数据通过种子脚本 / 数据初始化导入。**例外**：§17.8 POC 后台必做清单明列的「上架/下架」运行时管理操作纳入本期（见 D9），不随 §17.4 整体延后。
- 不改动 c02 的 ONLYOFFICE 集成、保存回调、文档版本契约；本期只调用，不修改。
- 模板为静态资产，不触发公网模型调用，不引入新的 PHI / PII 脱敏路径（脱敏边界沿用 AIMed / 翻译既有前置校验）。
- 不纳入数字员工的创建 / 运行 / 编排 / 执行历史。

## Decisions

### D1. 模板资产与元数据：三表模型，复用 PRD §18 命名

复用 PRD §18 核心表 `templates` / `template_categories` / `template_usage`，职责切分如下：

- `templates`（模板主表）：承载 PRD §14.8 资产要求与 §14.5 卡片字段。核心列（命名沿用 §18 风格）：`id`、`tenant_id`（可见域，见 D5）、`category_id`（外键 `template_categories`）、`name`、`scene_desc`（适用场景说明）、`file_type`（docx / xlsx / pptx 等）、`storage_key`（对象存储中模板源文件引用）、`preview_image_key`（预览图引用）、`tags`（标签数组）、`commercial_use`（是否可商用 bool）、`copyright_source`（版权来源：self_built / ai_assisted / licensed）、`copyright_status`（版权状态：见 D3）、`copyright_note`（授权说明 / 授权方）、`created_by`（创建人）、`version`（版本号）、`status`（on_shelf / off_shelf，本期默认 on_shelf）、`usage_count`（冗余使用次数，见 D2）、`created_at` / `updated_at`。
- `template_categories`：6 大分类的字典表，`id`、`name`、`sort_order`。固定 6 行（医疗服务 / 教学科研 / 人力资源 / 行政办公 / 财务采购 / 运营管理），由种子数据初始化。
- `template_usage`：使用与收藏行为流水，`id`、`template_id`、`user_id`、`tenant_id`、`action`（use / favorite / unfavorite）、`result_document_id`（use 时指向复制生成的个人文档）、`created_at`。收藏状态由该表 `action=favorite/unfavorite` 的最新记录推导，使用次数由 `action=use` 计数。

**备选方案与取舍**：

- *备选 A：把卡片字段塞进 `documents` 表加 `is_template` 标志位*。否决——模板资产字段（版权状态、是否可商用、预览图、适用场景）与文档语义差异大，混入会污染 `documents` 契约并增加 c02 文档中心的耦合；独立 `templates` 表语义清晰且与 §18 命名一致。
- *备选 B：收藏与使用次数各建独立表（`template_favorites` + `template_usage`）*。否决——POC 阶段两类行为同构（都是「用户对模板的一次动作」），合并进 `template_usage` 用 `action` 区分更简单，符合「最小代码」原则；若 V1.1 收藏需要独立索引再拆。
- *使用次数为何冗余到 `templates.usage_count`*：卡片列表是高频读、按使用次数排序的场景，实时 `COUNT(template_usage)` 会成为热点；冗余计数 + 写时自增，列表读零聚合。代价是需在「使用模板」事务内同步自增（见 D4），可接受。

### D2. 使用次数与收藏状态的读写路径

- **写**：每次「使用模板」在同一事务内 `INSERT template_usage(action=use)` 并 `UPDATE templates SET usage_count = usage_count + 1`。收藏 / 取消收藏只写 `template_usage`，不改 `templates`。
- **读**：列表 / 卡片直接读 `templates.usage_count`；当前用户收藏状态用一次按 `(user_id, template_id)` 的批量查询（IN 列表 id）合并进卡片 DTO，避免 N+1。

**备选**：用 Redis 计数器异步回写。否决——POC 单机 / 内网部署，引入额外缓存组件不划算，PostgreSQL 行级自增足够；离线 / 私有化环境也少一个依赖。

### D3. 版权状态追踪（合规红线）

`templates` 增加三列实现版权可追踪（PRD §14.8「版权确认」「是否可商用」「版权状态必须可追踪」、§24.5）：

- `copyright_source`：`self_built`（系统 / 运营自建）/ `ai_assisted`（AI 辅助生成后人工确认）/ `licensed`（获授权第三方资产）。
- `copyright_status`：`confirmed`（已确认可用）/ `pending`（待确认，**禁止上架**）/ `restricted`（受限，仅特定场景）。**只有 `confirmed` 的模板可对用户可见 / 可使用**，作为防止未授权资产泄漏到生产的硬闸门。
- `copyright_note`：自由文本，记录授权方 / 授权范围 / 自建说明，供合规追溯。

**取舍**：用枚举 + 闸门而非仅一个布尔 `is_authorized`。理由——PRD 明确禁止稻壳 / 天策类未授权资产，且要求「版权状态可追踪」，单布尔无法区分来源与受限场景，也无法支撑审计追溯；三列方案以极小成本满足合规并为 V1.1 管理后台预留语义。`commercial_use`（是否可商用）独立保留，因为「已授权」与「可商用」是正交维度（如授权仅限内部非商用）。

### D4. 使用模板的复制与文档中心落地（核心流程，衔接 c02）

「使用模板」复制为个人文档采用**服务端对象存储复制 + 据 c01 `documents` / `document_versions` 基础契约（owner=c01）创建个人文档**，不经过 ONLYOFFICE 转换、不走 c02 保存回调创建路径：

1. 校验：模板 `status=on_shelf` 且 `copyright_status=confirmed` 且对当前用户可见（D5），否则拒绝。
2. 对象存储侧 server-side copy：把 `templates.storage_key` 复制成新 object（新 `storage_key`），避免下载 / 上传往返。
3. 据 c01 `document-center` 既有 `documents` / `document_versions` 基础契约（owner=c01）创建 `documents` 行（owner = 当前用户、`tenant_id` = 当前租户、文件类型沿用模板 `file_type`、默认落入文档中心「医疗模板生成」空间，见 PRD §10.2）与首个 `document_versions`，**`source` 取值 `template`**（PRD §10.5 已定义 `source: ... / template` 枚举，直接复用，无需新增枚举）。模板复制首版本由 c08 直接写入 `source=template` 并由 c08 同事务自产 `template_created`，**不走 c02 保存回调创建路径、不产生 `save_new_version`**（c02 仅为 `save_new_version` / `ai_writeback` 两类 `document_events` 的唯一产生方，模板复制首版本不经保存回调产生，故由 c08 自产 `template_created`）。
4. 同事务写 `template_usage(action=use, result_document_id=新文档id)` 并自增 `usage_count`；在同事务向 c01 所建 `recent_tasks` upsert 一条 `source=模板生成文档`、`ref_type=document`、`ref_id=新文档 document_id`、`title=生成文档名`（缺省回退模板名）、按 `tenant_id`/`user_id` 隔离、按 `(ref_type,ref_id)` 幂等的记录，使模板来源进入 §6.4 最近任务并供 c05 经 `ref_id=document_id` 恢复（写入侧 owner=c08，展示/恢复编排 owner=c05/c01；对齐 c05 D4 投递契约 `c08→template`）；并在同事务产生一条 c01 契约形态的 `document_events`（`event_type=template_created`，§10.6 六类触发源之一，携带 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)`，`document_id`/`version_id` 指向新生成文档及首版本）——c08 为 `template_created` 的唯一产生方（c01 仅以契约形态承载该取值不产生），由 c03 消费侧据此触发新生成文档的解析与索引，c08 仅产生不消费。
5. 返回新 `document_id`；前端用 c02 既有「在 ONLYOFFICE 打开文档」路由打开，**打开后由 c05 `medical-ai-panel` 拥有的「文档打开后默认展示医疗 AI 面板」触发** 经 c02 `openAIPanel` Bridge 自动展示右侧面板（无需用户点击）并按文档类型渲染默认 P0 功能面板。归属：「文档打开后默认展示医疗 AI 面板」触发 owner=c05、面板挂载与渲染本体 owner=c05、c02 仅提供 `openAIPanel`/`closeAIPanel` 打开机制；c08 作为打开模板生成文档的入口 **引用** c05 的默认展示触发，不拥有该触发、不实现面板本体；「默认/自动展示」语义对应 PRD §14.6 / §14.8 / §5.4。
6. 关键行为（使用模板、复制为个人文档）写入 `audit_logs`。

**备选方案与取舍**：

- *备选 A：调用 ONLYOFFICE Bridge 的 `createNewDocument(content, templateId)`（PRD §9.7）由编辑器侧创建*。否决作为主路径——该 API 语义偏向「带内容新建空白文档」，把模板文件字节灌入需要先取内容再转换，链路长且依赖编辑器在线；server-side copy 直接得到与模板等价的文件，更快更稳，且复用 c02 已验证的文档落地链路。`createNewDocument` 仍可作为「空白医疗专区文档」的补充能力，但不在本期主流程。
- *备选 B：前端下载模板文件再上传为新文档*。否决——多一次双向传输、占用客户端带宽、且把版权 / 可见性校验暴露到客户端，server-side copy 把校验与复制都留在服务端，更安全。
- *为何复用 `source=template` 而非新增枚举*：PRD §10.5 已含 `template`，复用即满足「模板来源版本可追溯」，符合「复用 §18 命名 + 不改 c02 契约」纪律。

**离线 / 私有化降级**：本流程不依赖任何公网或外部模型 / 解析服务——模板文件与预览图存于本地 MinIO / S3，复制为纯对象存储操作，ONLYOFFICE 为本地部署。整条「使用模板」链路在无公网环境完整可用，无需降级分支。预览图生成（见 D6）若依赖外部渲染才需降级，已在 D6 处理。

### D5. 基础可见性（本期最小过滤，非投放引擎）

本期可见性 = 三个硬条件 AND：`status=on_shelf` AND `copyright_status=confirmed` AND 租户可见域匹配。租户可见域用 `templates.tenant_id` 表达：`NULL`（或约定的 `0`/平台域）表示「全平台可见」，非空表示「指定租户可见」。查询时过滤 `templates.tenant_id IS NULL OR templates.tenant_id = :currentTenant`，再叠加 c01 `auth-rbac` 既有模块访问门（有效会话 + 租户内，浏览侧不新增具名访问权限点）；管理侧的上架/下架另由 c01 `template:manage` 权限点约束（见 D9）。

**备选与取舍**：

- *备选：本期就建 `template_delivery_rules` 并实现按医院 / 科室 / 角色 / 标签的规则匹配*。否决——PRD §22.1 不含高级投放，§14.7 投放属 V1.1；本期实现规则引擎是范围外的过度设计。`template_delivery_rules` 表仅作 V1.1 预留（proposal 已声明），本期不创建其规则逻辑，也不在可见性查询中引用它。
- *为何用 `templates.tenant_id` 而非多对多投放表表达「全平台 vs 指定租户」*：这是 PRD §14.7 可见性枚举里最粗粒度的两档（「全平台可见」「医疗租户可见」），用主表单列即可覆盖 POC 验收，零额外联表；细粒度（医院 / 科室 / 角色）留给 V1.1 的 `template_delivery_rules`。

### D6. 预览图与模板源文件的存储与生成

- 模板源文件与预览图均存对象存储（MinIO / S3），`templates.storage_key` / `preview_image_key` 仅存引用，与文档中心同一存储底座。
- 预览图在**模板入库时离线预生成**（随种子数据一并产出 PNG/JPG），不在请求期实时渲染。卡片直接返回预览图 URL（带签名 / ACL 控制的临时访问）。

**备选与取舍**：

- *备选：请求期用 ONLYOFFICE / headless 渲染实时生成缩略图*。否决——实时渲染慢、增加编辑器负载，且对 200 个静态模板无意义；入库预生成一次即可，列表读零渲染。
- **离线 / 私有化降级**：预览图生成属于内容生产（交付计划，见 D7）的离线工序，由内容团队在交付前用本地工具（ONLYOFFICE 本地实例 / LibreOffice headless 导出图片）产出并随种子数据导入；运行期不依赖任何在线渲染服务，因此私有化 / 无公网环境运行期零外部依赖。若某模板预览图缺失，卡片回退到「按 `file_type` 的占位图」，不阻塞使用。

### D7. 200 个模板的来源与组织方式（交付计划）

200 个模板**不通过运营后台在线上传**（§17.4 后台属 V1.1），而是作为**数据初始化交付物**：模板源文件 + 预览图 + 元数据（含版权字段）以结构化清单（如 CSV / JSON 清单 + 文件目录）形式提交，由幂等的种子 / 导入脚本写入 `templates` / `template_categories` 与对象存储。

来源构成（满足 PRD §14.2「可由系统自建 / 用户提供 / AI 辅助生成，但版权归属清楚」）：

- **自建（`self_built`）**：参照公开格式规范 / 国家与行业标准（如病历书写规范、知情同意通用框架）由内容团队原创排版——版权清晰，作为主力来源。
- **AI 辅助（`ai_assisted`）**：用模型生成初稿排版结构，再由人工审定并标注，`copyright_status` 经人工确认后置 `confirmed`。
- **授权（`licensed`）**：仅在确有可商用 / 可分发授权时纳入，`copyright_note` 必须记录授权方与授权范围。

每个模板入库前必须满足资产要求（模板文件 / 预览图 / 版权确认 / 分类标签 / 适用场景 / 是否可商用 / 创建人 / 版本号齐备）且 `copyright_status=confirmed` 才允许 `on_shelf`。200 个在 6 分类间的具体配额作为内容生产细节，见 Open Questions Q1。

**取舍**：用种子脚本而非临时手工 SQL 或在线后台——脚本幂等、可重复执行、可纳入部署流程，且不需要为 POC 提前建 V1.1 管理后台；符合「最小代码 / 不做范围外能力」。

### D8. 接口与入口统一

两个入口（左侧导航「医疗模板库」、新建页「医疗专区」）共用同一组后端接口：列表 / 搜索 / 分类筛选（支持 `category_id`、关键词 over `name`+`tags`+`scene_desc`、分页）、模板详情 / 预览、收藏 / 取消收藏、使用模板。新建页「医疗专区」只是同一接口在「按 6 分类聚合 + 搜索」的前端编排，后端不为入口分叉。

**取舍**：单一后端契约服务两入口，避免重复实现与不一致；前端差异仅在布局编排。

### D9. 模板上架/下架运行时管理操作（§17.8 POC 必做）

PRD §17.8「V1.0 POC 后台必做清单」明列「模板分类、标签、预览图、上架 / 下架」为 POC 必做，且 §22.1 将「管理后台」列为 P0。因此把「上架/下架」从 §17.4 整体延后中剥离，纳入本期最小实现：

- 数据模型已有 `templates.status`（`on_shelf` / `off_shelf`，D1），上架/下架只是该列的状态机切换，无需新增表。
- 管理操作受 c01 `auth-rbac` 定义的 `template:manage` 权限点约束（owner=c01，本能力仅键取、仅授 admin），普通用户不可操作。访问门（浏览侧）复用 c01 既有模块访问判定，不新增具名访问权限点。
- 下架后复用 D5 基础可见性的 `status=on_shelf` 过滤即对普通用户不可见 / 不可用（列表 / 搜索 / 筛选 / 预览 / 使用模板），重新上架恢复，零额外过滤逻辑。
- 每次切换写 `audit_logs`（操作者 / `tenant_id` / 模板标识 / 前后状态 / 时间），与既有审计口径一致。

**范围边界**：本期仅做上架/下架运行时状态机操作；§17.4 的在线上传 UI、投放规则编辑、§14.7 高级投放仍属 V1.1。这样既满足 §17.8 必做项，又不把整套管理后台拉进本期。

## Risks / Trade-offs

- **[版权未授权资产误入生产]** → `copyright_status=confirmed` 作为可见 / 可用的硬闸门（D3、D5），`pending` / `restricted` 一律不可见；种子脚本对缺失版权字段的条目拒绝入库；明确禁止导入稻壳 / 天策资产并在交付清单中逐条记录 `copyright_source` 与 `copyright_note`。
- **[使用次数冗余计数与流水不一致]** → 自增与 `template_usage` 插入放在同一数据库事务（D2、D4）；若需校正，可由 `template_usage(action=use)` 计数重算回填 `usage_count`。
- **[200 个模板内容未按期交付，阻塞验收]** → 模板内容生产与代码实现解耦：代码侧只依赖种子脚本契约，可先用少量样例模板打通链路与验收点，再批量灌入；分类配额（Q1）尽早定。
- **[server-side copy 在不同对象存储实现差异]** → 复用 c02 已抽象的对象存储客户端（MinIO / S3 兼容 copy 语义）；若底层不支持原子 server-side copy，退化为服务端「下载到内存 / 临时流再上传」，对调用方透明。
- **[预览图缺失 / 渲染工具不一致]** → 入库预生成 + 缺失时按 `file_type` 占位图回退（D6），不阻塞使用流程。
- **[与文档落地契约耦合]** → 严格复用 c01 `documents` / `document_versions` 基础契约（owner=c01）创建个人文档与首版本、并复用 c02 ONLYOFFICE Bridge 打开机制，二者契约均不改（Non-Goals）；模板首版本由 c08 自产 `source=template` + `template_created`，不走 c02 保存回调、不产生 `save_new_version`；`source=template` 复用 PRD §10.5 既有枚举，降低耦合面。
- **[基础可见性被误当成完整投放]** → 在 spec / tasks 中显式标注 §14.7 高级投放为 V1.1 Non-Goal，`template_delivery_rules` 仅预留不实现，避免验收期范围蔓延。

## Migration Plan

本期为纯新增能力，无破坏性变更，对既有 change 数据契约零改动。

1. **建表**：新增 `templates` / `template_categories` / `template_usage`（复用 §18 命名）；不创建 `template_delivery_rules` 的规则逻辑（仅 V1.1 预留，可空表或不建，由 tasks 决定）。
2. **字典初始化**：种子写入 6 行 `template_categories`。
3. **资产导入**：运行幂等导入脚本，将模板源文件 / 预览图写入对象存储，元数据（含版权字段）写入 `templates`；仅 `copyright_status=confirmed` 置 `on_shelf`。
4. **接口与前端**：上线模板中心接口与两个入口；新建页「医疗专区」复用同一接口。
5. **验收**：对照 PRD §24.5 逐项核验（200 模板、字段齐备、分类筛选、关键词搜索、医疗专区新建、点击创建副本、副本可 ONLYOFFICE 打开）；§24.5 清单止于「副本可用 ONLYOFFICE 打开」，「默认打开医疗 AI 面板」另列回引 PRD §14.6 / §14.8（功能本体依赖 c05），不计入 §24.5 逐项清单。

**回滚策略**：能力为新增且自包含——可下线模板中心入口 / 接口、或将 `templates.status` 批量置 `off_shelf` 使其对用户不可见，不影响 c02 文档中心与既有文档（已复制生成的个人文档是独立 `documents`，与模板解耦，回滚模板中心不影响这些文档）。建表为新增表，必要时可整表回滚 drop。

## Open Questions

- **Q1**：200 个模板在 6 大分类间的具体配额分布如何确定？（内容生产细节，不阻塞架构，建议由产品 / 内容团队按医院实际高频文书需求给出，例如医疗服务占比最高。）
- **Q2**：模板源文件类型范围——是否仅 docx，还是同时覆盖 xlsx / pptx？卡片字段含「文件类型」且 `createPresentation` 暗示存在演示模板，需确认 200 个模板的文件类型构成（影响预览图生成工序与 ONLYOFFICE 打开的编辑器类型选择）。
- **Q3**：`template_delivery_rules` 在本期是「建空表预留」还是「完全不建、留到 c-V1.1」？倾向不建以保持本期最小，但若部署脚本希望表结构一次到位可建空表，由 tasks 阶段定。
- **Q4**：「全平台可见 / 指定租户可见」之外，POC 是否需要演示「指定租户可见」的实际样例（即至少一个 `tenant_id` 非空的模板）？影响种子数据与可见性过滤的演示完整度。
- **Q5**：收藏状态是否需要跨入口实时一致（左侧导航与新建页同时打开时）？POC 可接受刷新后一致，不做实时推送。
