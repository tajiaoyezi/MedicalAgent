## ADDED Requirements

### Requirement: 医疗 AI 面板挂载与三类入口
系统 SHALL 在 ONLYOFFICE 编辑器右侧挂载医疗 AI 面板，并 MUST 同时提供三类入口：右侧固定图标「医疗 AI」、顶部自定义按钮「医疗空间」、以及选中文本时的选区浮层（润色 / 翻译 / 解释 / 补引用）。面板的打开、关闭、技能调用与内容回流 MUST 通过 c02 的 Bridge 面板控制能力（openAIPanel / closeAIPanel / runAIPanelSkill / streamContentToEditor）完成，本能力不直接读写编辑器底层。

#### Scenario: 通过右侧固定图标打开面板
- **WHEN** 用户在 ONLYOFFICE 编辑器中点击右侧固定图标「医疗 AI」
- **THEN** 系统调用 openAIPanel 在编辑器右侧展示医疗 AI 面板，面板根据当前文档类型（getDocumentType 返回 docx / pdf / ofd）渲染对应的 P0 功能列表
- **AND** 面板顶部 MUST 展示 §19.3 医疗免责声明文案

#### Scenario: 选中文本触发选区浮层
- **WHEN** 用户在文档中选中一段文本
- **THEN** 系统在选区附近浮层展示「润色 / 翻译 / 解释 / 补引用」四个快捷动作
- **AND** 用户点击任一动作时，系统 MUST 通过 getSelectedText 取得选区文本作为该动作的上下文

#### Scenario: 选区解释只展示不写回
- **WHEN** 用户在选区浮层点击「解释」（§9.3.2 选区浮层四动作之一）
- **THEN** 系统经 getSelectedText 组装选区上下文调用 c04 AIMed 就地问答，结果仅在选区浮层 / 面板内展示（§9.4 辅助显示「不写回」语义）
- **AND** 系统 MUST NOT 调用任何 c02 写回方法（replaceSelection / insertText / insertComment / insertCitation 等），MUST NOT 进入 `ai-writeback-confirmation` 确认网关、MUST NOT 对文档执行任何写回
- **AND** 该就地展示的医学回答 MUST 携带或可见 §19.3 医疗免责声明（复用面板顶部声明即可），即浮层独立呈现医学内容时声明 MUST 在场

#### Scenario: 顶部医疗空间按钮入口
- **WHEN** 用户点击顶部自定义按钮「医疗空间」
- **THEN** 系统打开医疗 AI 面板并将焦点定位到文档级 AI 功能区（全文润色 / 排版 / 校对 / 发起 AIMed / 发起医学翻译）

#### Scenario: 关闭面板
- **WHEN** 用户点击面板关闭按钮
- **THEN** 系统调用 closeAIPanel 收起面板，且不改动文档内容

### Requirement: 文档打开后默认展示医疗 AI 面板
文档在 ONLYOFFICE 中打开后，系统 SHALL 默认展示医疗 AI 面板（§5.4、§14.6、§14.8）。「文档打开后默认展示医疗 AI 面板」这一触发的唯一 owner 为本能力（c05 medical-ai-panel）：本能力 MUST 提供并拥有该默认展示触发（经 c02 `openAIPanel` 在文档打开后自动展示面板的能力），所有打开文档的入口（c08 模板生成文档打开、c04 AIMed 等）MUST NOT 各自重定义或自建该触发，只 **引用** 本能力的默认展示触发、并传入其新 `document_id`。默认展示后面板 MUST 按 `getDocumentType`（docx / pdf / ofd）渲染对应 P0 功能集并展示 §19.3 医疗免责声明。

#### Scenario: 文档打开默认展示医疗 AI 面板
- **WHEN** 用户在 ONLYOFFICE 中打开一篇文档（含模板复制生成的个人文档）
- **THEN** 系统 MUST 由本能力拥有的默认展示触发调用 `openAIPanel` 在编辑器右侧展示医疗 AI 面板，并按 `getDocumentType` 渲染对应 P0 功能集
- **AND** 面板顶部 MUST 展示 §19.3 医疗免责声明

#### Scenario: 下游入口引用本能力的默认展示触发
- **WHEN** c08 模板生成文档或 c04 等打开文档的入口以新 `document_id` 在 ONLYOFFICE 打开文档
- **THEN** 系统经本能力（c05）拥有的默认展示触发自动展示医疗 AI 面板，各打开入口 MUST NOT 自建或重定义该触发，仅引用本触发并传入对应 `document_id`

### Requirement: Word 文档 P0 AI 功能集
当 getDocumentType 为 docx 时，系统 SHALL 在面板中提供以下 P0 功能：全文润色、选区润色、校对、AI 论文排版、目录 / 更新目录 / 目录级别、分页、页眉页脚、段落、插入标注、辅助显示，并提供「从当前文档发起 AIMed」与「从当前文档发起医学翻译」入口。论文转 PPT、AI 文档脑图属 §22.2 V1.1 路线图，本能力 MUST NOT 在 V1.0 面板中提供。每项功能 MUST 按 §9.4 写回方式经 `ai-writeback-confirmation` 能力确认后写回，纯编辑器排版类操作（目录 / 分页 / 段落等）除外。

#### Scenario: 全文润色生成草稿并进入确认
- **WHEN** 用户选择「全文润色」并指定风格（更正式 / 更学术 / 更简洁 / SCI 英文风格 / 中文医学论文风格之一）
- **THEN** 系统通过 getFullText 取全文，按所选风格生成润色草稿，且 MUST NOT 直接覆盖原文
- **AND** 系统将「原文 / 修改后 / 修改说明 / 影响范围」交由 `ai-writeback-confirmation` 展示，默认写回策略为「全文修改→生成新文档」

#### Scenario: 选区润色替换选区
- **WHEN** 用户选中文本并选择「选区润色」
- **THEN** 系统对 getSelectedText 的文本生成润色结果，并在用户确认后通过 replaceSelection 替换该选区

#### Scenario: 校对以批注形式给出
- **WHEN** 用户选择「校对」
- **THEN** 系统对全文执行错别字、标点、医学术语、单位格式、缩写一致性、中英文空格、统计学表达、常识性文本错误校对（§15.2 八类），并以批注 / 建议形式（insertComment）输出，MUST NOT 直接改写正文

#### Scenario: 插入标注写回指定位置
- **WHEN** 用户在选区浮层点击「补引用」或在面板选择「插入标注」
- **THEN** 系统生成引用 / 批注 / 脚注内容，并在用户确认后通过 insertCitation / insertComment 写回到当前光标或选区指定位置
- **AND** 标注内容 MUST 可溯源至引用来源（标题 / 出处 / 段落定位），引用可点击率 MUST ≥ 95%

#### Scenario: 辅助显示不写回
- **WHEN** 用户选择「辅助显示」
- **THEN** 系统在面板内展示文档结构、引用与修改建议，MUST NOT 对文档执行任何写回

#### Scenario: 编辑器排版类操作直接执行
- **WHEN** 用户选择目录 / 更新目录 / 目录级别 / 分页 / 页眉页脚 / 段落对齐缩进行距之一
- **THEN** 系统通过 Bridge 编辑器操作（getDocumentOutline / applyStyle 等）直接执行该排版动作，此类不改写语义内容的操作可不经写回确认机制

### Requirement: PDF / OFD 文档 P0 AI 功能子集
当 getDocumentType 为 pdf 或 ofd 时，系统 SHALL 仅提供其 P0 子集：医学翻译、AIMed 学术助手、批注 / 预览。PDF 支持原生预览与上述 AI 处理；OFD 在 V1.0 MUST 经转 PDF / 文本抽取后支持 AIMed 与医学翻译，OFD 原生在线编辑与依赖第三方 SDK 的 OFD 批注 MUST NOT 纳入 V1.0。AI 文档脑图、文档生成 PPT 属 §22.2，本能力 MUST NOT 提供。

#### Scenario: PDF 预览并发起 AI 处理
- **WHEN** 用户在 PDF Viewer 中打开医疗 AI 面板
- **THEN** 系统展示 PDF 的 P0 子集（医学翻译 / AIMed / 批注），用户可在预览态发起这些处理
- **AND** PDF 在 V1.0 仅部分支持原生在线编辑，面板 MUST NOT 提供 docx 专属的全文润色 / 排版 / 目录等写回功能

#### Scenario: OFD 经转换后支持
- **WHEN** 用户对 OFD 文档发起 AIMed 或医学翻译
- **THEN** 系统先经转 PDF / 文本抽取得到可处理文本，再发起对应 AI 操作
- **AND** 若转换或文本抽取失败，系统 MUST 提示该 OFD 暂不可处理，且不进入公网模型调用

### Requirement: 从当前文档发起 AIMed
系统 SHALL 提供「当前文档发起 AIMed」入口（§24.2、§22.1 P0），将当前文档作为上下文创建 c04 的 AIMed 会话。本能力仅负责面板侧发起与上下文传递，MUST NOT 实现 AIMed 模式本体。发起 AIMed MUST NOT 直接写回文档。

#### Scenario: 以当前文档为上下文创建会话
- **WHEN** 用户点击「AIMed 学术助手 / 发起 AIMed」
- **THEN** 系统通过 getDocumentId / getDocumentTitle / getFullText（或选区）组装上下文，调用 c04 创建 AIMed 会话并附带 tenant_id / user_id
- **AND** 系统按 tenant_id / kb_id / user_id / role / document_acl / chunk_acl 过滤可用知识库与检索源（六维过滤由 c04 rag-retrieval 召回前执行，本能力消费其结果），越权来源 MUST NOT 进入会话上下文

#### Scenario: AIMed 回答可溯源且不直接写回
- **WHEN** AIMed 会话返回带引用的回答
- **THEN** 面板展示的回答 MUST 提供引用定位（可点击、可定位到来源页码 / 段落，引用源定位成功率 ≥ 90%）
- **AND** 系统 MUST NOT 将回答直接写入文档，如需写入须经 `ai-writeback-confirmation`

### Requirement: 从当前文档发起医学翻译
系统 SHALL 提供「当前文档发起医学翻译」入口（§24.2、§22.1 P0）。按 §8.12 分流规则，Word / PDF 文档内点击医学翻译、上传完整文档要求翻译全文 MUST 路由至 c07 医学翻译模块；选中文档中一段文字翻译由医疗 AI 面板 / AIMed 处理。本能力仅负责面板侧发起与上下文传递，MUST NOT 实现翻译任务本体（由 c07 落库 translation_jobs）。

#### Scenario: 文档内点击医学翻译路由至翻译模块
- **WHEN** 用户在 Word / PDF 文档内点击「医学翻译」或要求翻译全文
- **THEN** 系统按 §8.12 将请求路由至 c07 医学翻译模块，携带 document_id、语言方向与术语库选择发起文件级异步任务
- **AND** 文件级译文副本由 c07 产出并执行文件级落库确认（生成新版本不覆盖原文，对齐 §9.6「翻译结果→生成译文副本」与医疗免责声明），本能力 MUST NOT 经 `ai-writeback-confirmation` 对该文件级译文副本二次确认

#### Scenario: 选区短文本翻译由面板就地处理
- **WHEN** 用户在选区浮层点击「翻译」翻译选中的一段文字
- **THEN** 系统由医疗 AI 面板 / AIMed 就地返回译文，不创建文件级翻译任务

### Requirement: 面板公网模型调用的脱敏门禁
当面板触发的 AI 操作（润色 / 校对 / 翻译 / 辅助显示 / 解释 / 补引用 / 发起 AIMed，即所有可能含 PHI / PII 内容、需调用公网模型的面板动作）需要调用公网模型时，系统 MUST 在调用前先对文本经 c09 redaction-gateway 执行 PHI / PII 识别与脱敏判定（§19.4）。识别失败、脱敏置信度不足或识别服务（redaction-gateway）不可用时，系统 MUST 禁止调用公网模型，并提供切换私有化模型的降级路径。本能力不实现 PHI / PII 识别脱敏引擎，仅在公网出口预留门禁接缝、强制前置消费 c09 redaction-gateway 的判定结果，绝不绕过；脱敏门禁的端到端真实接入随 c09（phase 9）落地。

本期相位约束：redaction-gateway 接入前本期默认关闭公网 provider，仅私有化 / 离线路径跑通闭环（§16.4、§24.9）；redaction-gateway 判定结果不可用时，公网 provider MUST 按「识别服务不可用」保守处理，默认拒绝公网、仅走私有化 / 离线。

#### Scenario: 识别通过后脱敏送公网模型
- **WHEN** 用户发起需公网模型的面板操作（含润色 / 校对 / 翻译 / 辅助显示 / 解释 / 补引用 / 发起 AIMed）且经 c09 redaction-gateway 识别成功、脱敏置信度达标
- **THEN** 系统以脱敏后文本调用公网模型，并对该次调用留痕（写入 audit_logs，关联 privacy_redaction_events）

#### Scenario: 识别失败禁止公网调用
- **WHEN** 选区浮层的「解释 / 补引用」或其它需公网模型的面板操作触发，且 c09 redaction-gateway 识别失败、脱敏置信度不足或服务不可用
- **THEN** 系统 MUST 禁止本次公网模型调用，并提示用户改用私有化模型或取消
- **AND** 系统 MUST NOT 将任何未脱敏文本发送至公网模型

#### Scenario: redaction-gateway 未接入时默认关闭公网
- **WHEN** c09 redaction-gateway 尚未接入或不可用，用户发起需公网模型的面板操作
- **THEN** 系统 MUST 默认关闭公网 provider，仅经私有化 / 离线模型路径处理，MUST NOT 在无门禁判定的情况下放行公网调用
