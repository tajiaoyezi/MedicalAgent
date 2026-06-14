## ADDED Requirements

### Requirement: 知识库基表建表归属
本 phase（c06）SHALL 作为 PRD 第 18 章命名的 `knowledge_bases` 与 `kb_documents` 两张基表的唯一建表 owner 新建这两张表（c01/c02/c03 均不建）。`knowledge_bases` MUST 至少包含 `kb_id`（主键）、`tenant_id`、`name`、`description`、`created_by`、`is_seed`、`is_pinned`、`manual_weight`（可空）、`data_source`、`member_count`、`document_count`、`created_at`、`updated_at`；`kb_documents` MUST 包含 PRD 第 11.5.1 节导入 10 必录字段并带 `tenant_id`/`kb_id` 外键。其余被消费的表（`document_chunks`/`embeddings` 由 c03 所建、`citations` 由 c04 所建、`document_permissions`/`audit_logs` 由 c01 所建、`privacy_detection_rules`/`privacy_redaction_events` 由 c09 所建）MUST NOT 在本 phase 重复建表。

#### Scenario: 新建知识库基表
- **WHEN** 执行本 phase 的数据库迁移
- **THEN** 系统 SHALL 创建 `knowledge_bases` 与 `kb_documents` 两张基表
- **AND** `knowledge_bases` 含上述卡片与隔离字段、`kb_documents` 含 11.5.1 的 10 必录字段与 tenant_id/kb_id 外键

#### Scenario: 不重复建他人 owner 的表
- **WHEN** 执行本 phase 的数据库迁移
- **THEN** 系统 MUST NOT 创建 `document_chunks`/`embeddings`/`citations`/`document_permissions`/`audit_logs`/`privacy_detection_rules`/`privacy_redaction_events`
- **AND** 仅以消费/写入方式使用这些由 c01/c03/c04/c09 所建的表

### Requirement: 预置 13 个医疗知识库清单
系统 SHALL 在 V1.0 POC 默认预置 PRD 第 11.2 节规定的 13 个医疗知识库，每个知识库以「预置空库 + 演示资料库」形式存在，且全部 MUST 具备上传、导入、检索、问答、溯源与权限过滤能力。预置清单与名称 MUST 与 PRD 第 11.2 节表格一致，每个默认知识库 MUST 至少含 1 份演示文档以供验收。该批每库 ≥1 份知识库演示文档的资产装载 owner MUST 为本能力（c06）：演示文档经本 phase 导入管线装载并标 `authorized`；c09（§20.4 内置 Demo 数据集/验收测试集）仅引用本批已装载文档纳入验收清单、不重复装载（与 200 模板由 c08 装载、c09 引用同款边界）。

#### Scenario: 默认展示 13 个知识库卡片
- **WHEN** 终端用户进入医疗知识库首页
- **THEN** 系统展示恰好 13 个预置知识库卡片
- **AND** 13 个知识库名称依次为：医院制度与流程知识库、临床指南与专家共识知识库、药品说明书与用药安全知识库、医疗质量与质控规范知识库、感染防控知识库、护理规范知识库、医学文献与 PubMed 精选知识库、科研项目与论文写作知识库、医学检查检验知识库、临床路径与病例资料知识库、患者宣教知识库、医保与病案编码知识库、行政办公与会议资料知识库

#### Scenario: 预置空库仍具备完整能力
- **WHEN** 管理员打开某个尚无正式资料的预置空库
- **THEN** 该知识库仍 SHALL 提供上传、导入、检索、问答、溯源与权限过滤入口
- **AND** 系统不因库内文档为空而禁用上述能力

#### Scenario: 每个默认库含演示资料
- **WHEN** 验收测试集加载 Demo 数据
- **THEN** 13 个默认知识库中每一个 MUST 至少包含 1 份可被检索与问答的演示文档
- **AND** 该批演示文档由本能力（c06）经导入管线装载并标 `authorized`，c09 仅引用纳入验收清单、不重复装载

### Requirement: 知识库卡片字段
每个知识库卡片 SHALL 展示 PRD 第 11.2 节规定的字段：知识库名称、知识库 ID、创建人、知识库简介、成员人数、文档数量、更新时间、数据源、置顶状态。文档数量与成员人数 MUST 反映当前租户可见范围内的真实计数。`member_count`（成员人数）的数据源 MUST 为该知识库的 ACL/`document_permissions` 授权用户去重计数（取该知识库范围内被授予读取/问答及以上权限的、当前租户内可见用户的去重数量），本期 MUST NOT 依赖独立的 `kb_members` 表（独立知识库成员关系表归 V1.1）。

#### Scenario: 卡片展示全部规定字段
- **WHEN** 终端用户查看某知识库卡片
- **THEN** 卡片 SHALL 同时展示知识库名称、知识库 ID、创建人、知识库简介、成员人数、文档数量、更新时间、数据源与置顶状态

#### Scenario: 成员人数取知识库授权用户去重计数
- **WHEN** 系统计算并展示某知识库卡片的成员人数
- **THEN** `member_count` SHALL 等于该知识库 ACL/`document_permissions` 中被授予读取/问答及以上权限的、当前租户内可见用户的去重数量
- **AND** 不统计无该知识库任何权限的用户，也不依赖独立 `kb_members` 表

#### Scenario: 文档数量按可见范围计数
- **WHEN** 用户对某知识库不具备部分文档的访问权限
- **THEN** 卡片显示的文档数量 SHALL 仅统计该用户在当前 tenant_id 下可见的文档
- **AND** 不暴露其无权访问的文档数量与标题

#### Scenario: 入库或更新后刷新更新时间
- **WHEN** 知识库新增、更新或删除文档完成
- **THEN** 卡片的更新时间与文档数量 MUST 同步刷新为最新值

### Requirement: 知识库卡片排序规则
知识库首页列表 SHALL 严格按以下优先级排序：管理员置顶优先 → 手动权重降序 → 同权重按更新时间倒序 → 无配置时按创建时间倒序。排序 MUST 在置顶状态、权重或更新时间变化后即时重算。

#### Scenario: 置顶库排在最前
- **WHEN** 管理员将某知识库设为置顶
- **THEN** 该知识库 SHALL 排在所有未置顶知识库之前

#### Scenario: 非置顶库按权重降序
- **WHEN** 两个非置顶知识库分别配置了手动权重 10 与 5
- **THEN** 权重 10 的知识库 SHALL 排在权重 5 的知识库之前

#### Scenario: 同权重按更新时间倒序
- **WHEN** 两个知识库权重相同
- **THEN** 更新时间较新的知识库 SHALL 排在较旧者之前

#### Scenario: 无排序配置回退创建时间倒序
- **WHEN** 多个知识库既未置顶、又无手动权重、更新时间相同或缺失
- **THEN** 系统 SHALL 按创建时间倒序排列

### Requirement: 终端用户功能隔离
对终端（普通）用户，系统 SHALL 默认隐藏管理类入口：导入知识库、新建知识库、公开知识、我管理的、我加入的、历史会话侧边栏。普通用户 MUST 只能看到预设医疗知识库与被显式授权的私有知识库，不得看到或操作其无权访问的知识库。

#### Scenario: 普通用户隐藏管理类入口
- **WHEN** 普通用户进入知识库首页
- **THEN** 系统 SHALL 不展示导入知识库、新建知识库、公开知识、我管理的、我加入的与历史会话侧边栏入口

#### Scenario: 普通用户仅见预设库与授权私有库
- **WHEN** 普通用户浏览知识库列表
- **THEN** 系统 SHALL 仅返回预设医疗知识库以及该用户被显式授权的私有知识库
- **AND** 不返回其它租户或其未被授权的私有知识库

#### Scenario: 普通用户绕过 UI 直接调用被拒绝
- **WHEN** 普通用户绕过前端直接请求其无权访问的知识库管理接口
- **THEN** 系统 SHALL 拒绝请求并返回无权限错误，且写入 `audit_logs`

### Requirement: 知识库级权限（kb-level ACL）
系统 SHALL 在 tenant_id 隔离基础上对每个知识库实施知识库级 ACL，区分读取、问答、上传/导入与管理（排序/置顶/权限配置/重建索引）等权限。普通用户上传 MUST 默认仅进入个人资料区、会话上下文或私有知识库，不得直接写入公共知识库；写入公共知识库 MUST 限于平台管理员或对应知识库管理员的受控操作。

本能力的角色与权限点 MUST 锚定 c01-foundation auth-rbac 的唯一真值，MUST NOT 平行重定义角色判定或自造平台级权限点：
- 「知识库管理员」MUST NOT 被理解为 c01 `roles` 表的新全局角色（c01 `roles` 唯一真值仅 `admin`/`user`/`dept`/`doctor`/`reviewer` 五类）。「知识库管理员」SHALL 定义为「在某具体知识库上持有管理级知识库 ACL 授予记录的用户（per-kb scoped grant）」，其租户内全局角色仍取 c01 `roles` 表已登记角色；普通 c01 角色 MUST NOT 自动等同于库管理员。「平台管理员」对应 c01 `roles` 的 `admin` 角色。
- 「读取/问答/上传导入/管理」四类 SHALL 是 PRD §19.1「知识库级 ACL」下的 per-kb 资源级 ACL 能力，落 `document_permissions` / 知识库 ACL 授予记录（与 D2 `member_count` 取知识库授权用户去重计数口径一致），区别于 c01 `permissions` 表的平台 RBAC 权限点；本能力 MUST NOT 在 c01 `permissions` 表自造 `kb:read`/`kb:qa`/`kb:import`/`kb:manage` 等平台级权限点名。

#### Scenario: 库管理员身份取自 per-kb 管理级 ACL 授予而非新全局角色
- **WHEN** 系统判定某用户是否为某知识库的「知识库管理员」
- **THEN** 系统 SHALL 依据该用户在该知识库上是否持有管理级知识库 ACL 授予记录（per-kb scoped grant）判定，而非依据 c01 `roles` 表的某个全局角色
- **AND** 仅持有普通 c01 角色（如 `user`）而无该库管理级 ACL 授予的用户 MUST NOT 被判定为该库管理员，且本能力 MUST NOT 在 c01 `permissions` 表自造 `kb:*` 平台级权限点

#### Scenario: 知识库管理员仅能管理自己的库
- **WHEN** 知识库管理员尝试对其管理范围之外的知识库执行排序、置顶或权限配置
- **THEN** 系统 SHALL 拒绝操作并提示无权限
- **AND** 仅允许其对自己管理的知识库执行管理操作

#### Scenario: 平台管理员可写入任意公共库
- **WHEN** 平台管理员向任意公共知识库上传或导入资料
- **THEN** 系统 SHALL 允许该写入操作并记录操作人与时间

#### Scenario: 普通用户上传不进入公共库
- **WHEN** 普通用户上传一份资料
- **THEN** 系统 SHALL 仅将其写入该用户的个人资料区、会话上下文或其私有知识库
- **AND** MUST NOT 写入任何公共知识库

#### Scenario: 跨租户访问被隔离
- **WHEN** 某租户用户请求另一租户的知识库或其卡片字段
- **THEN** 系统 SHALL 按 tenant_id 过滤并返回不存在或无权限，不泄露跨租户数据

### Requirement: 管理员创建知识库
系统 SHALL 为持有租户级 `kb:create` 权限点的用户提供「创建知识库」入口（PRD 第 11.5 / 第 17.3 节管理员/后台能力），创建后在 `knowledge_bases` 落一条带 `tenant_id`、`name`、`created_by` 的空库记录并即时出现在该租户列表中。

授权判定 MUST 锚定 c01-foundation auth-rbac 唯一真值，且 MUST NOT 以 per-kb 知识库 ACL 表达「创建尚不存在的新库」这一授权谓词（待创建的库对象在创建时尚不存在，无法在其上持有 per-kb 管理级 ACL 授予，故创建授权不可由 per-kb scoped grant 判定）：
- 创建授权谓词 SHALL 为「持有 c01 `permissions` 表登记的租户级 `kb:create` 权限点」。`kb:create` 的唯一定义/登记 owner 为 c01-foundation auth-rbac（`permissions` 表唯一真值，默认授予 `admin` 角色），本能力仅引用该权限点判定创建授权，MUST NOT 在 c01 `permissions` 表自造 `kb:create` 或其它 `kb:*` 平台级权限点、MUST NOT 新增全局角色。
- 「平台管理员」（c01 `roles` 的 `admin` 角色）默认持有 `kb:create`。若本期/后续需向非 `admin` 用户开放创建，亦经 c01 auth-rbac 把 `kb:create` 授予对应角色实现，授权判定始终落在该可枚举权限点上，而非 per-kb ACL。

该入口 MUST 受 RBAC 管控：不持有 `kb:create` 的用户（含普通用户）MUST NOT 见到或调用该入口（与「终端用户功能隔离」一致）。本期仅覆盖创建空库（13 预置库由 seed 覆盖），私有库成员管理完整后台不在本期范围。

#### Scenario: 持有 kb:create 权限点的用户创建空知识库
- **WHEN** 持有 c01 `permissions` 表登记的租户级 `kb:create` 权限点的用户（默认即 `admin` 角色平台管理员）通过创建入口新建一个知识库
- **THEN** 系统 SHALL 先按 `tenant_id` 一致校验、再判定该用户持有 `kb:create` 权限点，通过后在 `knowledge_bases` 写入一条带 `tenant_id`、`name`、`created_by` 的空库记录
- **AND** 该知识库即时出现在当前租户的知识库列表中且具备上传/导入/检索/问答/溯源/权限过滤入口

#### Scenario: 创建授权按租户级 kb:create 权限点而非 per-kb ACL 判定
- **WHEN** 系统判定某用户是否可创建一个尚不存在的新知识库
- **THEN** 系统 SHALL 以「该用户是否持有 c01 `permissions` 表登记的租户级 `kb:create` 权限点」为唯一授权谓词
- **AND** MUST NOT 试图在待创建（尚不存在）的库对象上以 per-kb 管理级 ACL 授予作为创建授权依据，且 `kb:create` 由 c01 auth-rbac 唯一定义、本能力仅引用不自造

#### Scenario: 无 kb:create 权限点的用户无创建知识库入口
- **WHEN** 不持有 `kb:create` 权限点的用户（如仅 `user` 角色的普通用户）访问知识库首页或直接调用创建知识库接口
- **THEN** 系统 SHALL 不展示创建入口，并对接口调用返回无权限错误
