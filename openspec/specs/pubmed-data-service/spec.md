# PubMed Data Service Specification

## Purpose
PubMed 数据服务：在线 E-utilities 检索与离线 PubMed 缓存双路径（公网不可用或脱敏失败时降级离线）、PMC / DOI / URL 取数与授权状态标记、未授权商业数据库（万方 / 知网 / 维普）禁止默认抓取、公网 PubMed 调用前 PHI/PII 脱敏门禁。

## Requirements

### Requirement: PubMed 在线检索与离线缓存双路径
PubMed 数据服务 SHALL 提供公网可用与公网不可用两条路径。公网可用时系统 MUST 真实调用 PubMed 完成检索；公网不可用时系统 MUST 使用预置 PubMed 演示数据/离线 PubMed 缓存、管理员上传文献与本地知识库资料，完成离线演示完整闭环。两条路径返回的文献条目 MUST 包含可溯源元数据（PMID/标题/期刊/年份等），供引用溯源使用。

#### Scenario: 公网可用真实调用 PubMed
- **WHEN** 部署环境公网可用且用户在依赖 PubMed 的模式发起检索
- **THEN** 系统真实调用 PubMed 返回文献，并保留 PMID/标题/期刊/年份等可溯源字段

#### Scenario: 公网不可用走离线缓存闭环
- **WHEN** 部署环境公网不可用
- **THEN** 系统使用离线 PubMed 缓存/预置演示数据/管理员上传文献完成检索与作答
- **AND** 仍可生成带引用角标的可溯源答案

#### Scenario: 科研态势分析与循证证据溯源支持离线
- **WHEN** 用户在科研态势分析或循证证据溯源模式且公网不可用
- **THEN** 系统基于离线 PubMed 缓存完成检索，模式可用

### Requirement: PMC/DOI/URL 取数与授权状态标记
本能力 SHALL 收敛为「按 id/url 取数并归一化为 RetrievedSource + 离线缓存读取」的取数 provider：公网可用时支持按 PMC / DOI / URL 取回文献并归一化为内部 RetrievedSource。取回内容是否可落库由白名单/管理员授权决定，但导入授权门禁与「临时预览/正式库落库」裁决以 c06 kb-import 契约为唯一真值，本 phase MUST NOT 发起任何向 knowledge_bases/kb_documents 的落库写入。本能力对每个取回结果 MUST 返回授权状态标记（authorized / preview_only / rejected）供上层与 c06 消费；端到端「临时预览/正式库落库」隔离验收移交 c06 kb-import。取数与授权状态记录 MUST 写入 audit_logs。

#### Scenario: DOI 取数命中白名单标记 authorized
- **WHEN** 用户提交白名单内来源的 DOI 进行取数
- **THEN** 系统取回并归一化为 RetrievedSource，返回授权状态标记 authorized，并记录来源与授权到 audit_logs
- **AND** 本 phase 不向 knowledge_bases/kb_documents 落库（落库由 c06 kb-import 裁决执行）

#### Scenario: 授权不明确标记 preview_only
- **WHEN** 用户提交的 URL 不在白名单且无管理员授权
- **THEN** 系统返回授权状态标记 preview_only，本 phase MUST NOT 发起正式公共知识库落库写入（临时预览/正式库隔离裁决归 c06 kb-import）

#### Scenario: URL 取数授权状态由 c06 裁决
- **WHEN** 用户发起 URL 取数
- **THEN** 系统取回内容并按白名单/管理员授权返回 authorized/preview_only/rejected 标记，落库与授权裁决移交 c06 kb-import，本 phase 仅提供取数与标记

### Requirement: 未授权商业数据库禁止默认抓取
系统 SHALL NOT 将未授权商业数据库（如万方、知网、维普、临床指南库、药品说明书库等需授权来源）作为默认公网导入或抓取来源。仅当确认数据来源、版权与使用边界并取得授权后方可接入。

#### Scenario: 未授权商业库不默认抓取
- **WHEN** 系统执行默认检索或导入
- **THEN** 数据源 MUST 限于 PubMed/上传文件/医疗知识库等已授权来源，未授权商业数据库不被抓取或导入

#### Scenario: 商业库需授权方可接入
- **WHEN** 管理员尝试接入需授权的商业数据库
- **THEN** 系统要求确认来源、版权与使用边界并完成授权后方可启用，否则拒绝接入

### Requirement: 公网 PubMed 调用前脱敏门禁
调用公网 PubMed 前，系统 SHALL 消费 c09 的 redaction-gateway（PHI/PII 识别与脱敏引擎，唯一 owner=c09）对查询内容做识别与脱敏；本 phase 不自行实现 PHI/PII 识别脱敏，仅在公网出口调用该门禁接缝。当识别失败、脱敏置信度不足、识别服务不可用或 redaction-gateway 未接入时，系统 MUST NOT 调用公网 PubMed，MUST 降级使用离线 PubMed 缓存（本期默认公网关闭、离线优先）。脱敏命中与策略由 c09 redaction-gateway 在公网出口统一写入 privacy_redaction_events，本 phase 仅消费门禁判定、不另维护该表字段口径。

#### Scenario: 含 PHI 的查询脱敏后再检索
- **WHEN** 用户查询含潜在 PHI/PII 且需调用公网 PubMed
- **THEN** 系统先经 c09 redaction-gateway 脱敏再以脱敏后查询调用公网 PubMed，脱敏事件由该门禁写入 privacy_redaction_events

#### Scenario: 脱敏不可用降级离线
- **WHEN** PHI/PII 识别服务不可用或置信度不足
- **THEN** 系统禁止调用公网 PubMed，改用离线 PubMed 缓存检索
