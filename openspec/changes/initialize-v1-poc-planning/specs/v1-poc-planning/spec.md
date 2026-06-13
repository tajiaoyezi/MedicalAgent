## ADDED Requirements

### Requirement: V1.0 POC 必须只使用主 PRD 作为执行来源
规划系统 MUST 仅使用 `docs/MedOffice_AI_完整产品需求文档_V1.0.md` 作为 V1.0 POC phase、task、ADR、验收用例和实施计划生成的权威来源。

#### Scenario: 生成 V1.0 规划产物
- **WHEN** 生成 V1.0 POC phase、task、ADR、验收用例或实施计划
- **THEN** 生成产物 MUST 只从 `docs/MedOffice_AI_完整产品需求文档_V1.0.md` 推导执行范围
- **AND** `docs/MedOffice_AI_遗漏项与补充需求清单_V1.0.md` MUST NOT 扩大 V1.0 执行范围

#### Scenario: 遗漏项清单仅用于审查
- **WHEN** 规划过程中引用历史遗漏项清单
- **THEN** 该清单 MAY 仅用于追溯、遗漏审查或历史风险复核
- **AND** 除非同一范围已出现在主 PRD P0 章节中，否则该清单 MUST NOT 生成 V1.0 实施任务

### Requirement: V1.0 POC 实施任务必须只来自 P0
规划系统 MUST 只从主 PRD `22.1 P0 必做` 生成 V1.0 POC 实施任务。

#### Scenario: 能力只出现在 V1.1 或 V1.2+
- **WHEN** 某项能力只出现在 `22.2 V1.1 路线图` 或 `22.3 V1.2+ 后续规划`
- **THEN** 该能力 MUST 只记录为 backlog 或 roadmap
- **AND** 该能力 MUST NOT 出现在 V1.0 POC 实施任务中

#### Scenario: 正文能力描述与第 22 章冲突
- **WHEN** 正文能力描述与第 22 章优先级发生冲突
- **THEN** 规划系统 MUST 以第 22 章作为优先级裁决来源
- **AND** V1.0 POC task 清单 MUST 遵守 `22.1 P0 必做`

### Requirement: V1.0 POC 必须按 PRD 定义的 phase 顺序规划
规划系统 MUST 使用主 PRD `0.4 后续 phase / task 生成规则` 中定义的 phase 顺序与依赖关系。

#### Scenario: 生成 phase 清单
- **WHEN** 生成 V1.0 POC phase 计划
- **THEN** phase 清单 MUST 包含 `foundation`、`onlyoffice-bridge`、`model-and-parse`、`aimed-rag-citation`、`ai-panel-recent-tasks`、`knowledge-admin`、`medical-translation`、`template-center`、`security-evidence`
- **AND** phase 依赖 MUST 与主 PRD 定义的依赖顺序一致

#### Scenario: 启动有依赖的 phase
- **WHEN** 某个 phase 依赖前置 phase
- **THEN** 该 phase 的 task MUST 标明前置 phase 或具体前置 task
- **AND** 该 phase 的验收标准 MUST 证明前置产物已经可用

### Requirement: V1.0 POC task 覆盖范围必须映射所有 P0 项
规划系统 MUST 在 V1.0 POC phase/task 计划中覆盖主 PRD `22.1 P0 必做` 的所有条目。

#### Scenario: 检查 P0 覆盖
- **WHEN** 审查 task 计划
- **THEN** `22.1 P0 必做` 中的每个 P0 条目 MUST 至少被一个 V1.0 phase task 或明确的验证任务覆盖
- **AND** 任何 V1.1 或 V1.2+ 条目 MUST NOT 成为 V1.0 验收前置条件

#### Scenario: 规划带来源链路的能力
- **WHEN** task 覆盖 AIMed、知识库、翻译、文档 AI 或安全合规范围
- **THEN** 规划 MUST 在适用处包含来源解析、权限过滤、引用或追溯、模型/provider 配置和验收证据

### Requirement: 每个生成 task 必须可独立验收
每个 V1.0 POC task MUST 包含目标、范围、前置依赖、验收标准、测试/验证方式和风险。

#### Scenario: 审查单个 task
- **WHEN** 检查一个 task
- **THEN** task MUST 明确写出 `目标`、`范围`、`前置依赖`、`验收标准`、`测试/验证方式` 和 `风险`
- **AND** 验收标准 MUST 能通过 UI 检查、API 检查、数据检查、OpenSpec 校验、脚本测试或有记录的人工证据进行验证

#### Scenario: 拒绝模糊 task
- **WHEN** 生成的 task 使用“完善”“优化”“处理一下”等缺少可测结果的模糊表述
- **THEN** 该 task MUST 在实施前重写为可验证任务

### Requirement: V1.0 POC 计划必须保留主验收闭环
phase 计划 MUST 支撑 PRD 定义的主验收闭环，以及知识库、医学翻译、模板、后台配置和安全脱敏分项闭环。

#### Scenario: 验证端到端就绪
- **WHEN** phase tasks 全部完成
- **THEN** POC MUST 支持从登录、AIMed、解析、RAG 引用、生成在线 Word、ONLYOFFICE 编辑、医疗 AI 写回确认、文档版本保存到最近任务恢复的主流程
- **AND** POC MUST 支持知识库、医学翻译、模板、后台配置和安全脱敏的独立验证闭环

### Requirement: 医疗数字员工必须排除在 V1.0 POC 真实实施范围外
规划系统 MUST 排除医疗数字员工创建、运行、编排、工具调用和执行历史相关的 V1.0 POC 实施任务。

#### Scenario: 正文中出现数字员工运行能力
- **WHEN** 正文提到医疗数字员工运行时、Agent 配置、工具调用、执行记录、工作流编排或执行历史
- **THEN** 这些内容 MUST NOT 生成 V1.0 POC 实施任务
- **AND** 这些内容 MAY 只记录在 roadmap 或 backlog 中

#### Scenario: 保留数字员工入口
- **WHEN** V1.0 门户保留医疗数字员工导航入口
- **THEN** 入口 MUST 只展示规划中或即将上线状态
- **AND** 页面 MUST NOT 暴露创建、运行、编排、工具调用或执行历史功能

### Requirement: 当前文档发起 AIMed 与当前文档发起医学翻译必须进入 V1.0 P0
规划系统 MUST 将当前文档发起 AIMed 和当前文档发起医学翻译纳入 V1.0 POC P0 实施范围。

#### Scenario: 规划 ONLYOFFICE 医疗 AI 面板任务
- **WHEN** 生成 ONLYOFFICE 与医疗 AI 面板相关任务
- **THEN** task 清单 MUST 包含从当前文档发起 AIMed
- **AND** task 清单 MUST 包含从当前文档发起医学翻译

### Requirement: Roadmap backlog 必须可追溯但不阻塞 V1.0
roadmap 和 backlog 条目 MUST 保留来源章节和目标版本，但 MUST 不阻塞 V1.0 POC 实施。

#### Scenario: 记录 V1.1 条目
- **WHEN** 遇到文档脑图、文档生成 PPT、论文转 PPT、模板投放规则、数字员工基础平台、智能任务、Agent 配置、工具调用、执行记录或 Agent Evals 等 V1.1 条目
- **THEN** 这些条目 MUST 只列入 roadmap 或 backlog
- **AND** 这些条目 MUST NOT 成为 V1.0 POC 验收要求

#### Scenario: 记录 V1.2+ 条目
- **WHEN** 遇到 OFD 第三方 SDK、复杂论文排版规则、多人协作高级权限、私有化部署工具、高级审计、文档水印、电子签章、HIS/EMR/OA 对接或数字员工接入产品模块等 V1.2+ 条目
- **THEN** 这些条目 MUST 只列入 roadmap 或 backlog
- **AND** 这些条目 MUST NOT 成为 V1.0 POC 验收要求
