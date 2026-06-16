## Why

主验收闭环的「智能内核」是 AIMed 学术助手：从输入医学问题/选择模式，到检索 PubMed（或离线缓存）/上传文件/医疗知识库，再到流式生成「带引用角标且可点击溯源」的答案（PRD §5.1、§8、§24.1）。这是医疗安全红线「医学结论必须可溯源、不输出无依据建议」在产品上的直接落地点：没有 RAG 检索与引用溯源，AIMed 的回答既无法验证也无法采信。本期是 9 阶段中的第四阶段 aimed-rag-citation，承接 c02（ONLYOFFICE 编辑器与 Bridge、保存-版本链路）与 c03（模型 Provider 公网/私有化路由、文档解析/视觉解析入库），把已索引的多源内容组织成可检索、可溯源的问答能力，并为下游 c05（医疗 AI 面板写回与最近任务）、c06（知识库管理）、c07（医学翻译）提供 AIMed 会话与检索基座。

## What Changes

- 新增 AIMed 学术助手六大模式（通用问答 / 深度文献伴读 / 科研态势分析 / 循证证据溯源 / 智能综述生成 / 学术写作辅助），含各模式数据源约束、模式切换规则（数据源标签显隐、文件列表保留/清空、输入框内容强制保留）、占位文案、发送按钮状态机、智能模式匹配与优先级、复合任务与无关问题处理、答案生成过程/结构与答案操作栏（PRD §8、§5.1–5.2、§24.1）。
- 新增 PubMed 数据服务（收敛为「按 id/url 取数并归一化 + 离线缓存读取」的取数 provider）：公网可用时真实调用 PubMed 并支持按 PMC / DOI / URL 取回归一化；公网不可用时使用离线 PubMed 缓存/演示数据完成离线闭环；取回结果按白名单/管理员授权返回授权状态标记（authorized/preview_only/rejected），导入落库与「临时预览/正式库」授权裁决以 c06 kb-import 为唯一真值、本 phase 不发起落库；未授权商业数据库不得默认抓取或导入（PRD §16.1、§8.6）。
- 新增 RAG 检索能力：Query Rewrite → 数据源选择 → 权限过滤 → BM25 + 向量检索 → 合并去重 → rerank → source compression → 注入上下文 → 生成带引用答案；检索全程按 tenant_id / kb_id / user_id / role / document_acl / chunk_acl 六维过滤（PRD §11.9、§16.2、§16.3），其中 chunk_acl 为 c03 document_chunks 的 chunk 级 ACL 列、document_acl 为 document_permissions 派生的文档级维度。
- 新增引用与溯源能力：答案关键结论带引用角标、末尾参考资料结构化、点击角标定位来源（PubMed 文章详情 / 上传文件页码段落 / 知识库 chunk），并处理权限不足、原文已删除、外链失效、chunk 定位失败等引用异常分支（PRD §8.8、§8.9）。
- 本期不含破坏性变更：c03 之前无 AIMed 会话、RAG 检索与引用溯源能力，全部为净新增，无对既有 spec 行为的修改。
- 本期明确不做（不写入范围）：在线文档内医疗 AI 面板与选区写回（归 c05）；知识库管理与导入（归 c06，本期 RAG 仅检索已索引内容）；医学翻译模块文件级/全文翻译（归 c07，AIMed 内短文本翻译属学术写作辅助）。

## Capabilities

### New Capabilities

- `aimed-assistant`：AIMed 学术助手六大模式（通用问答 / 深度文献伴读 / 科研态势分析 / 循证证据溯源 / 智能综述生成 / 学术写作辅助）、各模式数据源约束、模式切换规则、占位文案、发送按钮状态机、智能模式匹配与优先级、复合任务与无关问题处理、答案生成过程与结构、答案操作栏。
- `pubmed-data-service`：PubMed 在线检索与离线 PubMed 缓存，公网可用/不可用两条路径，PMC / DOI / URL 导入（白名单约束），未授权商业库禁止默认抓取。
- `rag-retrieval`：RAG 检索流程（query rewrite → 数据源选择 → 权限过滤 → BM25 + 向量 → 合并去重 → rerank → source compression → 注入上下文 → 带引用答案）与 chunk 元数据、权限过滤。
- `citation-tracing`：引用与溯源，含引用角标、参考资料结构、点击定位（PubMed 文章 / 上传文件页码段落 / 知识库 chunk）、引用异常分支。

### Modified Capabilities

（无。`openspec/specs/` 当前为空，本期能力全部为新增，无既有 spec 的需求级行为变更。）

## Impact

- 受影响服务：AIMed 会话/模式编排服务、PubMed 数据服务（在线检索 + 离线缓存）、RAG 检索服务（BM25 全文检索 + 向量库 + rerank + source compression）、引用溯源服务；「生成在线 Word/在线文档」经 c01 文档中心服务端创建服务落 documents/document_versions 并由 c01 创建入口产生 §10.6 upload_success 供 c03 解析索引（本能力不依赖 c02 编辑器内 createNewDocument 服务端新建变体），再经 c02 打开链路在 ONLYOFFICE 打开；复用 c03 的模型 Provider 路由（公网/私有化、优先级与 fallback）与文档解析/视觉解析产物，复用 c01 的账号/RBAC、文档中心创建服务与对象存储。
- 受影响数据表（PRD §18）：
  - 本 phase 建表（owner=c04）：`conversations` / `messages`（AIMed 会话与答案；`conversations` 含 module/source 区分维，供 c06 知识库问答 kb_qa 会话复用同一基座写入）、`citations`（引用溯源）、`agent_runs` / `agent_steps` / `tool_calls`（AIMed/RAG 内部 AI 任务追踪）、`feedbacks`（泛化为可承载多来源反馈：`subject_type`∈{message,translation_job} + `subject_id` + `rating` + `reason`，AIMed 赞/踩与 c07 翻译质量反馈复用同一表，c07 仅写入）。`agent_checkpoints` 本期不建（V1.1 长任务断点续跑预留，§18 注记）。以上表含 tenant_id/user_id 隔离列；`citations` 绑定 message_id + source_type 定位指针。
  - 消费/写入由上游所建表（本 phase 不建）：写入 c01 所建 `recent_tasks`（AIMed 保存内容进入最近任务，展示/恢复归 c05）；消费 c03 所建 `document_chunks` / `embeddings` / `document_parse_jobs` / `document_visual_parse_results`、c01 所建 `documents` / `document_versions`（AIMed 对话内本地上传文件经 c01 文档中心上传服务落 documents/document_versions 获 document_id 并由该入口产生 §10.6 upload_success 供 c03 解析，本 phase 不直接产生 document_events）、c06 所建 `knowledge_bases` / `kb_documents`；审计相关写入 c01 所建 `audit_logs`（建表唯一 owner=c01，c04 仅写入不建表）、脱敏门禁判定消费 c09 所建 `privacy_redaction_events`（建表 owner=c09，脱敏命中/策略留痕由 c09 redaction-gateway 在公网出口单一写入，c04 仅消费门禁判定、不写不建该表）。
- 对其它 phase 的依赖与解锁：上游依赖 c02（ONLYOFFICE 集成与 Bridge、保存-版本链路）与 c03（模型 Provider 公网/私有化路由、文档解析与视觉解析入库），间接依赖 c01（账号/RBAC、文档中心、对象存储）；本期产出的 AIMed 会话与 RAG/引用基座被 c05（医疗 AI 面板复用 AIMed 技能与最近任务恢复）、c06（知识库的检索消费方与 PubMed/URL 导入沉淀的正式来源）、c07（学术写作辅助内短文本翻译与医学翻译模块分流）直接复用。本期 RAG 仅检索已索引内容，知识库的管理/导入归 c06；文档内选区写回与确认 UI 归 c05。
- 医疗安全与合规影响：
  - 可溯源红线——所有医学关键结论必须带可点击引用角标并能定位原文，未检索到资料时按规则提示「未找到相关文献」，不输出无依据的诊疗建议；AIMed 答案除医疗免责声明外按 c09 security-compliance 草稿/辅助建议标记契约（owner=c09，本期消费方）标记为草稿/辅助建议（PRD §8.8、§8.9、§24.7 第二项、§19.2）。
  - 权限与多租户——RAG 检索必须按 tenant_id / kb_id / user_id / role / document_acl / chunk_acl 六维过滤（§11.9），引用定位遇权限不足/原文已删除时降级为「该引用源暂时不可用/已删除」，不得越权暴露内容（PRD §11.9、§16.2、§16.3、§8.9）。
  - 脱敏前置——§19.4 规定 PHI/PII 识别发生在「上传时」与「调用模型时」两个时点：调用公网模型（含公网 PubMed）前必须经 c09 的 redaction-gateway（PHI/PII 识别与脱敏引擎，唯一 owner=c09）做识别与脱敏，本期为消费方、不自行实现识别脱敏；识别失败/置信度不足/识别服务不可用/gateway 未接入时禁止调用公网模型，切换 c03 提供的私有化模型与离线 PubMed 缓存路径。AIMed 文件上传入口属上传时点，文件在持久化入库或送 c03 解析前同样消费 c09 上传时 PHI/PII 识别契约（owner=c09），按策略识别并提示/脱敏后送模型/阻止上传，本期为消费方（PRD §19.4）。本期对「公网 provider」口径为：redaction-gateway 未接入前不得启用公网，仅私有化/离线路径跑通闭环（PRD §16.4/§24.9、§19、上下文红线）。
  - 数据来源合规——未授权商业数据库不得默认抓取/导入，URL 取数须经白名单或管理员授权；本 phase 仅对取回结果返回授权状态标记（authorized/preview_only/rejected），「临时预览/正式公共知识库落库」的授权裁决与落库以 c06 kb-import 为唯一真值，本 phase 不发起落库（PRD §16.1）。
  - 审计——AIMed 检索/生成与引用点击、外部导入授权写入 `audit_logs` / `agent_steps`，满足可审计红线；脱敏命中/策略留痕由 c09 redaction-gateway 在公网出口单一写入 `privacy_redaction_events`，c04 仅消费门禁判定、不写该表；所有医学回答需展示医疗免责声明。
