## ADDED Requirements

### Requirement: 医疗模板库资产与数量
系统 SHALL 在 V1.0 POC 中一次性交付 200 个真实可用的医疗模板，且每个模板 MUST 版权归属清楚、可追踪版权状态，禁止复用未授权的 WPS 稻壳 / 天策模板资产。模板可由系统自建、用户提供或 AI 辅助生成，但 MUST 在入库前完成版权确认。每个模板 MUST 具备如下资产字段：模板文件、预览图、版权确认、分类标签、适用场景说明、是否可商用、创建人、版本号，并 MUST 持有可枚举的版权状态。

#### Scenario: 模板库展示 200 个真实可用模板
- **WHEN** 用户进入医疗模板库
- **THEN** 系统展示 200 个真实可用医疗模板
- **AND** 每个模板均具备模板文件、预览图、分类、标签、适用场景与版权状态

#### Scenario: 缺少版权确认的模板不得入库上架
- **WHEN** 一个候选模板缺少版权确认或版权状态不明确
- **THEN** 系统拒绝将其上架到模板库
- **AND** 该模板不出现在任何用户的可见列表中

#### Scenario: 禁止引入未授权稻壳/天策模板
- **WHEN** 候选模板被标记为来源于未授权的 WPS 稻壳 / 天策资产
- **THEN** 系统拒绝入库并记录拒绝原因
- **AND** 不向用户暴露该模板

#### Scenario: 模板资产字段完整性校验
- **WHEN** 模板入库或上架
- **THEN** 系统 MUST 校验模板文件、预览图、版权确认、分类标签、适用场景说明、是否可商用、创建人、版本号八项资产齐备
- **AND** 任一项缺失时拒绝上架并提示缺失字段

#### Scenario: 抽样核验模板真实可用而非占位空壳
- **WHEN** 从 200 个模板中按 6 大分类各抽样若干份执行「使用模板 → 复制为个人文档 → ONLYOFFICE 打开」
- **THEN** 复制生成的个人文档 MUST 含可识别的结构化骨架（标题层级 / 字段占位 / 非空正文），且可在 ONLYOFFICE 正常编辑保存
- **AND** 抽样模板 MUST NOT 为仅含表头或占位文字、不可实际套用产出有效文书骨架的空白 / 占位 docx，以此把「真实可用」（§22.1 / §24.5）落为可勾稽通过的证据

### Requirement: 模板版权状态追踪
系统 SHALL 为每个模板维护可追踪的版权状态字段，并 MUST 记录是否可商用标识。版权状态由种子 / 导入脚本设定（本期不提供运行时版权信息编辑 UI，§17.4「版权信息」编辑属 V1.1，见 design Non-Goals / D7）；重导入导致版权状态变更时，系统 MUST 由导入流程留痕（导入操作人、导入时间、版权状态前后值），且模板的展示与「使用模板」能力 MUST 受当前版权状态约束。

#### Scenario: 版权状态可查询
- **WHEN** 用户或管理员查看某模板详情
- **THEN** 系统展示该模板的版权状态、是否可商用、创建人与版本号

#### Scenario: 重导入变更版权状态时由导入流程留痕
- **WHEN** 幂等导入脚本重导入某模板且其 `copyright_status` 相对库中现值发生变更
- **THEN** 系统由导入流程记录该变更的导入操作人、导入时间与版权状态前后值
- **AND** 本期不存在运行时改写 `copyright_status` 的入口，版权状态仅经种子 / 导入脚本设定

#### Scenario: 版权状态非 confirmed 时禁止使用
- **WHEN** 某模板 `copyright_status` 非 `confirmed`（`pending` / `restricted` / 导入设为不可用）
- **THEN** 系统在模板列表与详情中停止提供「使用模板」入口
- **AND** 已存在的个人文档副本不受影响

### Requirement: 模板分类与标签组织
系统 SHALL 将模板按 6 大分类组织：医疗服务、教学科研、人力资源、行政办公、财务采购、运营管理。每个模板 MUST 归属至少一个分类并 MAY 携带多个标签。分类 MUST 可用于筛选，标签 MUST 可用于关键词检索（标签作为独立筛选维度属 PRD §14.7 高级投放，V1.1）。

#### Scenario: 按 6 大分类组织模板
- **WHEN** 用户进入医疗模板库
- **THEN** 系统提供医疗服务、教学科研、人力资源、行政办公、财务采购、运营管理 6 个分类供浏览

#### Scenario: 模板必须归属分类
- **WHEN** 模板入库
- **THEN** 系统 MUST 要求该模板归属至少一个上述分类
- **AND** 未归属分类的模板不予上架

### Requirement: 模板卡片字段
系统 SHALL 以模板卡片形式呈现模板，且每张卡片 MUST 展示以下字段：模板名称、模板分类、适用场景、文件类型、更新时间、使用次数、收藏状态、预览图、适用租户、标签。其中「适用租户」字段本期仅渲染「全平台可见 / 指定租户可见」两档（对应 `templates.tenant_id` 为 `NULL` / 非空），医院 / 科室 / 角色档随 PRD §14.7 高级投放在 V1.1 落地。

#### Scenario: 模板卡片展示完整字段
- **WHEN** 用户浏览模板列表
- **THEN** 每张模板卡片展示模板名称、模板分类、适用场景、文件类型、更新时间、使用次数、收藏状态、预览图、适用租户与标签
- **AND** 「适用租户」本期取值仅渲染「全平台可见」或「指定租户可见」两档（对应 `tenant_id` `NULL` / 非空），不渲染医院 / 科室 / 角色档（§14.7 V1.1）

#### Scenario: 使用次数随使用行为更新
- **WHEN** 某模板被成功「使用」一次
- **THEN** 系统将该模板的使用次数加一并在卡片上反映

#### Scenario: 收藏状态可切换并按用户区分
- **WHEN** 用户对某模板执行收藏 / 取消收藏
- **THEN** 系统按当前 user_id 持久化该收藏状态
- **AND** 收藏状态仅对该用户生效，不影响其他用户

### Requirement: 模板搜索与分类筛选
系统 SHALL 支持关键词搜索与分类筛选（关键词覆盖名称 / 适用场景 / 标签，对齐 PRD §24.5「支持分类筛选」「支持关键词搜索」）。标签仅作为关键词检索维度命中，不作为独立筛选 facet（标签独立筛选属 PRD §14.7 高级投放，V1.1）。搜索与筛选 MUST 在当前用户的可见模板范围内进行，MUST NOT 返回对当前用户不可见的模板。

#### Scenario: 关键词搜索
- **WHEN** 用户输入关键词搜索
- **THEN** 系统返回名称、适用场景或标签命中关键词且对当前用户可见的模板
- **AND** 结果不包含对当前用户不可见的模板

#### Scenario: 分类筛选
- **WHEN** 用户选择某一分类进行筛选
- **THEN** 系统仅返回该分类下且对当前用户可见的模板

#### Scenario: 搜索无结果的边界
- **WHEN** 关键词在可见范围内无任何命中
- **THEN** 系统返回空结果并提示未找到匹配模板
- **AND** 不抛出错误也不泄露不可见模板的存在

### Requirement: 模板预览
系统 SHALL 允许用户在「使用模板」之前预览模板内容或预览图。预览 MUST NOT 修改模板原始资产，也 MUST NOT 创建任何个人文档副本。

#### Scenario: 使用前预览模板
- **WHEN** 用户点击某模板并选择预览
- **THEN** 系统展示该模板的预览图与适用场景等信息
- **AND** 不生成个人文档副本、不改变模板资产

#### Scenario: 预览不可见模板被拒绝
- **WHEN** 用户尝试预览对其不可见的模板
- **THEN** 系统拒绝预览并提示无权访问

### Requirement: 使用模板创建个人文档
系统 SHALL 提供「使用模板」核心流程：用户点击「使用模板」后，系统 MUST 将模板复制为该用户的个人文档，落入文档中心并按 `documents` / `document_versions` 进行版本管理；复制后的个人文档 MUST 可由 ONLYOFFICE 打开，且打开后 MUST 默认展示医疗 AI 右侧面板（对应 PRD §14.6 / §14.8 / §5.4，无需用户点击）。其中「文档打开后默认展示医疗 AI 面板」的触发 **owner 为 c05 `medical-ai-panel`**：本能力 **引用** c05 定义的默认展示触发，由 c05 在 ONLYOFFICE 打开模板生成文档后经 c02 `onlyoffice-bridge` 的 `openAIPanel` Bridge 自动展示并按文档类型渲染面板本体；c02 仅提供 `openAIPanel` / `closeAIPanel` 打开机制，本能力既不拥有该默认展示触发、也不实现面板渲染本体，仅作为打开模板生成文档的入口引用 c05 的默认展示触发。复制生成的个人文档 MUST 归属当前用户并受文档级 ACL 约束，MUST NOT 改动模板原始资产。复制成功并生成个人文档（含首个 `document_versions`）时，该首版本的 `source` MUST 取 c01 §10.5 枚举规范值 `template`（`source ∈ {user_edit, ai_writeback, translation, import, template}`，owner=c01，本能力仅键取该枚举不新增）。复制成功并生成个人文档（含首个 `document_versions`）时，本能力 MUST 产生一条 c01 契约形态的 `document_events`（`event_type=template_created`，§10.6 六类触发源之一），携带 c01 规定的稳定字段 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)`，`document_id` / `version_id` 指向新生成的个人文档及其首版本；c08 为 `template_created` 事件的唯一产生方（owner=c08，c01 仅以契约形态承载该取值不产生），该事件由 c03 消费侧触发新生成文档的解析与索引，本能力仅产生事件、不实现消费侧逻辑。该 `document_events` 产生 MUST 与 `documents` / `document_versions` / `template_usage` / `recent_tasks` 写入处于同一事务，保证模板复制成功即落事件、失败即不落。

#### Scenario: 使用模板复制为个人文档并由 ONLYOFFICE 打开
- **WHEN** 用户对一个对其可见的模板点击「使用模板」
- **THEN** 系统将模板复制为该用户的个人文档并写入文档中心
- **AND** 复制后的个人文档可由 ONLYOFFICE 打开
- **AND** 打开后默认展示医疗 AI 右侧面板

#### Scenario: 模板生成文档打开后引用 c05 默认展示触发
- **WHEN** 模板生成文档在 ONLYOFFICE 打开
- **THEN** 系统经 c05 `medical-ai-panel` 拥有的「文档打开后默认展示医疗 AI 面板」触发自动展示面板，本能力（c08）仅作为打开入口引用该触发、不自定义触发逻辑、不实现面板渲染本体
- **AND** 默认展示的触发与渲染由 c05 经 c02 `openAIPanel` 机制完成，c08 传入新生成文档的 `document_id` 供 c05 按文档类型渲染面板本体

#### Scenario: 个人文档归属当前用户并受 ACL 约束
- **WHEN** 用户使用模板生成个人文档
- **THEN** 该文档的 owner 为当前 user_id 且归属当前 tenant_id
- **AND** 其它无权用户按文档级 ACL 被拒绝访问该文档

#### Scenario: 模板复制成功产生 template_created 触发事件
- **WHEN** 用户使用模板成功复制为个人文档并生成首个 `document_versions`
- **THEN** 该首个 `document_versions` 的 `source` = `template`（取 c01 §10.5 枚举规范值，不写 import / user_edit 等其它值）
- **AND** 系统在同一事务产生一条 `event_type=template_created` 的 `document_events`，携带 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)` 稳定契约字段，`document_id` / `version_id` 指向新生成文档及其首版本
- **AND** 该事件可被 c03 消费侧用于触发新生成文档的解析与索引，c08 仅产生不消费

#### Scenario: 复制不改动模板原始资产
- **WHEN** 用户基于某模板生成个人文档并在 ONLYOFFICE 中编辑
- **THEN** 编辑只作用于个人文档副本
- **AND** 模板原始文件、预览图与使用次数以外的资产保持不变

#### Scenario: 对不可见或不可用模板拒绝使用
- **WHEN** 用户尝试对其不可见、或版权状态为不可用的模板执行「使用模板」
- **THEN** 系统拒绝创建副本并提示不可用
- **AND** 不写入任何个人文档

#### Scenario: 模板生成的临床文书草稿不绕过既有确认链路
- **WHEN** 由模板生成的个人文档属于病历 / 知情同意等临床文书且后续通过医疗 AI 面板产生诊疗 / 用药 / 医嘱相关内容
- **THEN** 系统将该内容视为草稿 / 辅助建议
- **AND** 仍需经既有医生（或授权审核角色）确认链路与 AI 写回前确认，本能力不改变也不绕过该确认要求

### Requirement: 模板生成写入最近任务
系统 SHALL 在「使用模板」成功复制为个人文档后，于同一事务（与 `documents` / `document_versions` / `template_usage` 写入同事务）向 c01 所建 `recent_tasks` 写入（upsert）一条最近任务记录，使模板来源进入 §6.4 六类来源并可被 c05 恢复编排。该记录 MUST 取 `source=模板生成文档`（对齐 c01 recent-tasks-shell 的 §6.4 source 规范值）、`ref_type=document`、`ref_id` 指向生成的个人文档 `document_id`、`title` 取生成文档名（缺省回退模板名），并 MUST 按 `tenant_id` / `user_id` 隔离、按 `(ref_type, ref_id)` 幂等（同一生成文档重复触发只更新不重复插入）。最近任务的展示规则（§6.5）与恢复编排（§6.6 模板生成：模板 ID / 生成文档 / 使用时间 / 编辑状态）归 c05 recent-tasks 与 c01 列表壳，本能力仅负责模板来源条目的写入。

#### Scenario: 使用模板后写入 source=模板生成文档 的最近任务记录
- **WHEN** 用户使用模板成功复制为个人文档
- **THEN** 系统在同事务向 `recent_tasks` upsert 一条 `source=模板生成文档`、`ref_type=document`、`ref_id=生成文档 document_id` 的记录，`title` 取生成文档名
- **AND** 该记录可被 c05 经 `ref_id=document_id` 恢复模板生成任务并打开生成文档（详情字段映射归 c05 / c01）

#### Scenario: 最近任务记录按租户与用户隔离且幂等
- **WHEN** 同一生成文档对应的最近任务被重复触发写入
- **THEN** 系统按 `(ref_type, ref_id)` 幂等 upsert，仅更新该条记录而不产生重复条目
- **AND** 该记录按 `tenant_id` / `user_id` 隔离，用户 MUST NOT 看到其它租户或非授权用户的模板生成任务

### Requirement: 模板库两个入口
系统 SHALL 提供两个进入医疗模板库的入口：入口 1 为医疗空间左侧导航的「医疗模板库」；入口 2 为文档中心 / 在线编辑器新建页的「医疗专区 → 选择模板」。两个入口 MUST 进入同一模板能力，且其搜索、筛选、预览、使用模板行为一致。

#### Scenario: 左侧导航入口进入模板库
- **WHEN** 用户在医疗空间左侧导航点击「医疗模板库」
- **THEN** 系统进入模板库并展示对其可见的模板

#### Scenario: 新建页医疗专区入口进入模板库
- **WHEN** 用户在文档中心 / 新建页选择「医疗专区」
- **THEN** 系统进入模板专区并支持浏览或搜索模板
- **AND** 点击「使用模板」后复制为个人文档并在 ONLYOFFICE 打开、自动展示医疗 AI 面板

#### Scenario: 两个入口行为一致
- **WHEN** 用户分别从左侧导航与新建页医疗专区对同一模板执行「使用模板」
- **THEN** 两条路径生成等价的个人文档并触发相同的 ONLYOFFICE 打开与医疗 AI 面板默认展示行为

### Requirement: 模板可见性与多租户隔离
系统 SHALL 按 `tenant_id` 可见域逐模板过滤模板可见性（`tenant_id IS NULL` 表示全平台可见 / `tenant_id` 非空表示指定租户可见），并叠加模板中心整体 RBAC 访问门，确保用户仅能浏览、搜索、预览和使用对其可见的模板。本期模板中心访问门复用 c01 `auth-rbac` 既有的模块访问判定（已登录且在所属租户内的有效会话即可访问模板中心浏览侧），不新增具名「访问」权限点；模板管理侧的运行时操作另由 c01 定义的 `template:manage` 权限点约束（见「模板上架/下架管理操作」Requirement）。本期仅实现基础可见性呈现；per-template 的 role / 医院 / 科室 / 角色 / 标签粒度的高级投放规则不在本期范围（属 PRD §14.7 高级投放，V1.1）。

#### Scenario: 仅展示对当前用户可见的模板
- **WHEN** 用户进入模板库
- **THEN** 系统按 `tenant_id` 可见域（全平台 `NULL` 或与当前租户匹配）逐模板过滤、并叠加 c01 `auth-rbac` 既有模块访问门（有效会话 + 租户内）后，仅返回对该用户可见的模板
- **AND** 不可见模板不出现在列表、搜索与筛选结果中

#### Scenario: 跨租户访问被隔离
- **WHEN** 用户尝试访问 `tenant_id` 非空且不等于其当前租户的模板
- **THEN** 系统拒绝访问

#### Scenario: 高级投放规则不在本期生效
- **WHEN** 在本期 POC 评估模板可见性
- **THEN** 系统仅依据基础可见性（`tenant_id` 可见域 + c01 `auth-rbac` 既有模块访问门）判定
- **AND** 不执行 per-template 按医院 / 科室 / 角色 / 标签的高级投放规则（属 PRD §14.7 V1.1）

### Requirement: 模板上架/下架管理操作
系统 SHALL 提供管理员侧的模板上架/下架运行时管理操作（对应 PRD §17.8 V1.0 POC 后台必做清单「模板分类、标签、预览图、上架 / 下架」、§17.4 模板管理）。该管理操作 MUST 受 c01 `auth-rbac` 定义的 `template:manage` 权限点约束（`template:manage` 为模板中心管理权限的唯一真值，owner=c01，本能力仅键取不自定义；仅授予 `admin`）：持有 `template:manage` 权限点的管理员 MUST 能将 `templates.status` 在 `on_shelf` / `off_shelf` 间切换；不具 `template:manage` 的普通用户 MUST NOT 执行该操作。模板被下架（`status=off_shelf`）后，该模板在列表 / 搜索 / 筛选 / 预览 / 使用模板对普通用户 MUST 不可见、不可用（与既有基础可见性过滤 `status=on_shelf` 一致）；重新上架（`status=on_shelf`）后恢复可见可用。其中「重新上架恢复可见」对 `copyright_status=confirmed` 的前置由 D5 读侧基础可见性过滤（`status=on_shelf` AND `copyright_status=confirmed`）等价保证：即便管理员把 `copyright_status` 非 `confirmed` 的模板置 `on_shelf`，读侧仍因版权未确认而不对用户可见，故上架/下架状态机写侧仅切换 `status`、不强校验 `copyright_status`。每次上架/下架切换 MUST 写入一条 `audit_logs`，至少包含操作者、`tenant_id`、模板标识、切换前后状态与操作时间。本 Requirement 仅覆盖运行时上架/下架状态机操作；PRD §17.4 中的在线上传 UI / 投放规则编辑 / §14.7 高级投放属 V1.1，不在本期。

#### Scenario: 管理员下架模板后对用户不可见
- **WHEN** 具备 c01 `template:manage` 权限点的管理员将某 `on_shelf` 模板下架置为 `off_shelf`
- **THEN** 系统将该模板 `status` 更新为 `off_shelf`
- **AND** 该模板不再出现在普通用户的列表、搜索、筛选、预览结果中，且「使用模板」对其不可用

#### Scenario: 管理员重新上架恢复可见
- **WHEN** 管理员将某 `off_shelf` 模板重新上架置为 `on_shelf`（且 `copyright_status=confirmed`）
- **THEN** 系统将该模板 `status` 更新为 `on_shelf`
- **AND** 该模板对可见域内用户恢复出现在列表 / 搜索 / 预览并可「使用模板」

#### Scenario: 非授权用户不能执行上架/下架
- **WHEN** 不具备 c01 `template:manage` 权限点的用户尝试执行上架/下架操作
- **THEN** 系统拒绝该操作并提示无权限
- **AND** 模板 `status` 不发生变更

#### Scenario: 上架/下架写审计
- **WHEN** 管理员成功执行一次上架或下架切换
- **THEN** 系统写入一条 `audit_logs`，包含操作者、`tenant_id`、模板标识、前后状态与操作时间

### Requirement: 模板使用与复制行为审计
系统 SHALL 将模板的「使用模板」、复制为个人文档等关键行为写入 `audit_logs`。审计记录 MUST 至少包含操作用户、tenant_id、模板标识、生成的文档标识、操作时间与操作结果，以保证可追溯。

#### Scenario: 使用模板生成审计记录
- **WHEN** 用户成功使用某模板复制为个人文档
- **THEN** 系统在 `audit_logs` 写入一条记录，包含操作用户、tenant_id、模板标识、生成文档标识、操作时间与结果

#### Scenario: 失败操作也留痕
- **WHEN** 用户因模板不可见 / 不可用导致「使用模板」失败
- **THEN** 系统写入一条标记为失败的审计记录并附拒绝原因
