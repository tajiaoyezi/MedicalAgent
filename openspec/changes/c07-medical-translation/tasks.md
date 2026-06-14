## 1. 数据模型与依赖接入

- [ ] 1.1 建 `translation_jobs` 表（本 change 为唯一建表 owner，对齐 §18；D1 字段：`job_id`（主键，与 `translation_segments.job_id` 外键、`recent_tasks.ref_id`、c05 回源口径同名，不用 `id`）/ `tenant_id` / `created_by` / `source`(`local`/`my_docs`/`team_docs`/`onlyoffice_current`) / `source_document_id` / `source_version_id` / `file_format` / `file_size` / `engine` / `lang_from` / `lang_to` / `term_base_id` / `corpus_id` / `output_mode`(`translation_only`/`side_by_side`/`paragraph_interleaved`) / `layout_style`(译文排版方式，独立设置项，如 `compact`/`loose`/`preserve_layout`；`preserve_layout`=保持原版式风格，与下方布尔列 `keep_original` 命名不碰撞) / `output_format`(输出/下载格式，如 `docx`/`pdf`) / `keep_original`(§13.7「是否保留原文」内容布尔开关) / `keep_image` / `keep_table` / `bilingual` / `status` / `progress` / `failure_code` / `failure_reason` / `result_document_id` / `result_version_id` / 时间戳），并建 `tenant_id` + `created_by` 复合索引；验证：迁移可建表回滚，按 tenant 查询命中索引，`layout_style`/`output_format` 可写入回读（对应 spec「翻译设置与三种输出模式」）
- [ ] 1.2 建 `translation_segments` 表（D1 字段：`id` / `job_id` / `seq` / `page_no` / `paragraph_index` / `bbox` / `heading_level` / `block_type`(`paragraph`/`table_cell`/`header`/`footer`/`caption`/`formula`/`footnote`) / `source_text` / `target_text` / `term_hits` / `status` / `confidence`），外键 `job_id`，建 `job_id` + `seq` 索引；验证：可按 job 分页拉取有序 segments
- [ ] 1.3 建术语库 / 语料库表 `term_bases` / `terms`（源词 / 目标词 / 领域 / 优先级）与 `corpora`（双语句对 / 风格参考）（本 change 为唯一建表 owner，对齐 §18，含 `tenant_id` 隔离列与启停 / 绑定字段），并灌入演示术语库与演示语料库种子数据；验证：可按 `term_base_id` / `corpus_id` 读取条目，演示库非空
- [ ] 1.4 接入 c03 `model_routes`「医学翻译」「术语抽取」用途路由与 `visual_parse_providers` 路由的只读客户端（不自建 provider 配置）；验证：能取到公网 / 私有化两类入口、优先级与 fallback 列表
- [ ] 1.5 在公网出口预留脱敏门禁接缝并接入 c09 redaction-gateway（PHI/PII 识别脱敏引擎与 `privacy_detection_rules` / `privacy_redaction_events` 唯一 owner=c09，c01 不实现；c07 仅消费判定，事件由门禁写入），以及 `audit_logs`（owner=c01）写入封装；验证：门禁接缝可对一段含 PHI 文本经 c09 判定后产出 redaction 事件 + 一条审计；c09 真实判定接入与端到端验收随 c09（phase9）落地完成，本期默认门禁不可用时拒绝公网

## 2. 上传来源、格式与限制校验

- [ ] 2.1 实现六类上传来源接入：拖入文件 / 点击 + 上传 / 选择本地文档 / 我的文档中心 / 团队文档中心 / 从当前 ONLYOFFICE 文档（取 `document_id`）；验证：六类来源均能产出一个待创建任务的来源描述（对应「多入口与上传来源发起翻译」）
- [ ] 2.2 对文档中心来源与当前 ONLYOFFICE 文档来源做 `tenant_id` + 文档级 ACL 过滤，命中才允许加入；验证：有权文档加入成功（「从文档中心右键发起翻译需通过 ACL 过滤」场景）
- [ ] 2.3 越权来源硬拒绝：无权访问的文档不创建任务、提示无权访问并写 `audit_logs`；验证：越权来源返回拒绝且无任务生成（「越权来源被拒绝」场景）
- [ ] 2.4 实现支持格式校验：Word(doc/docx) / PPT(ppt/pptx) / 文本型 PDF / 扫描 PDF / 图片(png/jpg) / OFD（转换后）通过，其余格式拒绝；验证：受支持格式全部接受（「受支持格式与限制内文件通过校验」场景）
- [ ] 2.5 实现上传限制校验：单次最多 10 个文档、单文档 ≤ 50MB、加密文档拒绝，任一不满足返回明确原因不静默丢弃；验证：第 11 个文档 / 60MB 文档 / 加密文档分别被拒并给出对应提示（三场景）
- [ ] 2.6 实现 OFD 来源消费 c02/c03 的转 PDF / 文本抽取结果纳入流程，转换失败返回明确失败原因；验证：OFD 转换成功后进入流程、转换失败给原因（「OFD 转换后支持」场景，不实现转换器本身）
- [ ] 2.7 上传时 PHI/PII 识别（消费 c09 上传闸）：翻译文件在持久化入库或送模型前前置消费 c09 security-compliance「上传时 PHI / PII 识别与『阻止上传』策略执行」契约（owner=c09，c07 不自实现识别脱敏），按策略「识别并提示 / 脱敏后送模型 / 阻止上传」处理，命中且策略=阻止上传时拒绝入库；命中处理由 c09 写 `privacy_redaction_events` 回填 `audit_logs.id`，被阻止上传写 `result=失败`、`failure_reason` 非空的审计；验证：含 PHI 翻译文件在「阻止上传」策略下被拒入库且不写原文存储（对应 spec「翻译文件上传命中 PHI 按阻止上传策略被拒入库」场景）；c09 真实判定接入与端到端验收随 c09（phase9）落地

## 3. 待翻译列表与翻译设置

- [ ] 3.1 实现待翻译列表展示字段（文档名称 / 格式 / 大小 / 上传状态 / 解析状态 / 翻译状态 / 操作），其中三列状态由 `translation_jobs.status` + `progress` 按固定规则派生映射（如 `queued`→已上传/待解析/待翻译，`parsing`→已上传/解析中/待翻译，`translating`→已上传/已解析/翻译中 N%），仅展示当前用户 `tenant_id` 下有权文档；验证：列表展示全部规范字段且按权限隔离（「列表展示全部规范字段」场景），上传完成但解析未开始时三列分别展示 已上传/待解析/待翻译（「三列状态独立派生展示」场景）
- [ ] 3.2 实现列表三类操作：添加文档、移除文档（释放名额）、预览原文（不改原文）；验证：移除后名额释放、预览不修改原文内容（「移除待翻译文档」「预览原文」场景）
- [ ] 3.3 实现翻译设置项：翻译引擎、语言方向、译文排版方式(`layout_style`)、术语库、语料库、输出格式(`output_format`)、是否保留原文 / 图片 / 表格 / 双语对照；验证：设置项可保存到 `translation_jobs` 对应字段，`layout_style` 与 `output_format` 提交后可回读原值（「译文排版方式与输出格式可保存并回读」场景）
- [ ] 3.4 实现语言方向选择（至少含 中文简体→英语 / 英语→中文简体 / 中文简体→中文繁体 / 日语→中文简体）；验证：选「英语→中文简体」任务译文为中文简体（「选择语言方向」场景）
- [ ] 3.5 实现三种输出模式参数（`translation_only` / `side_by_side` / `paragraph_interleaved`）与保留开关落库；验证：三种模式各提交一次任务记录正确（覆盖输出模式三场景的参数侧）

## 4. 异步任务状态机与队列

- [ ] 4.1 实现作业表驱动的后台 worker 与阶段化状态机（D2）：`queued`→`parsing`→`translating`→`laying_out`→`succeeded`/`failed`/`cancelled`，状态变更落 `translation_jobs` 并写 `audit_logs`；验证：合法文档依次经历各状态到达翻译成功（「任务按状态机正常完成」场景）
- [ ] 4.2 实现翻译中进度暴露：`progress` = 已完成 segment 数 / 总 segment 数实时计算，对外展示「翻译中 N%」；验证：翻译中阶段可读到递增百分比（「翻译中展示进度」场景）
- [ ] 4.3 实现协作式取消：worker 在阶段边界检查取消标志，对 `queued`/`parsing`/`translating` 任务取消转 `cancelled` 并停止后续，写 `audit_logs`；验证：取消后任务停在 `cancelled` 且有审计（「用户取消任务」场景）
- [ ] 4.4 实现阶段失败转 `failed` 并记 `failure_code` / `failure_reason`，单 segment 失败不阻塞整篇、失败段比例超阈值才整篇失败；验证：解析 / 翻译 / 排版任一阶段不可恢复错误转翻译失败（「阶段失败转入翻译失败」场景）
- [ ] 4.5 实现队列并发限制与单租户配额、segments 流式处理（不整篇载入内存）；验证：并发提交多个大文档时 POC 单机不无响应、进度可见可取消

## 5. PHI/PII 脱敏门禁与公网 / 私有化路由

- [ ] 5.1 在公网翻译 / 公网视觉解析调用边界挂载脱敏门禁（D5）：调用前执行 PHI/PII 识别脱敏，成功且置信度达标才以脱敏后内容调公网，并写 `privacy_redaction_events`；验证：脱敏通过后公网调用成功并落事件（「脱敏通过后调用公网翻译」场景）
- [ ] 5.2 实现硬阻断：识别失败 / 脱敏置信度不足时禁止任何公网调用、提示切私有化、不向公网发送任何待翻译内容；验证：识别失败时无任何公网出网请求（「识别失败禁止公网调用」场景）
- [ ] 5.3 实现公网不可用按 `model_routes` / `visual_parse_providers` fallback 切私有化继续，私有化也不可用则任务失败记原因，全部切换写 `audit_logs`；验证：公网识别 / 视觉解析不可用时切私有化跑通，无私有化则失败（「公网识别服务不可用切私有化」场景）
- [ ] 5.4 真实外接连通性测试：对接 c03 配置一个真实公网翻译 Provider 与真实公网视觉解析 Provider，跑通脱敏→公网翻译→落库全链；验证：真实公网调用返回译文且 redaction / 审计齐全
- [ ] 5.5 离线 / 私有化降级演示：在断公网环境仅配私有化翻译模型与私有化视觉解析，跑通整篇翻译闭环；验证：无公网下任务可达翻译成功，无任何公网出网

## 6. 版式还原分流与三种输出生成

- [ ] 6.0 实现 `.pdf` 文本层探测与文本型 / 扫描型分流路由（D3）：对每个 `.pdf` 探测可提取文本层覆盖率，达阈值走 c03 `document-parsing` 文本管线、否则走 c03 `visual-parsing-service` 视觉解析管线；混合页按页分流，无法可靠判定时整篇兜底走视觉解析；验证：文本型 `.pdf` 与扫描 `.pdf` 分别被正确路由、混合页 / 判定失败时兜底走视觉解析不产出空译文（「文本型 PDF 与扫描件按文本层正确分流」「混合页或判定失败时的兜底」场景）
- [ ] 6.1 实现 Word/PPT(OOXML) 就地替换管线（D3）：保留标题层级、表格、图片、页眉页脚、页码，尽量保留公式 / 角标 / 参考文献，无法安全替换的块降级「保留原文 + 旁注译文」；验证：含标题 / 表格 / 图片 / 页眉页脚的 docx 译文保留对应结构（「Word 译文保留结构元素」场景）
- [ ] 6.2 实现文本型 PDF 管线：消费 c03 `document-parsing` 取页 / 段 / 表格 / 图片位置，做页级预览、段落级翻译、表格识别、图片位置保留并生成可下载译文，保留页 / 段对应关系；验证：含表格图片的文本型 PDF 段落级翻译并保留图片位置生成可下载文件、页级预览按页对应（两场景）
- [ ] 6.3 实现扫描 PDF / 图片管线：消费 c03 `visual-parsing-service` 与 `document_visual_parse_results`（文本 / 页码 / 坐标 / 表格 / 图片位置 / 置信度 / chunk 定位），按坐标回填译文、支持左右对照预览与可下载译文；验证：扫描 PDF 经视觉解析后翻译还原、译文段可按页码坐标定位回原文（「扫描 PDF 经视觉解析后翻译」「视觉解析结果可溯源定位」场景）
- [ ] 6.4 实现视觉解析失败处理：无法识别时任务转 `failed` 给「无法完成文档视觉解析」类原因并允许重提；验证：图像不可读时转翻译失败且可重新提交（「视觉解析失败转入翻译失败」场景）
- [ ] 6.5 实现基于 segments 原 / 译文配对的统一输出生成器，按 `output_mode` 渲染三种布局（仅译文 / 左右对照 / 逐段对照）并受保留开关控制（D7）；验证：三种输出模式各生成正确布局结果文件（输出模式仅译文 / 左右对照 / 逐段对照三场景）
- [ ] 6.6 版式结构保留率验收：对 §24.4 Word/PPT 验收集执行翻译并比对结构元素（标题 / 表格 / 图片 / 页眉页脚 / 页码加权），定稿保留率度量口径；验证：版式结构保留率 ≥ 90%、视觉解析页码定位成功率 ≥ 90% 且表格结构识别成功率 ≥ 85%（「版式结构保留率达标」场景）

## 7. 术语一致性与术语库 / 语料库

- [ ] 7.1 实现译前术语锁定（D4）：翻译前对全任务 segments 做术语匹配，命中 `terms` 以受保护占位注入提示词约束译法，命中写 `segment.term_hits`；验证：选定术语库后某术语全任务采用术语库一致译法（「同一任务术语保持一致」场景）
- [ ] 7.2 实现任务级译法表：同一源术语整篇任务维护唯一目标译法，语料库作 few-shot 软参考注入；验证：长文档中同词同译、风格参考生效
- [ ] 7.3 实现术语命中溯源：译文可展示某译法来源于哪个术语库 / 术语条目；验证：核对术语时返回来源术语库与条目（「术语命中可溯源」场景）
- [ ] 7.4 实现未配置 / 未选术语库时降级使用演示术语库与演示语料库，私有化模型同样走占位锁定；验证：无外配术语库时 POC 仍可演示一致译法
- [ ] 7.5 术语一致性验收：对 §24.4 验收集以参考译文比对术语、定稿一致性分母口径；验证：术语一致性 ≥ 95%，未达阈值任务详情告警并提供重译入口（「术语一致性达标」场景）

## 8. 翻译历史、落库与 ONLYOFFICE / 最近任务联动

- [ ] 8.1 实现翻译历史页与操作：预览 / 下载 / 删除 / 重新翻译 / 打开到 ONLYOFFICE / 查看原文 / 查看任务详情 / 查看失败原因，仅展示 `tenant_id` + ACL 命中任务；验证：成功任务可预览下载在线打开、历史按权限隔离（「翻译成功后可预览下载与在线打开」「翻译历史按权限隔离」场景）
- [ ] 8.1a 实现翻译历史「删除」操作（对齐 §6.7）：点击删除 MUST 二次确认，确认后删除该翻译任务记录、按 `(ref_type=translation_job, ref_id=job_id)` 同步更新 / 移除对应 `recent_tasks` 条目、默认保留已生成译文文件（`result_document_id` / `result_version_id` 对应版本），仅当用户显式选择「同时删除关联文档」时才删除关联译文文档，并写一条 `audit_logs`；验证：删除需二次确认、默认不删译文文件、同步 recent_tasks、写审计（对应 spec「删除翻译历史任务（二次确认、不删译文文件、同步最近任务）」场景）
- [ ] 8.2 实现任务详情与原文查看：展示全链路状态、设置参数与来源原文；验证：点详情 / 查看原文返回完整链路与来源（「查看任务详情与原文」场景）
- [ ] 8.3 实现译文落库（D6）：成功后经 c02 `save-callback-versioning` 生成新 `document_version`(`source=translation`) 不覆盖原文，回填 `result_document_id` / `result_version_id`，写入文档中心前需用户确认并写 `audit_logs`；验证：译文生成副本而非覆盖、写入前确认（「译文生成副本而非覆盖原文」「写入文档中心前用户确认」场景）
- [ ] 8.4 实现「打开到 ONLYOFFICE」：按 `result_version_id` 用 c02 编辑器打开译文新版本；验证：打开的是译文新版本、原文版本不变，ONLYOFFICE 保存回调成功率 ≥ 99%
- [ ] 8.4a 实现译文成功落库后产生 `document_events(event_type=translation_done)`（c07 为 PRD §10.6「翻译完成」触发源唯一产生方，c01 把 `translation_done` 指派给 c07）：任务到达翻译成功并经 c02 生成译文新版本（`source=translation`）后，产出一条携带 c01 契约稳定字段 `(event_type=translation_done, document_id=result_document_id, version_id=result_version_id, tenant_id, occurred_at, payload)` 的事件，供 c03 作为 `document_events` 全部 6 类触发源的唯一重解析 / 索引消费方消费触发重解析；c03 解析 / 索引就绪后另发「索引就绪」事件，再由 c04 检索侧构建索引、c06 知识库收尾侧消费该「索引就绪」事件刷新 `index_status` 与文档计数（c06 消费的是 c03 下游「索引就绪」事件而非 `translation_done`，c06 不直接消费 `document_events`）；与 `document_versions.source=translation` 版本来源标记区分、不混用，且不被 c02 保存回调的 `save_new_version` 替代；验证：译文落库后产出 1 条 `event_type=translation_done` 的 `document_events` 且字段对齐 c01 §10.6 契约（对应 spec「译文成功落库产生 translation_done 事件」场景）
- [ ] 8.5 实现最近任务联动（写入侧 owner=c07，对齐 c04 7.5 范式）：任务创建及到达翻译成功时向 c01 所建 `recent_tasks` upsert 一条 `source=医学翻译`（§6.4 规范值）、`ref_type=translation_job`、`ref_id=translation_jobs.job_id`、按 `tenant_id` / `user_id` 隔离、以 `(ref_type, ref_id)` 为幂等键的记录（展示 / 恢复编排归 c05，本能力仅写入条目并提供按 job_id 回源接口）；验证：任务成功后 recent_tasks 落一条 source=医学翻译、ref_id 指向 job_id 的记录，重复写入幂等不重复插入，c05 可据 ref_id 恢复到对应翻译任务（对应 spec「翻译任务联动最近任务」的「翻译任务创建 / 完成写入最近任务」「最近任务记录按租户隔离且幂等」「从最近任务恢复进入翻译任务」场景）
- [ ] 8.6 真实外接连通性测试：对接 c02 真实 ONLYOFFICE 实例，完成译文落库→保存回调→打开编辑全链；验证：真实 ONLYOFFICE 下保存回调成功、译文可在线打开
- [ ] 8.7 离线 / 私有化降级演示：在无公网（私有化 ONLYOFFICE + 私有化模型 + 私有化解析）环境跑通译文落库与打开；验证：离线环境译文可落库并打开到 ONLYOFFICE

## 9. 当前 ONLYOFFICE 文档发起翻译

- [ ] 9.1 实现「按 c05 面板侧传入的 `document_id` 建任务」服务接口（面板侧经 Bridge 取数与打开翻译页归 c05，c07 不重复实现取数 / 跳转）：据传入 `document_id` 创建 `translation_jobs`(`source=onlyoffice_current`) → 加入待翻译列表 → 返回医学翻译页路由目标；验证：传入 `document_id` 后创建任务并返回翻译页路由（「接收传入 document_id 创建任务」场景）
- [ ] 9.2 实现当前文档 `tenant_id` + 文档级 ACL 校验，无权拒绝并写 `audit_logs`；验证：当前文档 ACL 未授权时拒绝创建并提示无权访问（「当前文档无权限被拒绝」场景）
- [ ] 9.3 实现「接收 AIMed 答案栏整篇翻译请求建 translation_job」服务接口（按钮挂载与 §8.12 选区 / 短文本 vs 整篇分流由 c04 答案操作栏拥有，c07 仅接收整篇请求建任务）：据 c04 传入的文档 / 答案内容引用与设置创建 `translation_jobs` 并加入待翻译列表、返回翻译页路由目标，建任务前校验 `tenant_id` + 文档级 ACL；验证：c04 传入整篇翻译请求后 c07 建任务成功（对应 spec「接收 AIMed 答案栏整篇翻译请求建 translation_job」场景）

## 10. 失败原因、重译、免责声明与任务成功率

- [ ] 10.1 实现失败原因展示与持久化：文件加密 / 损坏 / 无法解析 / 视觉解析失败 / 版式无法重建给明确原因，写 `translation_jobs` 与 `audit_logs`；验证：损坏文件转失败展示「文件损坏 / 无法解析」类原因（「损坏文件展示明确失败原因」场景）
- [ ] 10.2 实现失败任务重新翻译：复用原来源文档与设置重新进入 `queued` 状态机；验证：失败任务点重新翻译复用原设置重排队（「失败任务重新翻译」场景）
- [ ] 10.3 实现译文医疗免责声明：结果页标识译文为草稿 / 辅助产物需人工确认，下载 / 导出译文均携带免责声明；验证：结果页与导出文件均含免责声明（「翻译结果页展示免责声明」「导出译文携带免责声明」场景）
- [ ] 10.3a 实现高风险译文人工确认前置消费（消费 c05 ai-writeback-confirmation 高风险确认链路，以 `translation_job` 为确认 subject，与 c04 AIMed 答案 / c06 kb_qa 答案复用同一链路与同一 `writeback_confirmations` 表）：c05 高风险确认键已泛化为 `(subject_type, subject_id)`，c07 译文文书以 `subject_type=translation_job` + `subject_id=translation_jobs.job_id`（取自 c07 自有 `translation_jobs` 主键，稳定可回读）为确认 subject，c07 MUST NOT 向 c04 `conversations` / `messages` 写入译文文书行、MUST NOT 依赖或重定义任何翻译专属 `conversations.module` 取值；高风险译文下发 / 落库前将待下发内容交 c05 服务端 `risk_type` 分类器判定，命中高风险（诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息）时以 `(subject_type=translation_job, subject_id=translation_jobs.job_id)` 为键进入 c05 高风险确认链路，按 `confirmed_role∈{doctor,reviewer}` 裁决——普通用户仅能生成草稿 / 提交审核 MUST NOT 直接下发，授权角色确认后方可下发；`risk_type` 判定与 `writeback_confirmations` 记录归 c05，c07 仅前置消费并写 `audit_logs`；与文件级落库确认（8.3）并存不重叠；验证：高风险译文以 `subject_type=translation_job`/`subject_id=job_id` 为键落 c05 writeback_confirmations 可按 `(subject_type, subject_id)` 回读核对（不写 c04 会话表），普通用户仅能提交审核、授权角色确认后下发（对应 spec「高风险译文以 translation_job 为确认 subject 消费 c05 确认链路」「普通用户高风险译文仅能提交审核」「授权角色确认后下发并落确认记录」场景）
- [ ] 10.4 翻译任务成功率验收：对 §24.4 验收集批量执行文件级翻译并支持参考译文对比；验证：医学翻译任务成功率 ≥ 95%（「翻译任务成功率达标」场景）

## 11. 主验收闭环与多入口联调

- [ ] 11.1 接入四类发起入口（左侧导航 / 文档中心右键「发起翻译」/ AIMed 答案操作栏「翻译 / 保存后翻译」/ ONLYOFFICE 医疗 AI 面板「医学翻译」）并各跑通一次发起；验证：四入口均能创建任务进入待翻译列表
- [ ] 11.2 端到端跑通主验收闭环翻译段：上传论文 → 解析 / 视觉解析 → 翻译（仅译文 / 左右对照 / 逐段对照）→ 生成译文新版本 → 存文档中心 → 打开到 ONLYOFFICE → 最近任务恢复；验证：闭环跑通且全链路写 `audit_logs`
- [ ] 11.3 §17.6 翻译管理最小入口：翻译引擎只读路由视图 / 任务队列查看 / 失败任务重试 / 最小翻译质量反馈入口可查可操作（不做评测闭环；术语库 / 语料库配置见第 12 组）；翻译质量反馈写入 §18 `feedbacks` 表（建表 owner=c04 所泛化的多来源反馈表，c07 仅写入消费、不建表不改表结构），按 `subject_type=translation_job` + `subject_id=translation_jobs.job_id` 关联、按 `tenant_id` 隔离，`reason` 取翻译质量维度或自由文本 `comment`；验证：管理入口可见、可查队列并对失败任务重试、可提交一条质量反馈且该反馈以 `subject_type=translation_job`/`subject_id=job_id` 落 c04 所建 `feedbacks` 表、按 tenant 隔离可回读（对应 spec「翻译管理后台（最小）」的「管理员查看任务队列并对失败任务重试」「管理员查看翻译引擎路由视图」「提交一条翻译质量反馈」场景）

## 12. 术语库 / 语料库管理与后台配置闭环（§17.6 / §17.8 / §24.6 / §0.3）

- [ ] 12.1 实现术语库 / 语料库管理（最小形态）：管理员创建 / 编辑术语库与术语条目（源词 / 目标词 / 领域 / 优先级）、新增 / 编辑语料库并导入条目，按 `tenant_id` 隔离并写 `audit_logs`（不做可视化编辑器与批量导入器，属 §22.2/§22.3 延期）；验证：创建 / 编辑术语库条目与新增 / 导入语料库分别落库且各写一条审计（对应 spec「术语库 / 语料库管理与后台配置」的「管理员创建并编辑术语库与术语条目」「管理员新增并导入语料库」场景）
- [ ] 12.2 实现术语库 / 语料库启停与绑定翻译用途：停用后的库不再可被前台翻译任务选用，绑定关系持久化并写 `audit_logs`；验证：停用 / 绑定生效且写审计（对应 spec「启停与绑定翻译用途」场景）
- [ ] 12.3 实现配置→前台生效闭环（§0.3）：管理员修改某源术语目标译法后，前台对同一源术语再发起翻译任务采用更新后译法并可经术语命中溯源核对；验证：术语条目更新后新任务译文采用新译法（对应 spec「配置术语条目后前台翻译按配置生效」场景）
- [ ] 12.4 实现翻译引擎 / 术语库 / 语料库配置连通性与前台生效：管理员配置翻译引擎（绑定 c03 `model_routes`「医学翻译」用途）+ 选定术语库 / 语料库并经 c03 `provider_health_checks` 执行连通性测试，通过后前台翻译走该引擎路由、命中所选术语库一致译法并体现语料库风格；连通性测试不通过的公网引擎不置为前台可用（与 redaction-gateway 未接入前公网默认关闭一致）；验证：配置 + 连通性测试通过后前台按配置生效、测试失败时该公网引擎不可用（对应 spec「配置后连通性测试通过且前台按配置生效」「连通性测试失败时阻止前台启用该公网引擎」场景）
