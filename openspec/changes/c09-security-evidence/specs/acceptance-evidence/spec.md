## ADDED Requirements

### Requirement: 内置 Demo 数据集
系统 SHALL 随 V1.0 POC 交付一套内置 Demo 数据集，数据清单 MUST 至少包含：医学论文 PDF 2-3 篇、医学论文 DOCX 1 篇、扫描 PDF 1 个、PPTX 1 个、XLSX 1 个、图片 1 个、双栏论文 PDF 1 个，包含表格 / 公式 / 参考文献的翻译样例各 1 个，覆盖 docx / pptx / pdf / 扫描 PDF 的医学翻译参考译文，覆盖演示术语库核心术语的术语期望表，左右对照与逐段对照双语样例各 1 个，每个默认知识库至少 1 份演示文档，200 个真实可用模板（由 c08 模板中心作为资产 owner 经 `templates` / `template_categories` 表交付，c09 引用纳入清单、不重复装载），1 个演示医学术语库，1 个演示语料库，以及覆盖 AIMed 六大模式测试问题的 PubMed 离线缓存。该数据集 MUST 可在个人机或内网环境一键装载，使主验收闭环在无公网时仍可演示。

#### Scenario: Demo 数据集装载后清单齐备
- **WHEN** 在个人机或内网环境装载内置 Demo 数据集
- **THEN** 系统 MUST 提供上述全部数据项，且数量满足 PRD §20.4 规定的最少清单
- **AND** PubMed 离线缓存 MUST 覆盖 AIMed 六大模式测试问题，确保离线可演示

#### Scenario: 翻译参考与术语期望可用于验收比对
- **WHEN** 验收医学翻译能力
- **THEN** Demo 数据集 MUST 提供覆盖 docx / pptx / pdf / 扫描 PDF 的参考译文、术语期望表与左右 / 逐段双语对照样例
- **AND** 这些参考数据 MUST 可被 Evals 用于术语一致性与版式结构保留率比对

### Requirement: 内置验收测试集与用例字段
系统 SHALL 随 V1.0 POC 交付内置验收测试集，MUST 至少覆盖：主验收闭环用例、AIMed 六大模式测试问题、医学翻译测试文件、知识库问答测试集、模板创建测试集、ONLYOFFICE 保存回调测试、最近任务恢复测试、模型配置连通性测试、文档视觉解析测试、安全合规 / 脱敏测试、公网模型禁用场景测试、未授权 URL 导入负向测试；每个验收用例 MUST 记录用例 ID、前置数据、操作步骤、期望结果、通过证据、失败判定与关联 P0 需求。

#### Scenario: 验收用例字段完整
- **WHEN** 查看任意一条内置验收用例
- **THEN** 该用例 MUST 包含用例 ID、前置数据、操作步骤、期望结果、通过证据、失败判定与关联 P0 需求
- **AND** 缺失任一字段的用例 MUST 被判定为不合格用例

#### Scenario: 测试集覆盖全部规定用例类别
- **WHEN** 清点内置验收测试集
- **THEN** 测试集 MUST 覆盖 PRD §20.4 列出的全部用例类别，含安全合规 / 脱敏、公网模型禁用与未授权 URL 导入负向用例
- **AND** 每个类别 MUST 至少有 1 条可执行用例

### Requirement: 验收用例关联各 P0 需求
系统 SHALL 保证每个验收用例显式关联到 PRD §22.1 的 P0 需求：验收用例与 P0 需求 MUST 形成可追溯映射，所有 P0 必做项 MUST 至少被一条验收用例覆盖，无任何 P0 需求处于未被任何用例覆盖的状态。

#### Scenario: 每个 P0 需求均被用例覆盖
- **WHEN** 对照 PRD §22.1 P0 清单核查验收用例映射
- **THEN** 每个 P0 需求 MUST 至少关联一条验收用例
- **AND** 存在未被任何用例覆盖的 P0 需求时，验收 MUST 判定为不通过

#### Scenario: 用例可反查关联 P0 需求
- **WHEN** 查看任意验收用例
- **THEN** 该用例 MUST 标明其关联的一个或多个 P0 需求标识
- **AND** 通过该标识 MUST 可反查到对应的 P0 需求条目

### Requirement: Evals 指标与阈值
系统 SHALL 提供可重复执行的 Evals 跑批，按 PRD §20.3 度量并对照阈值判定：模式识别准确率 ≥ 85%、PubMed RAG Hit@5 ≥ 80%、引用可点击率 ≥ 95%、引用源定位成功率 ≥ 90%、文档上传解析成功率 ≥ 95%、ONLYOFFICE 保存回调成功率 ≥ 99%、医学翻译任务成功率 ≥ 95%、最近任务恢复成功率 ≥ 98%、医学翻译术语一致性 ≥ 95%、医学翻译版式结构保留率 ≥ 90%、文档视觉解析页码定位成功率 ≥ 90%、文档视觉解析表格结构识别成功率 ≥ 85%、引用定位页码误差 ≤ 1 页。任一指标低于阈值时，对应能力的验收 MUST 判定为不通过，结果 MUST 写入 `eval_results`。

#### Scenario: 指标达标判定通过
- **WHEN** 在内置 Demo 数据集与测试集上执行 Evals 跑批
- **THEN** 当某指标达到或优于 §20.3 阈值时，系统 MUST 判定该项通过并记录实测值
- **AND** 实测值与阈值 MUST 写入 `eval_results` 供复核

#### Scenario: 指标低于阈值判定不通过
- **WHEN** 某指标实测值低于 §20.3 阈值（如引用源定位成功率 < 90% 或保存回调成功率 < 99%）
- **THEN** 系统 MUST 将该能力对应验收判定为不通过
- **AND** 系统 MUST 记录失败项、实测值与关联用例供整改追溯

#### Scenario: 引用结果可溯源且引用定位达标
- **WHEN** 评测 AIMed / 知识库问答的引用能力
- **THEN** 生成回答 MUST 携带可点击引用并能定位到源文档位置，引用可点击率 ≥ 95% 且引用源定位成功率 ≥ 90%、引用定位页码误差 ≤ 1 页
- **AND** 引用结果 MUST 可溯源到具体来源，无法定位的引用 MUST 计入失败

### Requirement: 可观测性日志与 Metrics 采集
系统 SHALL 按 PRD §20.1 / §20.2 采集可观测性日志与 Metrics：日志 MUST 至少覆盖用户操作日志、文档编辑日志、AI 写回日志、Agent 运行日志（`agent_runs` / `agent_steps`，限 V1.0 内部 AI 任务追踪：AIMed / RAG / 翻译 / 文档 AI，不含数字员工执行历史）、工具调用日志、RAG 检索日志、翻译任务日志与错误日志；Metrics MUST 至少包含 AIMed 首 token 延迟、RAG 检索延迟、引用定位成功率、文档解析成功率、ONLYOFFICE 保存回调成功率、医学翻译任务成功率、最近任务恢复成功率与 AI 任务完成率。这些日志与 Metrics MUST 作为验收用例的通过证据来源。

#### Scenario: 关键行为产生日志与 Metrics
- **WHEN** 用户执行文档编辑、AI 写回、RAG 检索、翻译任务或触发 AIMed / RAG / 翻译 / 文档 AI 的内部 AI 任务运行
- **THEN** 系统 MUST 产生对应日志（含 Agent 运行日志 `agent_runs` / `agent_steps`）并更新相关 Metrics
- **AND** 这些日志（含 Agent 运行日志）与 Metrics MUST 可被验收用例引用为通过证据

#### Scenario: 错误行为可观测
- **WHEN** 任一能力发生错误或被门禁 / 权限拦截
- **THEN** 系统 MUST 记录错误日志并反映在相关 Metrics（如 AI 任务完成率、保存回调成功率）
- **AND** 验收用例 MUST 能据此判定失败并定位原因

### Requirement: 性能门槛验收
系统 SHALL 按 PRD §21 度量并判定性能门槛：门户首页加载 ≤ 2 秒、AIMed 首 token ≤ 5 秒、知识库搜索 ≤ 3 秒、普通 docx / pptx / xlsx 在线打开各 ≤ 5 秒、普通 docx 保存回调 ≤ 10 秒、文档解析状态刷新 ≤ 3 秒、文档视觉解析状态刷新 ≤ 3 秒、翻译进度刷新 ≤ 3 秒、最近任务恢复 ≤ 2 秒。任一性能项超过门槛时，对应能力验收 MUST 判定为不通过并记录实测耗时。

#### Scenario: 性能项达标判定通过
- **WHEN** 在内置 Demo 数据集上度量各性能项
- **THEN** 当实测耗时不超过 §21 门槛时，系统 MUST 判定该项通过并记录实测耗时
- **AND** 实测耗时 MUST 作为该用例的通过证据

#### Scenario: 性能项超标判定不通过
- **WHEN** 某性能项实测耗时超过 §21 门槛（如 AIMed 首 token > 5 秒或保存回调 > 10 秒）
- **THEN** 系统 MUST 将对应能力验收判定为不通过
- **AND** 系统 MUST 记录超标项与实测耗时供整改

### Requirement: 禁用公网模型时主验收闭环仍可完成
系统 SHALL 保证在禁用公网模型时主验收闭环（登录 → 门户 → 上传 → AIMed → 解析 → 检索 → 带引用回答 → 生成在线 Word → 文档中心 → ONLYOFFICE → AI 面板 → 用户确认写回 → 保存版本 → 最近任务恢复）仍可经私有化模型或离线 PubMed 缓存完成；AIMed 与医学翻译 MUST 同时具备公网与私有化路径验收，Embedding / Rerank MUST 支持独立配置与连通性验收，文档视觉解析 MUST 支持公网与私有化解析服务验收；模型 fallback MUST 记录 provider、失败原因、切换目标与审计日志。

#### Scenario: 禁用公网模型主闭环离线可完成
- **WHEN** 部署禁用公网模型并仅启用私有化模型与离线 PubMed 缓存
- **THEN** 主验收闭环 MUST 仍可完整跑通并产出带引用回答与可保存版本
- **AND** 验收用例 MUST 证明该闭环不依赖任何公网模型调用

#### Scenario: 含 PHI 内容禁用公网且可降级私有化
- **WHEN** 待处理内容含 PHI / PII 且公网模型调用被前置门禁拦截或识别服务不可用
- **THEN** 系统 MUST 禁止调用公网模型并允许切换私有化模型继续完成处理
- **AND** 该拦截与切换 MUST 进入审计日志，闭环 MUST 不中断

#### Scenario: 模型 fallback 留痕
- **WHEN** 一次模型调用因失败触发 fallback 切换至备用 provider
- **THEN** 系统 MUST 记录 provider、失败原因、切换目标与审计日志
- **AND** 该记录 MUST 可经审计日志溯源到本次 fallback

#### Scenario: 公私网双路径与连通性验收
- **WHEN** 验收 AIMed、医学翻译、Embedding / Rerank 与文档视觉解析的模型与服务路径
- **THEN** 系统 MUST 分别完成公网路径与私有化路径验收，Embedding / Rerank MUST 通过独立配置与连通性验收
- **AND** 任一路径不可用时验收 MUST 记录失败项，且写回前的用户确认与 ACL 权限过滤 MUST 始终生效

#### Scenario: 未接入脱敏门禁时公网 provider 不可启用
- **WHEN** 脱敏门禁 `redaction-gateway` 尚未接入或被禁用，验收尝试启用 `deployment_kind=public` 的公网 provider
- **THEN** 验收 MUST 判定该公网 provider 启用为「不允许」，仅私有化 / 离线路径可跑通闭环
- **AND** 该用例 MUST 证明本期主验收闭环默认公网关闭、私有化优先，并在 `eval_results` 记录该硬门判定结果

#### Scenario: 脱敏置信度不足负向用例有确定失败判定基线
- **WHEN** 以 POC 默认脱敏置信度阈值（0.9，随 `privacy_detection_rules` 装载）跑「脱敏置信度不足→禁止公网」负向用例，构造一条命中敏感信息但脱敏置信度低于该阈值的输入
- **THEN** 验收 MUST 据该确定阈值判定本次公网调用被禁止，给出确定的失败 / 拦截判定
- **AND** 实测置信度、所用阈值与拦截结果 MUST 写入 `eval_results` 供复核
