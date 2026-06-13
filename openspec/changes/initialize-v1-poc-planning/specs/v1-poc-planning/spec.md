## ADDED Requirements

### Requirement: V1.0 POC 必须只使用主 PRD 作为执行来源
规划系统 MUST 仅使用 `docs/MedOffice_AI_完整产品需求文档_V1.0.md` 作为 V1.0 POC phase、task、ADR、验收用例和实施计划生成的权威来源。

#### Scenario: 生成 V1.0 规划产物
- **WHEN** 生成 V1.0 POC phase、task、ADR、验收用例或实施计划
- **THEN** 生成产物 MUST 只从 `docs/MedOffice_AI_完整产品需求文档_V1.0.md` 推导执行范围
- **AND** `docs/MedOffice_AI_遗漏项与补充需求清单_V1.0.md` MUST NOT 扩大 V1.0 执行范围

### Requirement: V1.0 POC 实施任务必须只来自 P0
规划系统 MUST 只从主 PRD `22.1 P0 必做` 生成 V1.0 POC 实施任务。

#### Scenario: 能力只出现在 V1.1 或 V1.2+
- **WHEN** 某项能力只出现在 `22.2 V1.1 路线图` 或 `22.3 V1.2+ 后续规划`
- **THEN** 该能力 MUST 只记录为 roadmap 或 backlog
- **AND** 该能力 MUST NOT 出现在 V1.0 POC 实施任务中

#### Scenario: 正文能力描述与第 22 章冲突
- **WHEN** 正文能力描述与第 22 章优先级发生冲突
- **THEN** 规划系统 MUST 以第 22 章作为优先级裁决来源
- **AND** V1.0 POC task 清单 MUST 遵守 `22.1 P0 必做`

### Requirement: 规划型 change 不得把未来实施项暴露为当前 apply checkbox
规划型 change MUST 区分当前 change 的可执行规划任务与后续 implementation change 的业务实施任务。

#### Scenario: 记录未来 phase/task 计划
- **WHEN** 规划型 change 记录未来 V1.0 POC phase 或业务 task
- **THEN** 未来实施项 MUST NOT 使用 `- [ ]` checkbox
- **AND** 未来实施项 MUST 明确标注为非当前 apply 任务

#### Scenario: 执行规划型 change 的 apply
- **WHEN** 对规划型 change 执行 `openspec apply`
- **THEN** apply 任务列表 MUST 只包含当前规划 change 自身的可执行任务
- **AND** apply MUST NOT 要求立即实现账号、门户、ONLYOFFICE、RAG、翻译、模板或安全合规业务能力

### Requirement: 后续实施必须按 phase 拆分为独立 implementation change
后续实现 V1.0 POC 业务能力时，规划系统 MUST 为每个 phase 或足够小的 phase 子集创建独立 implementation change。

#### Scenario: 启动 foundation 实施
- **WHEN** 准备实施 `foundation` phase
- **THEN** 必须先创建独立 implementation change
- **AND** 该 implementation change 的 `tasks.md` 才能使用 `- [ ]` checkbox 跟踪 foundation 的真实开发任务

### Requirement: V1.0 POC 必须按 PRD 定义的 phase 顺序规划
规划系统 MUST 使用主 PRD `0.4 后续 phase / task 生成规则` 中定义的 phase 顺序与依赖关系。

#### Scenario: 生成 phase 清单
- **WHEN** 生成 V1.0 POC phase 计划
- **THEN** phase 清单 MUST 包含 `foundation`、`onlyoffice-bridge`、`model-and-parse`、`aimed-rag-citation`、`ai-panel-recent-tasks`、`knowledge-admin`、`medical-translation`、`template-center`、`security-evidence`
- **AND** phase 依赖 MUST 与主 PRD 定义的依赖顺序一致

### Requirement: 后续 task 必须可独立验收
每个后续 V1.0 POC task MUST 包含目标、范围、前置依赖、验收标准、测试/验证方式和风险。

#### Scenario: 审查单个后续 task
- **WHEN** 检查一个后续实施 task
- **THEN** task MUST 明确写出 `目标`、`范围`、`前置依赖`、`验收标准`、`测试/验证方式` 和 `风险`
- **AND** 验收标准 MUST 能通过 UI 检查、API 检查、数据检查、OpenSpec 校验、脚本测试或有记录的人工证据进行验证

### Requirement: 医疗数字员工必须排除在 V1.0 POC 真实实施范围外
规划系统 MUST 排除医疗数字员工创建、运行、编排、工具调用和执行历史相关的 V1.0 POC 实施任务。

#### Scenario: 正文中出现数字员工运行能力
- **WHEN** 正文提到医疗数字员工运行时、Agent 配置、工具调用、执行记录、工作流编排或执行历史
- **THEN** 这些内容 MUST NOT 生成 V1.0 POC 实施任务
- **AND** 这些内容 MAY 只记录在 roadmap 或 backlog 中

### Requirement: 当前文档发起 AIMed 与当前文档发起医学翻译必须进入 V1.0 P0
规划系统 MUST 将当前文档发起 AIMed 和当前文档发起医学翻译纳入 V1.0 POC P0 后续实施范围。

#### Scenario: 规划 ONLYOFFICE 医疗 AI 面板任务
- **WHEN** 生成 ONLYOFFICE 与医疗 AI 面板相关后续任务
- **THEN** task 计划 MUST 包含从当前文档发起 AIMed
- **AND** task 计划 MUST 包含从当前文档发起医学翻译

### Requirement: Roadmap backlog 必须可追溯但不阻塞 V1.0
roadmap 和 backlog 条目 MUST 保留来源章节和目标版本，但 MUST 不阻塞 V1.0 POC 实施。

#### Scenario: 记录 V1.1 或 V1.2+ 条目
- **WHEN** 遇到主 PRD `22.2` 或 `22.3` 的后续版本条目
- **THEN** 这些条目 MUST 只列入 roadmap 或 backlog
- **AND** 这些条目 MUST NOT 成为 V1.0 POC 验收要求
