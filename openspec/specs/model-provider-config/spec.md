# Model Provider Config Specification

## Purpose

模型 Provider 抽象层与配置：`model_providers` 持久化 + 四类协议接入（OpenAI-compatible / Anthropic Messages / 本地模型网关 / 第三方模型服务），Anthropic 仅限生成类能力；每种模型能力提供「公网 / 私有化」双入口独立配置；按用途经 `model_routes` 绑定 provider 与优先级，失败时 fallback 容错并在 `audit_logs` 记录四要素（provider / 失败原因 / 切换目标 / 时间戳）；连通性测试写 `provider_health_checks`；公网调用前强制消费 c09 `redaction-gateway` 的 PHI/PII 脱敏门禁（本期门禁未接入时公网默认拒绝、改走私有化降级）；配置按 `tenant_id` 隔离、RBAC 管控、敏感凭据掩码返回。

## Requirements

### Requirement: 模型 Provider 抽象层与多协议接入
系统 SHALL 提供统一的模型 Provider 抽象层，并通过 `model_providers` 表持久化每个 provider 配置；抽象层 MUST 至少支持 OpenAI-compatible API、Anthropic Messages API、本地模型网关、第三方模型服务四类协议。上层调用方 MUST 仅依赖统一调用接口，不得感知底层协议差异。系统 MUST 限制 Anthropic 协议仅用于生成类能力（LLM 聊天 / 总结 / 翻译 / 术语抽取 / 校对润色排版 / PPT 大纲），Embedding、Rerank、文档视觉解析 MUST 通过单独配置的 provider 提供。

#### Scenario: 通过 OpenAI-compatible 协议接入 provider
- **WHEN** 管理员新增一个 provider，协议选择 OpenAI-compatible，并填写 `base_url`、`api_key`、`model`
- **THEN** 系统在 `model_providers` 中持久化该配置，并使上层调用方可经统一接口调用该 provider，无需感知协议细节

#### Scenario: Anthropic 协议不可绑定 Embedding/Rerank/视觉解析
- **WHEN** 管理员尝试将一个 Anthropic Messages 协议的 provider 绑定到 Embedding、Rerank 或文档视觉解析用途
- **THEN** 系统拒绝该绑定并提示 Anthropic 协议仅用于生成类能力，Embedding/Rerank/视觉解析须单独配置 provider

### Requirement: 公网与私有化双入口配置
每一种模型能力 SHALL 同时提供「公网」与「私有化」两个独立配置入口，二者可独立配置、独立启用。公网模型配置 MUST 至少包含 `provider`、`base_url`、`api_key`、`model`、超时、重试、用途绑定、启用状态、默认优先级字段；私有化模型配置 MUST 至少包含 `provider`、内网 `base_url`、`api_key` 或 `token`、`model`、网络访问策略、用途绑定、启用状态、默认优先级字段。私有化部署 SHALL 允许完全不配置任何公网 provider，此时系统 MUST 正常运行不报错。

#### Scenario: 公网与私有化入口独立配置
- **WHEN** 管理员为「医学翻译」用途分别配置一个公网 provider 与一个私有化 provider
- **THEN** 系统将两条配置分别持久化，公网配置含超时/重试等字段、私有化配置含网络访问策略字段，且二者启用状态互不影响

#### Scenario: 私有化部署完全不配置公网模型
- **WHEN** 部署环境无公网，管理员仅配置各用途的私有化 provider，未配置任何公网 provider
- **THEN** 系统正常启动并允许所有用途经私有化路径调用，不因缺少公网 provider 报错

### Requirement: 按用途绑定的可独立配置模型能力
系统 SHALL 支持以下模型能力各自独立绑定 provider：LLM 聊天/AIMed、长文总结/文献伴读、医学翻译、Embedding、Rerank、文档视觉解析、术语抽取、校对/润色/排版、PPT 大纲生成。每种能力 MUST 可独立选择公网或私有化 provider，互不影响。`model_routes` 表 SHALL 记录用途与 provider 的绑定关系及其优先级。

#### Scenario: 不同用途绑定不同 provider
- **WHEN** 管理员将「LLM 聊天/AIMed」绑定到公网 provider A、将「Embedding」绑定到私有化 provider B
- **THEN** 系统在 `model_routes` 中分别记录两条绑定，调用 AIMed 时走 provider A、调用 Embedding 时走 provider B，互不干扰

#### Scenario: 用途未绑定任何 provider 时拒绝调用
- **WHEN** 某用途（如 Rerank）未配置任何启用的 provider，上层发起该用途调用
- **THEN** 系统拒绝调用并返回明确的「该用途未配置可用模型」错误，不静默回退到其它用途的 provider

### Requirement: 优先级路由与 fallback 容错
同一用途 SHALL 支持配置多个 provider 并按默认优先级排序。调用时系统 MUST 按优先级从高到低尝试；当高优先级 provider 调用失败或不可用时，系统 SHALL 自动 fallback 到下一优先级 provider。每一次 fallback MUST 在 `audit_logs` 中记录 provider、失败原因、切换目标与时间戳。公网 provider 不可用或调用失败时，系统 MUST 能切换到同用途的私有化 provider。

#### Scenario: 高优先级失败时按优先级 fallback
- **WHEN** 某用途配置了优先级 1 的 provider A 与优先级 2 的 provider B，调用 provider A 返回错误或超时
- **THEN** 系统自动切换到 provider B 重试，并在 `audit_logs` 写入一条记录，包含原 provider A、失败原因、切换目标 provider B 与时间戳

#### Scenario: 公网不可用时切换私有化
- **WHEN** 某用途的公网 provider 调用失败且存在同用途启用的私有化 provider
- **THEN** 系统切换到私有化 provider 完成调用，并记录公网失败原因与切换至私有化的审计日志

#### Scenario: 全部 provider 均失败时返回明确错误
- **WHEN** 某用途的所有已配置 provider 依次调用均失败
- **THEN** 系统终止重试，返回明确的不可用错误，并将每次失败的 provider 与原因写入 `audit_logs`

### Requirement: 连通性测试与健康检查
系统 SHALL 为每个 provider 提供连通性测试入口，管理员触发后系统 MUST 实际向该 provider 发起一次最小调用以验证连通性，并将结果（成功/失败、延迟、失败原因、时间戳）写入 `provider_health_checks` 表。Embedding 与 Rerank provider 的连通性 MUST 可独立测试与验收。

#### Scenario: 触发连通性测试成功
- **WHEN** 管理员对某 provider 点击「连通性测试」
- **THEN** 系统向该 provider 发起一次最小验证调用，返回成功与延迟，并在 `provider_health_checks` 写入成功记录

#### Scenario: 连通性测试失败记录原因
- **WHEN** 某 provider 的 `base_url` 不可达或鉴权失败，管理员触发连通性测试
- **THEN** 系统返回失败及具体失败原因，并在 `provider_health_checks` 写入失败记录，不将该 provider 标记为可用

#### Scenario: Embedding/Rerank 独立连通性验收
- **WHEN** 管理员分别对 Embedding provider 与 Rerank provider 触发连通性测试
- **THEN** 系统对两者各自独立发起验证调用并分别记录结果，互不影响

### Requirement: 公网模型调用前 PHI/PII 脱敏门禁接缝
在调用任意公网 provider 之前，系统 MUST 先消费 **c09 `redaction-gateway`** 的 PHI/PII 识别与脱敏判定结果（命中规则 + 脱敏后文本 + 置信度）。PHI/PII 识别+脱敏引擎本体的唯一 owner 为 c09，c01 不实现该能力；本能力仅在公网出口预留门禁接缝并强制门控，不自行实现识别脱敏算法。当识别失败、脱敏置信度不足或识别服务不可用时，系统 MUST 禁止调用公网 provider，且 SHALL 在存在同用途私有化 provider 时切换到私有化路径，否则拒绝调用。脱敏门禁触发与切换结果 MUST 写入 `audit_logs`。本期口径：在 `redaction-gateway`（c09，phase 9）接入前，公网 provider 默认按「识别服务不可用」处理而拒绝/降级，主闭环经私有化/离线路径完成；公网经脱敏放行的端到端验收随 c09 落地完成。

#### Scenario: 脱敏通过后允许调用公网模型
- **WHEN** 上层请求经公网 provider 处理，且经 c09 `redaction-gateway` 的 PHI/PII 识别与脱敏成功、置信度达标
- **THEN** 系统以脱敏后的内容调用公网 provider，并记录脱敏已通过的审计日志

#### Scenario: 识别失败时禁止公网调用并切私有化
- **WHEN** 公网调用前经 c09 `redaction-gateway` 的 PHI/PII 识别失败或置信度不足，且该用途存在启用的私有化 provider
- **THEN** 系统禁止调用公网 provider，切换到私有化 provider 完成调用，并在 `audit_logs` 记录脱敏门禁触发与切换原因

#### Scenario: 识别服务不可用且无私有化路径时拒绝调用
- **WHEN** c09 `redaction-gateway` 识别服务不可用，且该用途无可用私有化 provider
- **THEN** 系统拒绝该调用并返回明确错误，绝不在未脱敏情况下调用公网 provider，并记录拒绝审计日志

#### Scenario: 脱敏门禁未接入时公网默认拒绝（本期保守降级）
- **WHEN** c09 `redaction-gateway` 判定结果尚不可用（本期 phase 3 默认公网关闭、门禁未接入），上层发起需经公网 provider 的调用
- **THEN** 系统按「识别服务不可用」处理，禁止启用公网 provider，存在私有化 provider 时改走私有化、否则返回明确错误并落拒绝审计，绝不在未脱敏情况下明文外发

### Requirement: 模型配置的租户隔离与权限控制
模型 Provider、路由与健康检查配置 SHALL 按 `tenant_id` 隔离，并仅允许具备管理后台「模型与评测管理」权限的角色读写。非授权角色 MUST 不能查看或修改模型配置，且 `api_key`/`token` 等敏感字段 MUST 不以明文返回给前端。

#### Scenario: 非授权角色无法修改模型配置
- **WHEN** 不具备模型配置权限的用户尝试访问或修改 `model_providers` 配置
- **THEN** 系统按 RBAC 拒绝该操作并记录审计日志，配置不被改动

#### Scenario: 敏感凭据不明文返回
- **WHEN** 授权管理员在后台查看某 provider 配置
- **THEN** 系统返回配置时对 `api_key`/`token` 做掩码处理，不向前端返回明文凭据
