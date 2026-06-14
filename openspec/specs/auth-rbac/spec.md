# Auth RBAC Specification

## Purpose
单租户演示下的账号、登录、会话与 RBAC 权限模型（管理员/普通用户/科室/医生/授权审核五类角色及权限点），作为全平台访问控制、多租户隔离与下游权限过滤的唯一真值与基线。

## Requirements

### Requirement: 内置演示租户与演示账号
系统 SHALL 在 V1.0 POC 单租户演示模式下内置一个演示租户（`tenant`），并内置至少一个管理员演示账号与一个普通用户演示账号；系统 MUST NOT 强制接入医院 SSO / LDAP / OA。所有内置账号 MUST 归属于该内置演示租户，且演示账号的初始口令 MUST 以非明文（哈希）形式存储。

#### Scenario: 系统初始化生成内置演示租户与账号
- **WHEN** 系统首次初始化（演示数据集装载）完成
- **THEN** 存在且仅存在一个内置演示租户，并存在一个管理员演示账号与一个普通用户演示账号
- **AND** 两个演示账号的 `tenant_id` 均等于该内置演示租户的标识

#### Scenario: 不依赖外部身份源即可登录
- **WHEN** 部署环境无公网且未配置 SSO / LDAP / OA
- **THEN** 演示账号仍可凭内置用户名与口令登录，不依赖任何外部身份源

### Requirement: 用户名口令登录与会话
系统 SHALL 提供基于用户名与口令的登录；登录成功时 MUST 创建会话并将用户带入医疗空间门户（默认进入 AIMed）；登录失败时 MUST 返回明确错误且 MUST NOT 创建会话；被禁用账号 MUST NOT 登录成功。每次登录尝试（成功或失败）MUST 记录到审计日志。

#### Scenario: 正确凭据登录成功
- **WHEN** 启用状态的用户输入有效用户名与口令并提交
- **THEN** 系统创建会话并进入医疗空间门户，默认展示 AIMed 学术助手
- **AND** 记录一条登录成功审计事件

#### Scenario: 错误凭据被拒绝
- **WHEN** 用户输入错误口令
- **THEN** 系统拒绝登录并提示凭据无效，不创建会话
- **AND** 记录一条登录失败审计事件

#### Scenario: 被禁用账号拒绝登录
- **WHEN** 已被管理员禁用的用户输入正确用户名与口令
- **THEN** 系统拒绝登录并提示账号不可用，不创建会话

#### Scenario: 未登录访问受控资源被拦截
- **WHEN** 无有效会话的请求访问门户或任一受控模块
- **THEN** 系统拒绝访问并要求登录，不返回受保护数据

### Requirement: 角色模型（管理员/普通用户/科室/医生/授权审核）
系统 SHALL 提供管理员（`admin`）、普通用户（`user`）、科室（`dept`）、医生（`doctor`）、授权审核（`reviewer`）五类角色，并将角色与用户关联（`roles`）；用户 MUST 至少拥有一个角色；普通用户 MUST 可被分配科室归属。`roles` 表 MUST 作为全平台 RBAC 角色的唯一真值，下游 phase（c05 高风险写回确认、c09 安全收口）MUST 引用本表角色而 MUST NOT 平行重定义角色判定。`doctor` 与 `reviewer` 两类角色语义 MUST 对应 PRD §19.2「高风险内容确认人需具备医生或授权审核角色」，并经权限点 `highrisk:confirm` 表达「可完成高风险内容最终确认」的能力，供 c05/c09 确认链路键取 `confirmed_role`；普通用户 MUST NOT 持有 `highrisk:confirm` 权限点、MUST NOT 完成最终确认。管理员角色 MUST 能看到管理后台导航入口，普通用户 MUST NOT 看到管理后台入口。角色判定 MUST 限定在用户所属租户范围内。

#### Scenario: 管理员可见管理后台入口
- **WHEN** 管理员角色用户进入门户
- **THEN** 左侧导航展示“管理后台”入口

#### Scenario: 普通用户不可见管理后台入口
- **WHEN** 普通用户角色用户进入门户
- **THEN** 左侧导航不展示“管理后台”入口，且直接访问管理后台路由被拒绝

#### Scenario: 科室归属可分配
- **WHEN** 管理员为某普通用户设置科室归属并保存
- **THEN** 该用户记录上保存对应科室，且变更被写入审计日志

#### Scenario: 高风险确认角色与权限点可被下游键取
- **WHEN** c05/c09 的高风险写回确认链路需要判定 `confirmed_role` 是否具备最终确认资格
- **THEN** 系统提供 `doctor` / `reviewer` 角色及 `highrisk:confirm` 权限点作为唯一真值，确认链路按该角色名/权限点判定，`confirmed_role` 取值可枚举为 `doctor` / `reviewer`
- **AND** 仅持有 `user` 角色而不具 `highrisk:confirm` 的用户 MUST 被判定为不可完成最终确认

### Requirement: RBAC 权限模型与租户隔离
系统 SHALL 以 RBAC 为访问控制基线，权限（`permissions`）通过角色授予用户，所有授权判定 MUST 先校验 `tenant_id` 一致再校验角色与权限。系统 MUST 拒绝跨租户访问任何资源。RBAC 判定结果 MUST 作为后续 phase（RAG 检索、文档/知识库 ACL）按 `tenant_id`/`user_id`/`role` 过滤的前置约束。`permissions` 表 MUST 作为全平台 RBAC 权限点的唯一真值，下游 phase MUST 引用本表已登记的权限点而 MUST NOT 自造权限点名。本 phase MUST 登记的种子权限点至少包含：`highrisk:confirm`（高风险内容最终确认，授予 `doctor` / `reviewer`，供 c05/c09 键取）、`template:manage`（模板中心管理，对应 PRD §17.8「模板分类、标签、预览图、上架 / 下架」与 §19.1 RBAC，仅授予 `admin`，供 c08 模板上架/下架管理引用）、`kb:create`（创建知识库，对应 PRD §11.5「管理员能力·创建知识库」与 §17.3 后台「创建知识库」，授予 `admin`，供 c06 判定「创建尚不存在的知识库」的授权）。`kb:create` 是租户级（平台级）权限点而非 per-kb ACL——因创建一个尚不存在的新库时该库对象尚未存在、无法以 per-kb scoped grant 表达创建授权，故创建授权 MUST 以本表 `kb:create` 权限点判定；`permissions` 表为 `kb:create` 的唯一真值，c06 MUST 引用本权限点判定创建授权而 MUST NOT 自造 kb 创建权限点名、MUST NOT 以 per-kb scoped grant 表达创建授权。

#### Scenario: 越权操作被拒绝
- **WHEN** 普通用户尝试调用仅管理员可执行的操作（如用户管理、角色分配）
- **THEN** 系统返回无权限错误并拒绝执行，不产生数据变更

#### Scenario: 跨租户访问被隔离
- **WHEN** 某租户用户的请求携带另一租户资源标识
- **THEN** 系统按 `tenant_id` 过滤后判定无权限并拒绝，不泄露目标租户任何数据

#### Scenario: 授权判定可被下游消费
- **WHEN** 下游模块（文档中心、知识库、RAG）需要按 `tenant_id`/`user_id`/`role` 过滤资源
- **THEN** 该模块可复用本能力提供的租户与角色判定结果作为过滤前提

#### Scenario: 种子权限点为下游唯一真值
- **WHEN** c08 模板中心需要判定「模板上架/下架管理」权限、c05/c09 需要判定高风险最终确认权限、或 c06 需要判定「创建知识库」权限
- **THEN** 系统提供本表已登记的 `template:manage`（仅 `admin`）、`highrisk:confirm`（`doctor` / `reviewer`）与 `kb:create`（仅 `admin`）权限点作为唯一真值，下游按权限点名引用而不自造

#### Scenario: 创建知识库授权以 kb:create 权限点判定
- **WHEN** c06 需要判定某用户是否可创建一个尚不存在的新知识库
- **THEN** 系统以本表 `kb:create` 权限点（租户级、授予 `admin`）作为可枚举授权谓词判定，持有 `kb:create` 者放行创建、不持有者拒绝
- **AND** 因目标库在创建时尚不存在、无法以 per-kb scoped grant 表达，创建授权 MUST 以 `kb:create` 判定而 MUST NOT 以 per-kb 管理级 ACL 表达，c06 引用本权限点、不自造 kb 创建权限点名
