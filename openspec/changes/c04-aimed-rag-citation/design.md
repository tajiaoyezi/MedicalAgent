## Context

AIMed 学术助手是主验收闭环（PRD §5.1）的「智能内核」：用户输入医学问题/选择模式 → 检索 PubMed（或离线缓存）/上传文件/医疗知识库 → 流式生成「带引用角标且可点击溯源」的答案 → 一键「生成在线 Word」落入文档中心。本 change（c04，9 阶段第四阶段）要把 c03 已索引的多源内容组织成可检索、可溯源的问答能力。

当前状态与上游依赖：
- c01 提供账号/RBAC、多租户、对象存储（MinIO/S3）、文档中心。
- c02 提供 ONLYOFFICE 集成、文档 Bridge API、「生成 docx → 保存文档中心 → 打开编辑器」入口与保存-版本链路（`documents` / `document_versions`）。
- c03 提供模型 Provider 公网/私有化路由（优先级 + fallback）、文档解析与视觉解析（产出 `document_parse_jobs` / `document_visual_parse_results`，含页码/段落/表格定位），并在公网出口预留脱敏门禁接缝。
- c09 提供 PHI/PII 识别与脱敏引擎 redaction-gateway（及 `privacy_detection_rules` / `privacy_redaction_events`），是被 c03–c07 前置消费的横切能力；本 phase 在公网出口消费该门禁接缝，不自行实现 PHI/PII 识别脱敏。本期对「公网 provider」口径为：redaction-gateway 未接入前不得启用公网，仅私有化模型 / 离线 PubMed 缓存路径跑通闭环（§16.4/§24.9）。
- `openspec/specs/` 当前为空，本期能力全部净新增。

约束（不可违背的医疗安全红线，源自 instruction context 与 PRD §16/§19）：
- 医学关键结论必须带可点击引用角标并能定位原文；未检索到资料时按规则提示，不输出无依据建议；生成内容默认草稿。
- RAG 检索必须按 `tenant_id / kb_id / user_id / role / document_acl / chunk_acl` 六维过滤（§11.9）；其中 `chunk_acl` 为 c03 document_chunks 的 chunk 级 ACL 物理列（owner=c03，默认继承来源文档 ACL、可严于文档级），`document_acl` 为文档级维度（非 chunk 物理列）：对 c04 自有来源（上传文件/当前文档/团队文档）由本 phase 直接按 c01 `document_permissions`（phase 1 即就绪）派生的可见 document 集合独立执行、不依赖 c06；对知识库（kb）来源可由 c06 注入预计算可见 document 集合作为优化路径（非 document_acl 维唯一执行方）；引用定位遇权限不足/已删除时降级不越权。
- 调用公网模型（含公网 PubMed）前必须先脱敏；识别失败/置信度不足/服务不可用时禁止调用公网，可切私有化模型。
- 未授权商业数据库不得默认抓取/导入；URL 导入须白名单或管理员授权。
- 离线优先：部署环境可能无公网，核心闭环必须有离线 PubMed 缓存/私有化模型降级路径。
- 数据模型遵循 PRD §18 核心表命名与唯一建表 owner 约定：本 phase（c04）建表 `conversations` / `messages` / `citations` / `agent_runs` / `agent_steps` / `tool_calls` / `feedbacks`（`agent_checkpoints` V1.1 预留、本期不建）；其余消费表（documents/document_versions 归 c01；document_chunks/embeddings/document_parse_jobs/document_visual_parse_results 归 c03；knowledge_bases/kb_documents 归 c06；recent_tasks 归 c01；audit_logs 归 c01；privacy_redaction_events 归 c09）由各自 owner 所建，本 phase 仅消费/写入、不建。

利益相关方：医疗科研工作者（主用户）、医生/审核角色（高风险内容确认链路）、管理员（数据源授权/模型配置）。

## Goals / Non-Goals

**Goals:**

1. 实现 AIMed 六大模式（通用问答 / 深度文献伴读 / 科研态势分析 / 循证证据溯源 / 智能综述生成 / 学术写作辅助）的数据源约束、模式切换规则、占位文案、发送按钮状态机、智能模式匹配（关键词→Tab 高亮提示，不强制切换）、复合任务与无关问题处理。
2. 实现统一 PubMed 检索接口：公网可用走真实 PubMed（含 PMC/DOI/URL 导入，受白名单约束），公网不可用走离线 PubMed 缓存/演示数据，对上层透明降级。
3. 实现 RAG 检索管线：Query Rewrite → 数据源选择 → 权限过滤 → BM25 + 向量检索 → 合并去重 → rerank → source compression → 注入上下文 → 生成带引用答案。
4. 实现引用溯源：答案带角标、末尾结构化参考资料、点击定位（PubMed 详情 / 上传文件页码段落 / 知识库 chunk）、引用异常分支。
5. 答案「生成在线 Word / 保存为」经 c02 Bridge 与文档中心落地；`生成在线 Word` 后由 c05 拥有的「文档打开后默认展示医疗 AI 面板」触发自动展开面板（默认展示触发与面板本体均归 c05，本期仅产出引用该触发并传入新生成文档 `document_id` 的打开入口信号，不自建/重定义该触发）。
6. 命中率与引用定位指标可观测（PRD §20.3）。

**Non-Goals:**

- 在线文档内医疗 AI 面板的选区写回与确认 UI（归 c05；本期只产出「展开面板」入口信号）。
- 知识库管理/导入/13 卡片/URL 与 PubMed 导入落库（归 c06；本期 RAG 仅检索 c06 已索引内容，离线缓存的预置/演示数据由 c06/部署脚本提供）。
- 医学翻译模块文件级/全文异步翻译（归 c07；AIMed 内短文本翻译属学术写作辅助，直接走模型生成，不建 translation_jobs）。
- chunk/embedding 生产侧（解析、切分、向量化入库归 c03/c06）；本期为消费方。
- 上游所建 §18 表（documents/document_versions 归 c01；document_chunks/embeddings/document_parse_jobs/document_visual_parse_results 归 c03；knowledge_bases/kb_documents 归 c06；recent_tasks 归 c01；audit_logs 归 c01；privacy_redaction_events 归 c09）的建表 DDL 不在本期（本期仅消费/写入）；模型 Provider 路由的实现归 c03、PHI/PII 识别与脱敏引擎（redaction-gateway）的实现归 c09，本期为调用方/消费方。
- `agent_checkpoints` 表 V1.0 不建（仅为 V1.1 长任务断点续跑预留，§18 注记）。
- 答案操作栏「生成 PPT 大纲」的生成逻辑（PPT 生成 / 论文转 PPT 属 §22.2 V1.1，本期仅占位/禁用入口，与 c05 §22.2 PPT 排除口径一致，移出本期 P0 实现范围）。
- V1.1/V1.2 能力（中文文献库、临床指南/药品说明书库、数字员工等）。

## Decisions

### D1 AIMed 会话/消息模型与六模式数据源约束的落点

**决策**：本 phase 新建 §18 `conversations` / `messages`（建表 owner=c04，见 D8），会话粒度对应一次会话；模式（mode）、当前数据源约束（allow_pubmed/allow_upload/allow_current_doc）、已上传文件列表作为会话级状态存于 `conversations`（扩展列或 metadata JSON，仅作会话级文件清单展示）；每条用户输入与每个答案版本各为一条 `messages`，答案的引用集合写本 phase 新建的 `citations`，关联 `message_id`。模式的数据源/文件约束（PRD §8.2）由后端 `mode_policy` 配置表驱动（非硬编码到代码分支），六模式的 policy 是一份声明式映射：

注意：`conversations.uploaded_files` JSON 仅作会话级文件清单的轻量缓存，**不替代文档持久化**。AIMed 对话内「本地文件」上传 MUST 经 c01 文档中心上传服务落为 `documents` / `document_versions` 并获得 `document_id`、由该入口产生 §10.6 `upload_success` 事件供 c03 解析（见 aimed-assistant「文件上传与管理」Requirement 与 F19 主闭环契约），使深度文献伴读的上传论文进入「上传 → 解析 → chunk → 带引用回答」主闭环，引用定位以真实 `document_id` 为准；本 phase 自身不直接产生 document_events。

为支持 c06 知识库问答（kb_qa）会话复用同一会话/消息基座（Decision B），`conversations` 增 `module`（aimed/kb_qa）与 `source` 两个区分维度：AIMed 六大模式会话 module=aimed、source=「AIMed 学术助手」；c06 知识库问答会话经本表接口写入 module=kb_qa、source=「医疗知识库问答」（`mode` 在 kb_qa 下不强制取 AIMed 六模式枚举）。c04 是 `conversations`/`messages` 唯一建表 owner，c06 通过本接口写入 kb_qa 会话并据此向 recent_tasks 写 source=「医疗知识库问答」条目（c06 自己的 spec 声明该写入），c05 恢复编排据 module 区分 aimed/kb_qa。

| mode | allow_pubmed | allow_upload | allow_kb | allow_current_doc | upload_required | clear_files_on_enter |
|---|---|---|---|---|---|---|
| general | ✓ | ✓ | ✓ | - | - | - |
| deep_reading | ✗(禁连 PubMed) | ✓ | ✗ | - | ✓ | - |
| trend_analysis | ✓ | ✗(无入口) | ✗ | - | - | ✓ |
| evidence_tracing | ✓ | ✗(无入口) | ✗ | - | - | ✓ |
| review_gen | ✓ | ✓ | ✗ | - | - | - |
| writing_assist | ✓ | ✓ | ✗ | ✓ | - | - |

`allow_kb`（医疗知识库）维按 §8.2 数据源列设：仅 general=✓，其余五模式（deep_reading/trend_analysis/evidence_tracing/review_gen/writing_assist）=✗，使「仅上传文件」「仅 PubMed」「PubMed+上传文件」对医疗知识库的排除可被声明式 policy 强制（院内制度/指南/SOP 等 KB 内容在禁用 KB 的模式下 MUST NOT 被检索注入或出现在引用，含合规面）。

模式切换规则（§8.3）：切到 trend_analysis/evidence_tracing 清空文件，其他保留；任何切换强制保留输入框内容（前端状态，后端切模式接口不清 draft）。发送按钮状态机（§8.5）由「模式 + 文件上传/解析状态 + 输入是否有效文本」三元组在后端计算返回 `can_send` + 置灰原因，前端只渲染，避免前后端两套状态机漂移。

**备选与取舍**：
- 备选 A：六模式各建独立服务/独立 prompt 链 —— 否决。模式差异主要在「数据源开关 + prompt 模板 + 文件约束」，独立服务造成大量重复 RAG 编排代码，违背 Simplicity First。
- 备选 B：模式约束硬编码 if/else —— 否决。状态机与数据源约束共 11+ 行规则（§8.5），硬编码难测试、易与 PRD 漂移；声明式 policy 表可对照 PRD 表逐行验证，并被「智能模式匹配」「无关问题处理」复用。
- 选 policy-driven 单服务 + 声明式映射：一处定义、前后端共识、可直接映射 §24.1 验收项。

### D2 智能模式匹配：关键词 → 推荐 Tab 高亮，不强制切换

**决策**：发送时（§8.11.1）触发模式识别，采用「规则关键词优先 + 轻量分类兜底」两级：
1. 一级：关键词词典匹配（§8.11.2 表），命中按 PRD 固定优先级排序：深度文献伴读 > 循证证据溯源 > 科研态势分析 > 智能综述生成 > 学术写作辅助。
2. 二级（仅一级未命中或需校验时）：调用 c03 模型做意图分类（私有化优先，避免为分类把用户问题发公网）。
- 推荐模式 ≠ 当前模式时，仅对推荐 Tab 高亮 + 引导文案，**不自动切换**（§8.11.3）。
- 命中 ≥2 模式 → 复合任务提示分步建议，不切多模式（§8.11.4）。
- general 模式非医学/科研问题 → 固定拒答文案；专业模式不符合能力 → 基于当前模式尽力答，不自动切 Skill（§8.11.5）。
- 命中按钮的实际发送仍用「用户当前选定模式」，识别结果只影响 UI 提示，保证「不强制切换」红线。

**备选与取舍**：
- 备选 A：纯 LLM 分类 —— 否决。每次发送多一次模型调用，延迟与成本高；且把用户原始问题送分类模型与脱敏前置冲突。规则词典零调用、可解释、可直接对照 §8.11.2 验收。
- 备选 B：纯关键词无兜底 —— 词典覆盖不足时（模式识别准确率指标 ≥85%，§20.3）召回不够。两级方案用规则覆盖高频、LLM 兜底长尾，且二级走私有化模型规避脱敏问题。
- 选两级 + 规则优先：满足 ≥85% 指标的同时控制成本与合规。

### D3 RAG 管线编排与 rerank / source compression

**决策**：单条编排管线，节点严格对应 §16.2，每步可独立替换/降级，整条管线作为一次 `agent_run`、每节点一条 `agent_step` 落库（§18 说明：agent_runs/steps/tool_calls 用于 AIMed/RAG 内部 AI 任务追踪），便于观测命中率与定位指标。

```
Query Rewrite(模型, 私有化优先) → 数据源选择(按 mode policy) → 权限过滤(tenant/kb/user/role/document_acl/chunk_acl)
  → BM25(全文检索) ∥ 向量检索(向量库) → 合并去重(按 chunk 指纹/PMID/doc+page) → rerank → source compression → 注入上下文 → 生成带引用答案
```

- BM25 与向量检索并行，召回 topK 各自取回后合并；去重键 = `pubmed_id` 或 `(document_id, page, paragraph_index)`，避免同源重复占引用位。
- rerank：默认走 reranker 模型（cross-encoder / 模型 Provider 的 rerank 能力，c03 路由）；公网/私有化不可用时降级为「BM25 分 + 向量分加权融合（RRF）」纯算法排序，保证离线可用。
- source compression：对 rerank 后 topN chunk 做抽取式压缩（句子级相关性截取）优先，必要时调模型做生成式摘要（私有化优先）；目标是控制注入上下文 token 且保留可定位的原文片段（压缩不得丢失 page/paragraph 定位元数据，否则引用无法回溯）。
- 注入上下文时为每个 chunk 附带稳定 `cite_ref`（序号 + 来源指针），生成阶段要求模型在关键结论后输出 `[n]` 角标，与 `citations` 表记录一一对应。
- 检索索引来源：c03 解析流水线只把 chunk + embedding 写库并在 `indexing_handoff` 发出「索引就绪」事件、不构建检索索引；本能力作为该事件的唯一检索侧消费方，收到后按 (document_id, version_id) 幂等构建/刷新 BM25 全文索引与向量倒排索引（新版本就绪替换旧版本），消除该 handoff 事件的孤儿态，BM25/向量检索即基于这些已就绪索引执行（对齐 c03 design D8 索引就绪 handoff 边界）。

**备选与取舍**：
- 备选 A：只向量检索 —— 否决。医学检索 PMID/术语/缩写精确匹配场景多，纯向量召回对专有名词弱；BM25+向量混合是 §16.2 明确要求。
- 备选 B：rerank 强依赖外部 reranker —— 否决。离线环境无公网 reranker 会断链；故设 RRF 算法兜底，rerank 仅作增强。
- 备选 C：compression 全用 LLM 生成式摘要 —— 风险是改写原文导致溯源失真。优先抽取式、保留原句与定位元数据，符合可溯源红线。

### D4 PubMed 在线/离线统一检索接口与降级

**决策**：定义统一 `PubMedSearchProvider` 接口（search / fetch_detail / import_by_id(PMC/DOI/URL)），两个实现：
- `OnlinePubMedProvider`：公网可用时真实调用 E-utilities（esearch/efetch），支持 PMC/DOI/URL 导入；URL/外部导入前查白名单或管理员授权（§16.1），未授权商业库直接拒绝、不默认抓取。
- `OfflinePubMedProvider`：读离线 PubMed 缓存/预置演示数据（由部署预置 / c06 导入沉淀），同接口返回，完成离线闭环（§24.1「科研态势分析和循证证据溯源支持 PubMed 或离线 PubMed 缓存」）。
- 选择策略：启动探测 + 运行时熔断。公网 PubMed 调用前必须先经 c09 redaction-gateway 脱敏门禁（PHI/PII 识别与脱敏唯一 owner=c09，本 phase 为消费方；查询词可能含 PHI）；脱敏失败/置信度不足/识别服务不可用 → 禁止调公网，自动降级到离线缓存（而非私有化模型，因为这是数据源不是生成模型）。在线调用超时/失败 → 熔断降级离线，并在答案生成过程提示数据来源。
- 检索结果统一归一为内部 `RetrievedSource`（含 pubmed_id/doi/title/journal/year/url/abstract/source_type），与上传文件/知识库 chunk 同构后进入 D3 合并去重。

**备选与取舍**：
- 备选 A：在线/离线两套独立调用散落在 RAG 各处 —— 否决。降级判断分散、易漏脱敏前置。统一接口把「公网→脱敏→在线，否则离线」收敛到一处，单点保证红线。
- 备选 B：离线时报错中断 —— 违背离线优先与 §24.1。必须无缝降级。
- 选 Provider 接口 + 熔断降级：对 RAG 透明，离线可演示，脱敏前置单点把控。

### D5 引用溯源数据结构与定位（页码/段落/chunk）

**决策**：本 phase 新建 §18 `citations` 表（建表 owner=c04，见 D8）承载引用，每条引用绑定 `message_id` 与来源指针，按 `source_type` 三态定位：

| source_type | 定位指针字段（复用 §16.3 chunk 元数据） | 点击行为(§8.9) |
|---|---|---|
| pubmed | pubmed_id / doi / source_url | 打开 PubMed 文章详情 |
| upload | document_id + page + paragraph_index + section | 打开文档预览并定位页码/段落 |
| kb | kb_id + document_id + chunk_id | 打开知识库文档并定位 chunk |

- 引用序号 `[n]` 在生成时由 cite_ref 注入，答案末尾按 §8.8 渲染结构化参考资料（PMID/Title/Journal/Year 或 文档名+页+段）。
- 页码/段落定位精度依赖 c03 视觉解析的 page/paragraph_index（指标：引用定位页码误差 ≤1 页，§20.3）；chunk 定位失败时按 §8.9 降级「已打开来源文档，请手动查看相关段落」。
- 引用异常分支（§8.9）在点击定位 API 内统一裁决：权限不足（实时复核 tenant/acl）→「该引用源暂时不可用」；原文已删除（document 软删/version 失效）→「该引用源已删除」；外链失效（在线探测失败）→「该引用源暂时不可用」。引用点击事件写 `audit_logs`。

**备选与取舍**：
- 备选 A：把引用塞进 message 正文 JSON、不入 citations 表 —— 否决。无法独立做权限实时复核、可点击率/定位成功率统计与审计，违背可观测与审计红线。
- 备选 B：定位只记 chunk_id —— 否决。upload 来源需 page/paragraph 才能在 ONLYOFFICE/预览里跳转，且 §20.3 有「页码定位成功率/误差」指标，必须显式存 page/paragraph_index。
- 选 citations 表 + 按 source_type 分支定位指针：满足三类来源跳转、异常降级、审计与指标统计。

### D6 答案「生成在线 Word / 保存为」经 c02 Bridge 落地

**决策**：答案操作栏（§8.10）右侧「生成在线 Word」走 **c01 文档中心服务端创建服务**（owner=c01）：AIMed 将答案（含引用结构）渲染为 docx 模型 → 调 c01 文档中心服务端创建服务把 docx 落为 `documents` / `document_versions`、写入文档中心 `我的文档中心/应用/AIMed 学术助手/...`（权限按当前用户在该应用文档空间的创建能力校验）→ 返回 `document_id`，再经 c02 打开链路让前端在 ONLYOFFICE 打开该 `document_id` → 作为打开入口引用 c05 拥有的「文档打开后默认展示医疗 AI 面板」触发并传入该 `document_id`（默认展示触发与面板本体均归 c05，本能力不自建/重定义该触发）。本能力 MUST NOT 依赖 c02 编辑器内 `createNewDocument(content, templateId)` 的服务端新建变体、亦 MUST NOT 自发明服务端新建/落版契约（c02 `createNewDocument` 仍仅为编辑器内新建、可编辑权限，与服务端生成区分）。「保存为」（§8.10.2）按保存范围（当前回答/对话/全部）× 格式（在线文档/Word/PDF/Markdown）组合：在线文档/Word 走 c01 文档中心服务端创建服务（同上链路）；PDF/Markdown 走纯文本/渲染导出。命名规则 `yyyymmdd_对话名称`。生成的 docx 携带医疗免责声明。

**解析/索引触发（§10.6 闭环）**：AIMed「生成在线 Word / 保存为在线文档·Word」经 c01 文档中心服务端创建服务在文档中心新建 `documents` / `document_versions`，该首版入库 MUST 由 **c01 文档中心创建入口产生 §10.6 `upload_success` 事件**（产生方=c01、消费方=c03，与 AIMed 本地上传文件复用同一 `upload_success` 入库事件、对齐 c01「`upload_success` 唯一产生方=c01」契约，比照 c08 模板首版归入首版入库事件的同构口径），由 c03 消费 `upload_success` 异步解析/索引，使该生成文档可被后续 RAG 检索。c04 本身不直接产生 document_events、不另立第七类事件，仅复用 c01 创建服务触发 `upload_success`；该事件唯一产生方为 c01 文档中心创建入口。纯服务端净新建不经 ONLYOFFICE 保存回调，因此不走 `save_new_version`（save_new_version 仅由 c02 编辑器保存回调对已打开文档产生新版本时产生），消除「服务端净新建首版无确定产生方」的孤儿态。

**备选与取舍**：
- 备选 A：AIMed 自建 docx 写入与版本逻辑 —— 否决。文档创建/落版/upload_success 已由 c01 文档中心统一提供，重复实现违背 Surgical Changes 且制造两套入库事件。
- 备选 B：经 c02 `createNewDocument` 服务端新建并产生 `save_new_version` —— 否决。`createNewDocument` 是 c02 编辑器内新建变体（需对目标文档「可编辑」、签名带 templateId、经 ONLYOFFICE 会话），纯服务端净生成无编辑器会话、不经保存回调，套用会造成权限模型/签名/事件产生方不一致（F8/F9）；且 c04 不应单方面发明 c02 owner 未提供的服务端新建契约。
- 选 c01 文档中心服务端创建服务 + `upload_success`：文档入库与首版索引事件单一产生方=c01，与本地上传/模板首版同构，链路无孤儿、可被 RAG 检索。

### D7 命中率与引用定位指标的埋点

**决策**：把 §20.3 中本期归属的指标显式埋点，复用 `agent_runs`/`agent_steps`（管线观测）+ `feedbacks`（赞/踩、踩原因 §8.10.5）+ `eval_cases`/`eval_results`（内置验收集）：
- 模式识别准确率 ≥85%：识别结果 vs 实际发送模式/标注，记 agent_step。
- PubMed RAG Hit@5 ≥80%：检索 top5 是否含命中源，eval 集评测。
- 引用可点击率 ≥95% / 引用源定位成功率 ≥90% / 页码误差 ≤1 页：定位 API 成功率与误差，记 audit/eval。
踩反馈原因（§8.10.5 固定 7 项：不准确/引用错误/没有回答问题/格式不好/内容太少/内容太长/其他）入 `feedbacks`，闭环优化 RAG 与 prompt。

**备选与取舍**：
- 备选 A：另建一套指标表 —— 否决。§18 已有 agent_steps/feedbacks/eval_*，复用即可，避免新表（eval_cases/eval_results 由 c09 建表，本期消费）。
- 选复用现有表埋点：满足可观测，零新增表。

### D8 §18 表建表 owner 与 c09 redaction-gateway 的消费接缝

**决策（建表 owner）**：本 phase（c04）是以下 §18 表的唯一建表 owner，迁移第 0 步显式 CREATE：`conversations` / `messages` / `citations` / `agent_runs` / `agent_steps` / `tool_calls` / `feedbacks`。`agent_checkpoints` V1.0 不建（仅为 V1.1 长任务断点续跑预留，§18 注记）。本 phase 不建任何上游表，仅消费/写入其 owner 所建表：

**决策（feedbacks 多来源泛化对外契约）**：`feedbacks` 表 owner=c04，泛化为可承载多来源反馈的单一表（对齐 §18 单一 feedbacks 表口径），对外契约列固定为：`feedback_id`（主键）、`tenant_id`、`user_id`、`subject_type`（枚举 `message` / `translation_job`，区分反馈对象来源）、`subject_id`（承载 `message_id` 或 `translation_jobs.job_id`，替代原 `message_id` 非空外键硬约束）、`rating`（赞/踩或翻译质量评分）、`reason`（可扩展枚举/文本，承载 AIMed 踩原因枚举〔§8.10.5 原文 7 项：不准确/引用错误/没有回答问题/格式不好/内容太少/内容太长/其他〕∪ 翻译质量反馈维度）、`comment`、`created_at`。c04 自身写入按 `subject_type=message`、`subject_id=message_id`；c07 §17.6 翻译质量反馈作为写入侧消费方按 `subject_type=translation_job`、`subject_id=translation_jobs.job_id` 写入（c07 仅写入、不建表、不 ALTER，落库去向=本 owner 所建 feedbacks）。

| 表 | owner | c04 用途 |
|---|---|---|
| documents / document_versions | c01 | 消费（保存答案落文档中心经 c02 Bridge） |
| document_chunks / embeddings / document_parse_jobs / document_visual_parse_results | c03 | 消费（检索与溯源定位） |
| knowledge_bases / kb_documents | c06 | 消费（RAG 检索已索引内容） |
| recent_tasks | c01 | 写入（AIMed 保存内容进入最近任务，展示/恢复归 c05） |
| audit_logs | c01 | 写入（审计事件；建表唯一 owner=c01，c04 仅写入不建表） |
| privacy_redaction_events | c09 | 消费门禁判定，不写该表（脱敏命中/策略留痕由 c09 redaction-gateway 在公网出口单一写入；建表 owner=c09，c04 不建不写） |
| privacy_detection_rules | c09 | 消费（脱敏判定规则） |
| eval_cases / eval_results | c09 | 消费（内置验收集评测埋点） |

**决策（脱敏门禁）**：PHI/PII 识别与脱敏引擎 redaction-gateway 唯一 owner=c09，是被 c03–c07 前置消费的横切能力。c04 在「公网模型 / 公网 PubMed」出口消费该门禁接缝（契约：输入内容 → redaction-gateway 识别+脱敏 → 放行脱敏后内容 / 拒绝公网），不自行实现 PHI/PII 识别脱敏。phase 顺序：redaction-gateway 需早于公网放开；本期默认公网关闭、私有化模型与离线 PubMed 缓存优先，端到端公网脱敏验收随 c09（phase9）落地完成（符合 §16.4/§24.9 禁用公网仍经私有化/离线闭环）。门禁判定不可用时按「识别服务不可用」保守降级（禁用公网、走私有化/离线）。

**决策（草稿/辅助建议标记与上传时 PHI 消费）**：除「公网模型出口脱敏门禁」外，c04 还消费 c09 security-compliance 的两条横切契约：(1) 草稿/辅助建议标记——§24.7 第二项要求所有 AI 生成内容标记为草稿/辅助建议，owner=c09，c04 作为 AIMed 内容产出方为消费方，AIMed 答案除医疗免责声明外随内容标记草稿/辅助建议，端到端随 c09 验收；(2) 上传时 PHI/PII 识别——§19.4 规定识别发生在「上传时」与「调用模型时」两个时点，AIMed 文件上传入口属上传时点，文件在持久化入库或送 c03 解析前消费 c09 上传时 PHI/PII 识别契约（owner=c09），按策略「识别并提示/脱敏后送模型/阻止上传」处理，c04 仅消费判定、不自行实现识别脱敏。

**备选与取舍**：
- 备选 A：沿用旧文「这些表由上游 phase 建好」「脱敏由 c03 提供」—— 否决。会造成 conversations/messages/citations/agent_*/feedbacks 无人建表的孤儿表（F42），且与 c09 唯一 owner redaction-gateway 冲突。
- 选 c04 唯一建表 + 消费 c09 门禁接缝：建表归属唯一，脱敏横切能力单点 owner。

## Risks / Trade-offs

- [脱敏前置阻断公网检索导致召回下降] → 查询词脱敏后语义损失时召回降低：脱敏对检索关键词采用「保守替换/占位」而非删除，并在脱敏不可用时降级离线缓存而非直接失败；离线缓存命中不足时在答案过程明示数据来源受限。
- [离线缓存覆盖窄，Hit@5 不达 80%] → 离线 PubMed 演示数据规模有限：本期与 c06/部署脚本约定预置足够覆盖验收测试集（§20.4）的演示语料；指标在「离线模式」与「在线模式」分别度量，验收以演示集为准。
- [生成式压缩/摘要改写原文造成溯源失真] → compression 优先抽取式、保留原句与 page/paragraph 元数据；生成式仅用于超长上下文且不参与引用定位文本。
- [LLM 不按要求输出 `[n]` 角标 → 引用可点击率掉] → prompt 强约束 + 后处理校验：生成后比对答案中角标与 citations 集合，缺失/越界角标触发补全或重生成；可点击率指标监控。
- [rerank/reranker 外部依赖不可用] → RRF 算法兜底已设计（D3），离线零外部依赖仍可排序，仅排序质量略降。
- [引用权限实时复核增加点击延迟] → 复核走轻量 ACL 查询（tenant/kb/acl 索引），不重新跑检索；失效来源结果可短缓存。
- [模式识别误导用户] → 仅高亮不强制切换（红线），错误识别不改变实际数据源；复合任务给分步建议而非自动多模式，避免越权检索。
- [PubMed E-utilities 速率限制/不稳定] → 在线 Provider 加超时熔断 + 退避，触发即降级离线；不把外部抖动暴露为答案失败。

## Migration Plan

本期为净新增、无破坏性变更（proposal「本期不含破坏性变更」），无既有 spec 行为修改，故无数据迁移。部署/上线顺序：

1. 前置校验：确认 c01 文档中心服务端创建服务（生成在线 Word/在线文档落库并产 §10.6 upload_success）、c01 文档中心上传服务、c02 打开链路（ONLYOFFICE 打开 document_id）、c03 模型路由（含 rerank 能力）、c09 redaction-gateway（公网脱敏门禁，本期默认公网关闭、私有化优先，详见 D8）、c01 RBAC/对象存储就绪；本 phase 在迁移第 0 步建表 `conversations` / `messages` / `citations` / `agent_runs` / `agent_steps` / `tool_calls` / `feedbacks`（agent_checkpoints 本期不建），消费表 `document_chunks` / `embeddings`（c03 建）、`recent_tasks`（c01 建）、`knowledge_bases` / `kb_documents`（c06 建）须由其 owner 先就绪。
2. 配置 PubMed Provider：默认离线模式（公网未知时安全默认），公网环境通过开关启用 OnlinePubMedProvider 并配置白名单。
3. 预置离线 PubMed 演示数据与内置验收测试集（与 c06/部署脚本对齐 §20.4）。
4. 灰度：先开 general/trend/evidence（不依赖上传），再开依赖上传解析的 deep_reading/review/writing。
5. 回滚策略：AIMed 入口可整模块下线（门户隐藏入口），不影响 c01/c02 文档闭环；PubMed 在线异常可一键切回离线模式；rerank 异常自动 RRF 降级，无需回滚发布。

## Open Questions

1. 离线 PubMed 缓存的预置数据由「部署脚本预置」还是「c06 知识库导入沉淀」为权威来源？两处来源的去重与版本以谁为准，需与 c06 对齐。
2. 「保存为 - 全部对话」的导出边界：是否含已删除消息、跨会话聚合命名规则，PRD 未明确，暂按「当前对话名 + 仅未删除消息」实现，待确认。
3. 学术写作辅助内短文本翻译（§8.12）与 c07 术语库一致性：本期直接走模型生成，是否需复用 c07 `term_bases`/`terms` 以保证术语一致（§20.3 术语一致性 ≥95% 是翻译模块指标），暂不复用，待 c07 确认是否下沉为公共能力。
4. 引用「外链失效」的在线探测频率与缓存策略（每次点击实时探测 vs 周期巡检）需权衡延迟与准确性，暂定点击时实时 + 短缓存。
5. source compression 的 token 预算上限与不同 mode（如综述生成需更长上下文）的差异化配置，待结合所选模型上下文窗口定标。
