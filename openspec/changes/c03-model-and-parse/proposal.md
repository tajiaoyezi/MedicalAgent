## Why

主验收闭环（上传论文 → AIMed → 文档视觉/文本解析 → 检索 → 带引用回答 → 写回文档）依赖两块共享底座：一是统一的模型 Provider 抽象层，让 LLM、总结、翻译、Embedding、Rerank、视觉解析等所有模型能力都能在「公网」与「私有化」两个入口之间按用途绑定、优先级与 fallback 调度；二是把上传与保存后的文档真正转成可检索的 chunk（文本型直接切分、扫描/复杂文档先经视觉解析结构化）。c01（foundation）已落地租户、文档、对象存储与审计骨架，但尚无任何模型接入与解析能力，后续的 c04（aimed-rag-citation）、c05（ai-panel-recent-tasks）、c06（knowledge-admin）、c07（medical-translation）都无法启动。

本 phase（9 阶段中第三阶段，依赖 c01）的价值，是在离线优先与医疗安全红线约束下，先把「模型怎么接、文档怎么变成 chunk」这两件公共能力做实，为后续所有 AI 与 RAG 能力提供唯一、可降级、可审计的入口。

## What Changes

- 新增**模型 Provider 抽象层**：统一接入 OpenAI 兼容 / Anthropic Messages / 本地模型网关 / 第三方四类协议；每种模型能力（LLM 聊天/AIMed、长文总结/文献伴读、医学翻译、Embedding、Rerank、文档视觉解析、术语抽取、校对/润色/排版、PPT 大纲生成）都同时提供「公网」与「私有化」两个配置入口，可独立配置、按用途绑定。
- 新增**模型路由与容错**：同一用途可配置多个 provider 并设置优先级与 fallback；公网不可用或调用失败时切换私有化模型；记录连通性测试与健康检查结果；fallback 必须记录 provider、失败原因、切换目标与审计日志。私有化部署允许完全不配置公网模型。
- 新增**文本型文档解析入库**：保存/上传后异步发起解析作业（`document_parse_jobs`），完成 chunk 切分并写入符合 §16.3 的 chunk 元数据（含 source/page/paragraph_index/embedding/chunk_acl 等，§16.3 单一 acl 正名为 chunk 级 `chunk_acl` 物理列，由 c03 作为 document_chunks owner 物化），管理解析状态并触发索引写入。
- 新增**文档视觉解析服务**：面向扫描 PDF、图片、复杂 PDF、图表、表格与版式抽取，公网/私有化双配置；底层实现（OCR / 多模态 / 版面分析 / 表格识别 / 第三方 API）对上层透明，统一输出文本、页码、段落、坐标、标题层级、表格结构、图片位置、页眉页脚、置信度、失败原因与 chunk 定位信息，供文本解析作为结构化输入继续切分入库。
- 约定脱敏前置约束：PHI/PII 识别+脱敏引擎 `redaction-gateway` 唯一 owner 为 **c09**；c03 仅在公网模型/公网解析出口**预留门禁接缝**消费其判定结果。`redaction-gateway` 未接入前公网 provider 不启用，仅走私有化/离线路径跑通闭环；识别失败或置信度不足时禁止调用公网，仅允许私有化路径。c01 不实现 PHI/PII 识别脱敏。
- 无破坏性变更（specs/ 当前为空，本期均为新增能力，不改动既有约定）。

## Capabilities

### New Capabilities
- `model-provider-config`：模型 Provider 抽象层（OpenAI 兼容 / Anthropic Messages / 本地网关 / 第三方），公网与私有化双入口、按用途绑定、优先级与 fallback、连通性测试；可独立配置的各模型能力（LLM / 总结 / 翻译 / Embedding / Rerank / 视觉解析 / 术语抽取 / 校对润色排版 / PPT 大纲）。
- `document-parsing`：文本型文档解析入库——解析作业（`document_parse_jobs`）、chunk 切分与 chunk 元数据、解析状态、触发索引。
- `visual-parsing-service`：文档视觉解析服务（公网 / 私有化双配置），扫描 PDF / 图片 / 复杂 PDF / 图表 / 表格 / 版式抽取，结构化输出（文本 / 页码 / 段落 / 坐标 / 标题层级 / 表格结构 / 图片位置 / 置信度 / 失败原因 / chunk 定位）。

### Modified Capabilities
（无。`openspec/specs/` 当前为空，本期能力全部为新增。）

## Impact

- **受影响数据表（PRD 第 18 章）**：本 phase 唯一 owner **建表** `model_providers`、`model_routes`、`provider_health_checks`、`visual_parse_providers`、`document_parse_jobs`、`document_visual_parse_results`、`document_chunks`、`embeddings`；读取 c01 所建 `documents` / `document_versions`（保存后异步解析触发，§9.8）；脱敏相关 `privacy_detection_rules` / `privacy_redaction_events` 的建表与落地归 **c09**（`redaction-gateway`），本期仅作为公网调用前置门禁接缝消费其判定结果，不建表；解析与路由动作写入 c01 所建 `audit_logs`。`citations`、检索/问答相关读取逻辑不在本期。
- **受影响服务**：模型接入层、文档解析流水线、文档视觉解析服务；管理后台「模型与评测管理」（§17.7）的模型配置、公网/私有化配置、Embedding/Rerank/视觉解析配置入口；ONLYOFFICE 保存回调（c02）触发的异步解析挂钩。
- **对其它 phase 的依赖**：依赖 c01（foundation）的租户/RBAC/ACL、文档与对象存储、审计与脱敏骨架；被 c04（aimed-rag-citation，消费 chunk 与模型路由生成带引用回答与向量检索）、c05（ai-panel-recent-tasks）、c06（knowledge-admin，复用解析与 embedding 写入）、c07（medical-translation，复用翻译/视觉解析 provider）依赖。本期只产出 chunk 与 embedding 写入，**不含**向量库检索逻辑、RAG 检索与问答、知识库导入（分别归 c04 / c06）。
- **医疗安全 / 合规 / 人工确认 / 脱敏 / 审计**：
  - 脱敏前置——公网模型调用与公网视觉解析前必须先经 **c09 `redaction-gateway`** 完成 PHI/PII 识别与脱敏；c03 仅在公网出口预留门禁接缝消费其判定，识别失败、置信度不足或识别服务不可用时禁止走公网，须切换私有化路径，保证患者个体信息不外泄。本期 `redaction-gateway` 未接入前公网 provider 默认关闭，主闭环经私有化/离线跑通；公网放开的端到端验收随 c09 落地完成。
  - 离线降级——禁用公网模型时，主验收闭环仍须经私有化模型或离线缓存完成（§24.9）；私有化部署可不配置任何公网 provider。
  - 可审计——模型 fallback、provider 切换、连通性/健康检查、解析作业状态与失败原因均落 `audit_logs`，记录 provider、失败原因与切换目标，满足溯源要求。
  - 人工确认边界——本期为底座能力，不直接产出诊疗/用药/医嘱结论；解析输出与模型回答均为草稿/辅助素材，高风险内容的医生确认链路由 c04/c05 在消费本期产物时落实，本期通过 chunk 定位与置信度为后续溯源与确认提供依据。
