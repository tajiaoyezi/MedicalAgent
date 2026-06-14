## ADDED Requirements

### Requirement: 多入口与上传来源发起翻译
系统 SHALL 提供文件级医学翻译入口，覆盖医疗空间左侧导航「医学翻译」、文档中心右键菜单「发起翻译」、AIMed 答案操作栏「翻译 / 保存后翻译」、ONLYOFFICE 医疗 AI 面板「医学翻译」四个入口。上传来源 SHALL 支持拖入文件、点击 + 上传、选择本地文档、选择我的文档中心文件、选择团队文档中心文件、从当前 ONLYOFFICE 文档发起。选择文档中心或当前 ONLYOFFICE 文档作为来源时，系统 MUST 按 tenant_id 与文档级 ACL 过滤，禁止越权将无权访问的文档加入翻译任务。其中「AIMed 答案操作栏（翻译 / 保存后翻译）」入口的按钮挂载与点击分流（§8.12：选区 / 短文本走 AIMed 学术写作辅助、整篇 / 全文走医学翻译）由 c04 答案操作栏拥有并触发；c07 仅暴露「接收整篇翻译请求建 translation_job」的服务接口，不在本能力内渲染该按钮。

#### Scenario: 从左侧导航上传本地文件发起翻译
- **WHEN** 用户在「医学翻译」页通过拖入或点击 + 上传选择本地 docx 文件
- **THEN** 系统校验通过后将文件加入待翻译列表，并以当前用户 tenant_id / user_id 创建可后续提交的翻译任务

#### Scenario: 从文档中心右键发起翻译需通过 ACL 过滤
- **WHEN** 用户在文档中心对一篇有权访问的团队文档右键选择「发起翻译」
- **THEN** 系统校验该文档的 tenant_id 与文档级 ACL 命中当前用户后，将其加入待翻译列表

#### Scenario: 越权来源被拒绝
- **WHEN** 用户尝试将不属于其 tenant_id 或文档级 ACL 未授权的文档作为翻译来源
- **THEN** 系统拒绝加入并提示无权访问，不创建翻译任务，并写入 audit_logs

#### Scenario: 接收 AIMed 答案栏整篇翻译请求建 translation_job
- **WHEN** 用户在 AIMed 答案操作栏点击「翻译 / 保存后翻译」，c04 按 §8.12 判定为整篇 / 全文翻译并将该请求（文档 / 答案内容引用、语言方向等设置）传入 c07 翻译服务
- **THEN** c07 校验来源 tenant_id 与文档级 ACL 后创建 translation_jobs 任务并加入待翻译列表，返回医学翻译页路由目标；按钮挂载与选区 / 短文本走 AIMed 学术写作辅助的分流由 c04 答案操作栏负责，c07 仅提供建任务服务接口

### Requirement: 支持格式与上传限制校验
系统 SHALL 支持 Word（doc / docx）、PPT（ppt / pptx）、文本型 PDF、扫描 PDF、图片（png / jpg）以及 OFD（转换为 PDF / 文本抽取后）格式的文件级翻译。系统 MUST 在上传环节执行限制校验：单次最多上传 10 个文档、单文档大小 ≤ 50MB、暂不支持加密文档。任一限制不满足时，系统 MUST 拒绝该文档并返回明确原因，不得静默丢弃。

医学翻译文件上传作为 PRD §19.4 / §0.3 所定义的「上传时 PHI/PII 识别」拦截点（上传闸）之一，翻译文件在内容**持久化入库或送模型前** MUST 经 c09 security-compliance「上传时 PHI / PII 识别与『阻止上传』策略执行」契约（owner=c09，上传闸落点）完成识别与按策略处理：默认「识别并提示 / 脱敏后送模型」、策略=「阻止上传」时命中则拒绝入库；c07 仅前置消费该契约、自身 MUST NOT 实现 PHI/PII 识别脱敏。命中处理结果由 c09 写 privacy_redaction_events 并回填 audit_logs.id，被阻止的上传 MUST 写 result=失败且 failure_reason 非空的审计。该上传闸与本 spec「公网翻译模型调用前 PHI/PII 脱敏门禁」的出网闸是 §19.4 两个不同时点的拦截，二者均由 c09 唯一实现、c07 分别在上传入口与公网出口前置消费。

#### Scenario: 受支持格式与限制内文件通过校验
- **WHEN** 用户单次上传 3 个均 ≤ 50MB 的 docx / pptx / pdf 文件
- **THEN** 系统全部接受并加入待翻译列表

#### Scenario: 超过单次数量上限被拒绝
- **WHEN** 用户在单次上传中提交第 11 个文档
- **THEN** 系统拒绝第 11 个文档并提示「单次最多上传 10 个文档」

#### Scenario: 单文档超过大小上限被拒绝
- **WHEN** 用户上传一个 60MB 的文档
- **THEN** 系统拒绝该文档并提示「单文档大小 ≤ 50MB」

#### Scenario: 加密文档被拒绝
- **WHEN** 用户上传一个加密 / 设置打开口令的文档
- **THEN** 系统拒绝该文档并提示「暂不支持加密文档」，不进入解析与翻译

#### Scenario: OFD 转换后支持
- **WHEN** 用户上传 OFD 文件
- **THEN** 系统在转换为 PDF / 文本抽取后将其纳入翻译流程，无法转换时返回明确失败原因

#### Scenario: 翻译文件上传命中 PHI 按阻止上传策略被拒入库
- **WHEN** 用户上传的待翻译文件内容命中 PHI / PII，且当前策略配置为「阻止上传」
- **THEN** c07 在文件持久化入库或送模型前经 c09 上传闸识别命中后拒绝该上传并提示命中原因，不将含敏感信息的原文写入存储，由 c09 落 privacy_redaction_events 并写一条 result=失败、failure_reason 非空的 audit_logs

### Requirement: 待翻译列表与文档操作
系统 SHALL 提供待翻译列表，展示字段：文档名称、格式、大小、上传状态、解析状态、翻译状态、操作。其中上传状态、解析状态、翻译状态为三个彼此独立的列（§13.6），系统 MUST 能在「上传完成但解析未开始」「解析完成但翻译未开始」等阶段分别展示三列的独立取值；三列状态由 translation_jobs.status + progress 按固定规则派生映射（如 status=queued → 已上传 / 待解析 / 待翻译，status=parsing → 已上传 / 解析中 / 待翻译，status=translating → 已上传 / 已解析 / 翻译中 N%）。列表 MUST 支持添加文档、移除文档、预览原文三类操作。列表 SHALL 仅展示当前用户在其 tenant_id 下有权访问的待翻译文档。

#### Scenario: 列表展示全部规范字段
- **WHEN** 待翻译列表中存在已上传文档
- **THEN** 系统为每个文档展示文档名称、格式、大小、上传状态、解析状态、翻译状态与操作列

#### Scenario: 三列状态独立派生展示
- **WHEN** 某文档已上传成功但尚未开始解析（status=queued、progress=0）
- **THEN** 列表的上传状态、解析状态、翻译状态三列分别展示「已上传 / 待解析 / 待翻译」，三列取值彼此独立由 status + progress 派生

#### Scenario: 移除待翻译文档
- **WHEN** 用户对尚未提交翻译的文档点击「移除文档」
- **THEN** 系统将该文档移出待翻译列表并释放其占用名额

#### Scenario: 预览原文
- **WHEN** 用户对待翻译列表中的文档点击「预览原文」
- **THEN** 系统打开原文预览，且不修改原文档内容

### Requirement: 翻译设置与三种输出模式
系统 SHALL 提供翻译设置项：翻译引擎、语言方向、译文排版方式（layout_style）、术语库、语料库、输出格式（output_format）、是否保留原文、是否保留图片、是否保留表格、是否生成双语对照。语言方向 SHALL 至少支持中文简体→英语、英语→中文简体、中文简体→中文繁体、日语→中文简体等方向选择。输出模式 MUST 支持三种：仅译文、左右对照（左原文、右译文）、上下对照 / 逐段对照（原文段落后跟译文段落）。其中「译文排版方式」（layout_style）为与「输出模式」（output_mode）相区分的独立设置项，表达译文呈现的版式风格（如紧凑 / 宽松 / 保持原版式）；「输出格式」（output_format）表达译文可下载文件格式（枚举如 docx / pdf）。全部设置项（含 layout_style 与 output_format）MUST 持久化到 translation_jobs 对应字段且可回读。

#### Scenario: 选择仅译文输出
- **WHEN** 用户将输出模式设为「仅译文」并提交任务
- **THEN** 系统生成仅包含译文的结果文件

#### Scenario: 选择左右对照输出
- **WHEN** 用户将输出模式设为「左右对照」并提交任务
- **THEN** 系统生成左侧为原文、右侧为译文的对照结果

#### Scenario: 选择逐段对照输出
- **WHEN** 用户将输出模式设为「逐段对照」并提交任务
- **THEN** 系统生成原文段落后紧跟对应译文段落的对照结果

#### Scenario: 选择语言方向
- **WHEN** 用户选择语言方向「英语→中文简体」
- **THEN** 系统按该方向执行翻译，译文为中文简体

#### Scenario: 译文排版方式与输出格式可保存并回读
- **WHEN** 用户设置「译文排版方式」（layout_style）与「输出格式」（output_format，如 docx）并提交任务
- **THEN** 系统将 layout_style 与 output_format 持久化到 translation_jobs 对应字段，再次打开该任务设置时可回读到原值

### Requirement: 翻译任务状态机
系统 SHALL 以异步任务方式执行文件级翻译，并按状态机推进：排队中 → 解析中 → 翻译中（携带进度百分比，如「翻译中 15%」）→ 排版中 → 翻译成功；任意阶段失败转入翻译失败，用户主动取消转入已取消。任务状态 MUST 持久化到 translation_jobs，状态变更 MUST 写入 audit_logs 以可追溯。处于翻译中 / 排版中的任务 MUST 对外暴露进度。

#### Scenario: 任务按状态机正常完成
- **WHEN** 用户提交一个合法的待翻译文档
- **THEN** 任务依次经历排队中、解析中、翻译中（进度递增）、排版中并最终到达翻译成功

#### Scenario: 翻译中展示进度
- **WHEN** 任务处于翻译中阶段
- **THEN** 系统展示当前进度百分比（如「翻译中 15%」）

#### Scenario: 用户取消任务
- **WHEN** 用户对处于排队中 / 解析中 / 翻译中的任务点击取消
- **THEN** 任务转入已取消状态并停止后续处理，状态变更写入 audit_logs

#### Scenario: 阶段失败转入翻译失败
- **WHEN** 任务在解析中 / 翻译中 / 排版中任一阶段发生不可恢复错误
- **THEN** 任务转入翻译失败状态并记录失败原因

### Requirement: 公网翻译模型调用前 PHI/PII 脱敏门禁（消费 c09 redaction-gateway）
当翻译引擎或文档视觉解析需要调用公网模型 / 公网第三方 API 时，系统 MUST 先经脱敏门禁对待翻译内容执行 PHI / PII 识别与脱敏。PHI/PII 识别与脱敏引擎（redaction-gateway）及 privacy_detection_rules / privacy_redaction_events 由 c09 security-evidence 唯一实现；c07 不自行实现 PHI/PII 识别脱敏，仅在公网出口预留门禁接缝并消费 c09 的判定结果，识别与脱敏事件由门禁写入 privacy_redaction_events。当识别失败、脱敏置信度不足、识别服务不可用或 c09 判定结果不可用时，系统 MUST 禁止调用任何公网模型 / 公网视觉解析（默认拒绝公网），并 MUST 切换至私有化翻译模型 / 私有化解析的降级路径。本期口径：redaction-gateway 未接入前不得启用公网 provider，仅私有化 / 离线路径可跑通闭环（对齐 §16.4 / §24.9）。模型与解析路由 SHALL 通过 model_providers / model_routes / visual_parse_providers 配置公网与私有化入口及 fallback。

#### Scenario: 脱敏通过后调用公网翻译
- **WHEN** 任务配置使用公网翻译引擎且经 c09 redaction-gateway 的 PHI/PII 识别与脱敏成功、置信度达标
- **THEN** 系统以脱敏后内容调用公网模型，门禁写入 privacy_redaction_events

#### Scenario: 识别失败禁止公网调用
- **WHEN** PHI/PII 识别失败或脱敏置信度不足
- **THEN** 系统禁止调用公网模型，提示需切换私有化模型，并不向公网发送任何待翻译内容

#### Scenario: c09 判定不可用时默认拒绝公网
- **WHEN** c09 redaction-gateway 尚未接入或其识别判定结果不可用
- **THEN** 系统按「识别服务不可用」处理，默认拒绝任何公网 provider，仅以私有化 / 离线路径继续任务

#### Scenario: 公网识别服务不可用切私有化
- **WHEN** 公网识别 / 公网视觉解析服务不可用
- **THEN** 系统按 model_routes / visual_parse_providers 的 fallback 切换至私有化模型 / 私有化解析继续任务，或在无私有化路径时转入翻译失败并记录原因

### Requirement: Word / PPT 版式还原
对于 Word / PPT 文档，系统 SHALL 在译文中保持原文主要版式，MUST 保留标题层级、表格、图片、页眉页脚与页码结构，并 SHALL 尽量保留公式、角标与参考文献格式。版式还原效果 SHALL 满足版式结构保留率 ≥ 90% 的验收口径。

#### Scenario: Word 译文保留结构元素
- **WHEN** 用户翻译含标题层级、表格、图片与页眉页脚的 docx
- **THEN** 译文结果保留对应的标题层级、表格、图片与页眉页脚 / 页码结构

#### Scenario: 版式结构保留率达标
- **WHEN** 对验收测试集中的 Word / PPT 文档执行翻译并比对结构元素
- **THEN** 版式结构保留率 ≥ 90%

### Requirement: PDF 文本型与扫描型分流路由判定
由于文本型 PDF 与扫描 PDF 共用 .pdf 扩展名（§13.4），系统 MUST 在解析阶段对每个 .pdf 文件做文本层探测以判定走哪条管线：当可提取文本层覆盖率达到阈值时走文本型 PDF 管线（c03 document-parsing），否则走扫描 PDF 视觉解析管线（c03 visual-parsing-service）。对于混合页 PDF（部分页有文本层、部分页为扫描图像），系统 SHALL 按页分流或在无法可靠按页分流时整篇兜底走视觉解析；判定不可用时 MUST 保守兜底走视觉解析而非误入文本管线产出空译文。

#### Scenario: 文本型 PDF 与扫描件按文本层正确分流
- **WHEN** 用户分别上传一个有文本层的文本型 .pdf 与一个无文本层的扫描 .pdf
- **THEN** 系统经文本层探测将文本型 .pdf 路由到 document-parsing 文本管线、将扫描 .pdf 路由到 visual-parsing-service 视觉解析管线

#### Scenario: 混合页或判定失败时的兜底
- **WHEN** 上传的 .pdf 为部分页有文本层部分页为扫描图像，或文本层探测无法可靠判定
- **THEN** 系统按页分流，无法按页分流时整篇兜底走视觉解析管线，不将扫描页误送文本管线得到空译文

### Requirement: 文本型 PDF 翻译与版式还原
对于文本型 PDF，系统 SHALL 支持页级预览、段落级翻译、表格识别、图片位置保留，并 MUST 支持生成可下载的译文文件。译文 SHALL 保留原文的页与段落对应关系以支持对照与溯源定位。

#### Scenario: 文本型 PDF 段落级翻译并保留图片位置
- **WHEN** 用户翻译含表格与图片的文本型 PDF
- **THEN** 系统按段落翻译、识别表格、保留图片位置并生成可下载译文文件

#### Scenario: 页级预览可用
- **WHEN** 用户对文本型 PDF 翻译结果点击预览
- **THEN** 系统提供页级预览，原文与译文按页对应

### Requirement: 扫描 PDF / 图片走文档视觉解析翻译
对于扫描 PDF 与图片（png / jpg），系统 MUST 通过文档视觉解析服务识别文字、版面、表格与图片位置后再行翻译与版式还原，SHALL 支持左右对照预览并支持生成可下载译文。视觉解析结果（含页码、坐标、表格结构、图片位置、置信度、失败原因、chunk 定位信息）SHALL 复用 c03 提供的 document_visual_parse_results。视觉解析 SHALL 满足页码定位成功率 ≥ 90%、表格结构识别成功率 ≥ 85% 的验收口径。

#### Scenario: 扫描 PDF 经视觉解析后翻译
- **WHEN** 用户上传扫描 PDF 并提交翻译
- **THEN** 系统先调用文档视觉解析服务获得结构化结果（文字 / 版面 / 表格 / 图片位置 / 页码 / 置信度），再据此翻译并还原版式

#### Scenario: 视觉解析结果可溯源定位
- **WHEN** 用户查看扫描件译文并需要核对来源
- **THEN** 系统基于视觉解析的页码与坐标 / chunk 定位信息，将译文段落定位回原文对应位置

#### Scenario: 视觉解析失败转入翻译失败
- **WHEN** 文档视觉解析无法完成识别（如图像不可读）
- **THEN** 任务转入翻译失败并展示「无法完成文档视觉解析」类失败原因，允许重新提交

### Requirement: 术语一致性与术语库 / 语料库
系统 SHALL 在同一翻译任务内保持术语翻译一致，并 MUST 优先使用用户所选术语库（term_bases / terms），同时 SHALL 支持演示术语库与演示语料库（corpora）。术语一致性 SHALL 满足 ≥ 95% 的验收口径。译文中应用术语库的结果 SHALL 可溯源到所用术语条目以便核对。

#### Scenario: 同一任务术语保持一致
- **WHEN** 同一文档中某医学术语在多处出现且用户选定了术语库
- **THEN** 系统在全任务内对该术语采用术语库中一致的译法

#### Scenario: 术语一致性达标
- **WHEN** 对验收测试集执行翻译并以参考译文比对术语
- **THEN** 术语一致性 ≥ 95%

#### Scenario: 术语命中可溯源
- **WHEN** 用户在译文中核对某术语译法
- **THEN** 系统可展示该译法来源于哪个术语库 / 术语条目

### Requirement: 术语库 / 语料库管理与后台配置
系统 SHALL 为管理员提供术语库 / 语料库的最小管理与后台配置能力（对齐 §17.6 翻译管理与 §17.8 / §24.6「翻译模型、术语库、语料库配置」）。管理员 SHALL 能创建 / 编辑术语库（term_bases）及其术语条目（terms：源词 / 目标词 / 领域 / 优先级），SHALL 能新增 / 编辑语料库（corpora）并导入条目，SHALL 能对术语库 / 语料库执行启用 / 停用并绑定到翻译用途。全部管理操作 MUST 按 tenant_id 隔离并写入 audit_logs。本期为最小形态：仅条目级新建 / 编辑 / 导入 / 启停 / 绑定，不做术语库可视化编辑器与批量术语导入器（属 §22.2/§22.3 延期），但 MUST 保证配置端变更可被前台翻译任务即时消费（配置→生效闭环，对齐 §0.3）。

#### Scenario: 管理员创建并编辑术语库与术语条目
- **WHEN** 管理员在翻译管理后台新建一个术语库并添加 / 编辑一条术语条目（源词、目标词、领域、优先级）
- **THEN** 系统持久化该术语库与条目到 term_bases / terms，按 tenant_id 隔离并写入一条 audit_logs

#### Scenario: 管理员新增并导入语料库
- **WHEN** 管理员新增一个语料库并导入若干双语句对 / 风格参考条目
- **THEN** 系统持久化该语料库到 corpora，按 tenant_id 隔离并写入一条 audit_logs

#### Scenario: 启停与绑定翻译用途
- **WHEN** 管理员将某术语库 / 语料库设为停用，或将其绑定到「医学翻译」用途
- **THEN** 系统更新其启停状态与绑定关系，停用后的库不再可被前台翻译任务选用，写入 audit_logs

#### Scenario: 配置术语条目后前台翻译按配置生效
- **WHEN** 管理员将某源术语的目标译法修改为新译法并保存，随后前台对同一源术语发起新翻译任务并选用该术语库
- **THEN** 该任务译文采用更新后的目标译法，并可经术语命中溯源核对到更新后的术语条目（配置→前台生效闭环）

### Requirement: 翻译管理后台（最小）
系统 SHALL 为管理员提供 §17.6 翻译管理的最小可查可操作入口，覆盖：翻译引擎只读路由视图（复用 c03 model_routes「医学翻译」用途路由，仅展示不在本能力配置 provider）、任务队列查看、失败任务重试触发、最小翻译质量反馈提交入口。本期边界 MUST 为「仅最小反馈入口、不做翻译质量评测闭环」（评测闭环属 §22.2/§22.3 延期）。术语库 / 语料库配置部分由「术语库 / 语料库管理与后台配置」Requirement 承载，本 Requirement 不重复定义。

#### Scenario: 管理员查看任务队列并对失败任务重试
- **WHEN** 管理员在翻译管理后台查看任务队列并对一个翻译失败任务点击「重试」
- **THEN** 系统展示队列中任务及其状态，并将该失败任务重新提交进入排队中状态，写入 audit_logs

#### Scenario: 管理员查看翻译引擎路由视图
- **WHEN** 管理员在翻译管理后台查看翻译引擎配置
- **THEN** 系统以只读视图展示当前「医学翻译」用途绑定的 model_routes（公网 / 私有化入口、优先级、fallback），引擎 provider 的新增 / 编辑归 c03 model-provider-config

#### Scenario: 提交一条翻译质量反馈
- **WHEN** 管理员或用户对某翻译结果提交一条质量反馈
- **THEN** 系统将该质量反馈写入 §18 `feedbacks` 表（建表 owner=c04 aimed-rag-citation 所泛化的多来源反馈表，c07 仅作为写入侧消费方、不建表不改表结构），按 `subject_type=translation_job` + `subject_id=translation_jobs.job_id` 关联、按 `tenant_id` 隔离，`reason` 取翻译质量反馈维度取值或自由文本 `comment`，仅最小记录形态、不触发任何自动评测 / 评分；写入后可按 tenant 回读核对

### Requirement: 翻译引擎 / 术语库 / 语料库配置连通性与前台生效
系统 SHALL 支持 §0.3 后台配置闭环在翻译支的可验收落点：管理员配置翻译引擎（绑定 c03 model_routes「医学翻译」用途路由）、选定术语库与语料库并执行连通性测试后，前台翻译任务 MUST 按该配置生效。连通性测试 SHALL 复用 c03 provider_health_checks 对所选翻译引擎 provider 进行探测；术语库 / 语料库的「生效」以前台翻译任务命中所选术语库一致译法、体现所选语料库风格为判定。

#### Scenario: 配置后连通性测试通过且前台按配置生效
- **WHEN** 管理员配置翻译引擎（model_routes 绑定「医学翻译」用途）并选定术语库 / 语料库后执行连通性测试且通过
- **THEN** 前台发起一条翻译任务时输出走该引擎路由，命中所选术语库的一致译法并体现所选语料库风格

#### Scenario: 连通性测试失败时阻止前台启用该公网引擎
- **WHEN** 所选公网翻译引擎 provider 连通性测试不通过
- **THEN** 系统不将该公网引擎置为前台可用，前台翻译按 fallback 走私有化路由或提示需修复配置

### Requirement: 翻译历史与操作
翻译完成后系统 SHALL 进入翻译历史页，并提供操作：预览、下载、删除、重新翻译、打开到 ONLYOFFICE、查看原文、查看任务详情、查看失败原因。翻译历史 SHALL 仅展示当前用户在其 tenant_id 与 ACL 下有权访问的任务。每个历史任务 MUST 可追溯其状态变更、来源文档与失败原因。

#### Scenario: 翻译成功后可预览下载与在线打开
- **WHEN** 任务到达翻译成功状态
- **THEN** 用户可在翻译历史页对其执行预览、下载与打开到 ONLYOFFICE

#### Scenario: 查看任务详情与原文
- **WHEN** 用户在翻译历史点击「查看任务详情」或「查看原文」
- **THEN** 系统展示任务全链路状态、设置参数与来源原文

#### Scenario: 翻译历史按权限隔离
- **WHEN** 用户访问翻译历史
- **THEN** 系统仅返回其 tenant_id 与 ACL 命中的翻译任务，不展示他人任务

#### Scenario: 删除翻译历史任务（二次确认、不删译文文件、同步最近任务）
- **WHEN** 用户对一个翻译历史任务点击「删除」并完成二次确认（对齐 §6.7 删除规则）
- **THEN** 系统删除该翻译任务记录、同步更新 / 移除对应 `recent_tasks` 条目（按 `(ref_type=translation_job, ref_id=translation_jobs.job_id)` 定位），默认不删除已生成译文文件（`result_document_id` / `result_version_id` 对应版本保留），仅当用户显式选择「同时删除关联文档」时才一并删除关联译文文档，并写入一条 audit_logs

### Requirement: 翻译任务联动最近任务
作为「医学翻译」来源最近任务记录的写入方，c07 MUST 在翻译任务创建及任务到达翻译成功（succeeded）时，向 c01 所建 `recent_tasks` 表 upsert 一条记录：`source=医学翻译`（对齐 c01 recent-tasks-shell 定义的 §6.4 来源规范值）、`ref_type=translation_job`、`ref_id=translation_jobs.job_id`、`title` 取来源文档名 / 任务名，按 `tenant_id` / `user_id` 隔离，并以 `(ref_type, ref_id)` 为幂等键（同一翻译任务多次状态变更只更新同一条记录，不重复插入）。其中医学翻译来源的 `ref_type` 取值 MUST 逐字为 `translation_job`，该字符串为 c07（写入方）与 c05（恢复编排方）约定的统一取值，c05 按 `ref_type=translation_job` 路由回源；`ref_id` MUST 指向 `translation_jobs.job_id`（主键列名为 `job_id`），使 `(ref_type, ref_id)` 幂等键与 §6.6 恢复路由逐字对齐。最近任务的展示规则（§6.5）与恢复编排（§6.6）归 c05 ai-panel-recent-tasks，本能力仅负责「医学翻译」侧条目写入。该写入是 §0.3 医学翻译闭环「翻译历史进入最近任务」收尾步骤的可验收落点。

#### Scenario: 翻译任务创建 / 完成写入最近任务
- **WHEN** 用户创建一个医学翻译任务，且该任务到达翻译成功（succeeded）状态
- **THEN** 系统在 c01 所建 recent_tasks 落一条 source=医学翻译、ref_type=translation_job、ref_id 指向该 translation_jobs.job_id、按 tenant_id / user_id 隔离的记录

#### Scenario: 最近任务记录按租户隔离且幂等
- **WHEN** 同一翻译任务在创建后又到达翻译成功，触发两次最近任务写入
- **THEN** 系统按 (ref_type, ref_id) 幂等键只保留 / 更新同一条 recent_tasks 记录，不重复插入，且该记录仅对其 tenant_id / user_id 可见

#### Scenario: 从最近任务恢复进入翻译任务
- **WHEN** c05 恢复编排按该记录的 ref_id（translation_jobs.job_id）发起恢复
- **THEN** 系统据 ref_id 回源到对应翻译历史 / 译文（原文文件、译文文件、语言方向、术语库、翻译进度、历史版本，§6.6），c07 提供按 job_id 回源翻译任务的取数接口，恢复编排本体归 c05

### Requirement: 译文打开回 ONLYOFFICE 与写入文档中心可确认可审计
将译文打开到 ONLYOFFICE 或写入文档中心时，系统 MUST 生成新版本（document_versions）而非覆盖原文，遵循「可确认、可回滚、可审计」。写入文档中心或回写文档前 MUST 经用户确认。ONLYOFFICE 保存回调 SHALL 满足成功率 ≥ 99% 的验收口径。全部写入与打开操作 MUST 写入 audit_logs。文件级译文落库 / 打开回写的确认语义 MUST 对齐 §9.6 AI 写回确认机制中的默认策略「翻译结果→生成译文副本」（含医疗免责声明），c07 仅产出译文产物与 `result_document_id` / `result_version_id` 并执行文件级落库确认，MUST NOT 另起一套与 c05 ai-writeback-confirmation 重复的面板侧写回确认 UI；面板侧选区 / 全文写回确认归 c05 §9.6 矩阵，文件级翻译历史结果落库确认归 c07，两者职责边界不重叠。

当翻译任务到达翻译成功（succeeded）并经 c02 `save-callback-versioning` 为译文生成新 `document_version`（`source=translation`）落入文档中心时，c07 作为 PRD §10.6「翻译完成」触发源的唯一产生方（c01 document_events 契约将 `translation_done` 指派给 c07 产生），MUST 产生一条 c01 契约形态的 `document_events`，其 `event_type` 取值 MUST 为 `translation_done`，并携带 c01 规定的稳定契约字段 `(event_type, document_id, version_id, tenant_id, occurred_at, payload)`：`document_id`=译文新版本所属文档（`result_document_id`）、`version_id`=译文新版本（`result_version_id`）、`tenant_id`=任务租户。该 `translation_done` 事件为「触发重新解析与索引」的输入事件：其消费侧重解析 / 索引归 c03（c03 document-parsing 为 `document_events` 全部 6 类触发源的唯一重解析 / 索引消费方），c03 解析 / 索引就绪后另发「索引就绪」事件，再由 c04 检索侧构建索引、c06 知识库收尾侧消费该「索引就绪」事件刷新 `index_status` 与文档计数；c06 不直接消费 `document_events`，亦不直接消费本 `translation_done`。该事件与 `document_versions.source=translation` 的版本来源标记语义区分、不混用：前者是 document_events 的 6 类 `event_type` 之一，后者是版本来源字段取值。译文落库自身经 c02 保存回调产生的 `save_new_version`（保存新版本）事件不替代本 `translation_done` 事件，c07 在译文成功落库后 MUST 单独产生 `translation_done`。

#### Scenario: 译文生成副本而非覆盖原文
- **WHEN** 用户对翻译结果点击「打开到 ONLYOFFICE」
- **THEN** 系统以译文生成新版本 / 副本打开，原文档版本保持不变

#### Scenario: 写入文档中心前用户确认
- **WHEN** 用户选择将译文保存到文档中心
- **THEN** 系统在写入前请求用户确认，确认后生成新版本并写入 audit_logs

#### Scenario: 译文成功落库产生 translation_done 事件
- **WHEN** 翻译任务到达翻译成功（succeeded）并经 c02 为译文生成新 document_version（source=translation）落入文档中心
- **THEN** c07 产生一条 event_type=translation_done 的 document_events，携带 c01 契约稳定字段 (event_type, document_id=result_document_id, version_id=result_version_id, tenant_id, occurred_at, payload)，供 c03 作为 document_events 全部 6 类触发源的唯一重解析 / 索引消费方消费触发对译文新版本的重解析；c03 解析 / 索引就绪后另发「索引就绪」事件，再由 c04 检索侧构建索引、c06 知识库收尾侧消费该「索引就绪」事件刷新 index_status 与文档计数；c06 不直接消费本 translation_done

### Requirement: 高风险译文文书下发前的人工确认（消费 c05 高风险确认链路，以 translation_job 为确认 subject）
医学翻译文书与 AIMed 答案、知识库问答（kb_qa）答案同属高风险确认链路的医学文书：译文中若含用药 / 诊疗 / 医嘱 / 临床文书 / 患者个体信息类高风险结论，下发 / 落库前 MUST 经人工确认。c05 ai-writeback-confirmation 高风险确认链路的确认键已泛化为 `(subject_type, subject_id)` 多态键，三类生产方各以自身原生标识为确认 subject：AIMed / 知识库问答答案以 `subject_type=message`（`subject_id=messages.message_id`）、**医学翻译文书以 `subject_type=translation_job`（`subject_id=translation_jobs.job_id`）**。据此，当译文文书被识别为高风险（命中诊疗、用药、医嘱、临床文书或患者个体信息）时，c07 MUST 在内容下发 / 落库前接入 c05 高风险确认链路，并以 `subject_type=translation_job` + `subject_id=translation_jobs.job_id` 为确认 subject，与 AIMed 答案、知识库问答答案复用同一条确认链路与同一 `writeback_confirmations` 表。

**确认 subject 来源（消除悬空键、不写 c04 会话表）**：c07 译文文书的确认 subject 直接取自 c07 自有的 `translation_jobs.job_id`（本 change 为唯一建表 owner、主键稳定可回读），无需在 c04 所建 `conversations` / `messages` 落消息行取 `message_id`。c07 MUST NOT 向 c04 `conversations` / `messages` 写入译文文书行，MUST NOT 依赖、亦不重定义 c04 `conversations.module` 取值域（其枚举保持 c04 owner 定义、不含翻译值）；`writeback_confirmations` 的 subject 列由 c05 owner 泛化为承载 `document_id` / `message_id` / `translation_job` 多态，c07 以 `subject_type=translation_job`、`subject_id=translation_jobs.job_id` 为键落确认记录，确认键稳定不悬空、可按 `(subject_type, subject_id)` 回读验收。`risk_type` 高风险判定与 `confirmed_role` 角色裁决的唯一 owner 为 c05 服务端，本能力 MUST NOT 自建高风险判定或确认记录，仅作为该链路的生产方前置消费：本能力 SHALL 在译文文书下发 / 落库前将待下发内容交由 c05 服务端 `risk_type` 分类器判定。命中高风险时，确认 MUST 按 `confirmed_role∈{doctor,reviewer}` 裁决并以 `(subject_type=translation_job, subject_id=translation_jobs.job_id)` 为键落 c05 所建 `writeback_confirmations`，普通用户只能生成草稿或提交审核、MUST NOT 完成最终确认与下发；具备 `doctor` 或 `reviewer` 角色者方可确认下发。确认记录与审计由 c05 owner 写入，本能力仅触发该链路并将翻译行为记录到 `audit_logs`。医学翻译来源的恢复 / 路由经 `recent_tasks` 的 `ref_type=translation_job` / `ref_id=translation_jobs.job_id` 实现（见「翻译任务联动最近任务」Requirement），与上述确认 subject 同以 `translation_jobs.job_id` 回源、不依赖 `conversations.module`。该高风险确认与「译文打开回 ONLYOFFICE 与写入文档中心可确认可审计」Requirement 的文件级落库确认并存、职责不重叠：文件级落库确认对齐 §9.6「翻译结果→生成译文副本」（含医疗免责声明、生成新版本不覆盖原文），本 Requirement 解决高风险译文内容在下发给用户前是否需经医生 / 审核角色裁决（§19.2、§24.7）。

#### Scenario: 高风险译文以 translation_job 为确认 subject 消费 c05 确认链路
- **WHEN** 译文文书被 c05 服务端 `risk_type` 分类器识别为高风险，需进入高风险确认链路
- **THEN** c07 MUST 以 `subject_type=translation_job` + `subject_id=translation_jobs.job_id`（取自 c07 自有 `translation_jobs` 主键）为确认 subject 前置消费 c05 确认链路，MUST NOT 向 c04 `conversations` / `messages` 写入译文文书行、亦不依赖任何翻译专属 `conversations.module` 取值
- **AND** c05 以该 `(subject_type, subject_id)` 为键落 `writeback_confirmations`，确认记录可按 `(subject_type=translation_job, subject_id=translation_jobs.job_id)` 回读核对，确认键稳定不悬空

#### Scenario: 普通用户高风险译文仅能提交审核
- **WHEN** 普通用户对被 c05 服务端 `risk_type` 分类器识别为高风险（命中诊疗 / 用药 / 医嘱 / 临床文书 / 患者个体信息）的译文文书请求下发 / 落库
- **THEN** 系统 MUST 在下发 / 落库前进入 c05 高风险确认链路（以 `subject_type=translation_job`、`subject_id=translation_jobs.job_id` 为键），阻止其直接下发，仅允许生成草稿或提交 `doctor` / `reviewer` 角色审核，MUST NOT 在未经授权角色确认前将高风险译文下发给用户

#### Scenario: 授权角色确认后下发并落确认记录
- **WHEN** 具备 `doctor` 或 `reviewer` 角色的用户对高风险译文文书点击确认下发
- **THEN** 系统允许下发，并以 `subject_type=translation_job`、`subject_id=translation_jobs.job_id` 为键向 c05 `writeback_confirmations` 生成确认记录（含 confirmed_by / confirmed_role（取值 ∈ {doctor, reviewer}）/ risk_type / audit_log_id）
- **AND** 该确认动作 MUST 写入 audit_logs；`risk_type` 判定与 `writeback_confirmations` 记录归 c05，c07 仅前置消费该链路

### Requirement: 从当前 ONLYOFFICE 文档发起翻译（接收面板侧传入）
当从 ONLYOFFICE 医疗 AI 面板「医学翻译」发起翻译时，c07 SHALL 接收 c05 面板侧 Bridge 传入的当前 document_id（及语言方向 / 术语库选择等设置），据此创建 translation_jobs 任务、加入待翻译列表并返回医学翻译页路由目标。其中「经 Bridge 取当前 document_id」与「打开医学翻译页」的执行归 c05 面板侧（§8.12 翻译分流 / §13.10）；c07 仅暴露「按传入 document_id 建任务」的服务接口并提供页面路由目标，不在本能力内重复实现面板侧取数与跳转。c07 MUST 在建任务时校验传入 document_id 对当前用户的 tenant_id 与文档级 ACL 权限。

#### Scenario: 接收传入 document_id 创建任务
- **WHEN** c05 面板侧将当前 document_id 与翻译设置传入 c07 翻译服务
- **THEN** c07 据该 document_id 创建 translation_jobs(source=onlyoffice_current)、加入待翻译列表并返回医学翻译页路由目标

#### Scenario: 当前文档无权限被拒绝
- **WHEN** 传入 document_id 的 ACL 未授权给该用户
- **THEN** c07 拒绝创建翻译任务并提示无权访问，写入 audit_logs

### Requirement: 失败原因展示与重新翻译
当文件加密、损坏、无法解析、无法完成文档视觉解析或版式无法重建时，系统 MUST 展示明确的失败原因，并 MUST 允许失败任务重新提交（重新翻译）。失败原因 MUST 持久化到 translation_jobs 并写入 audit_logs 以可追溯。重新翻译 SHALL 复用原来源文档与设置并重新进入状态机。

#### Scenario: 损坏文件展示明确失败原因
- **WHEN** 待翻译文档损坏导致无法解析
- **THEN** 任务转入翻译失败并展示「文件损坏 / 无法解析」类明确失败原因

#### Scenario: 失败任务重新翻译
- **WHEN** 用户对一个翻译失败任务点击「重新翻译」
- **THEN** 系统复用原来源与设置重新提交任务并重新进入排队中状态

#### Scenario: 翻译任务成功率达标
- **WHEN** 对验收测试集批量执行文件级翻译任务
- **THEN** 医学翻译任务成功率 ≥ 95%

### Requirement: 译文医疗免责声明
译文为机器辅助产物，系统 MUST 默认将其标识为草稿 / 辅助产物，需人工确认，不作为临床定论。翻译结果页与下载 / 导出的译文 MUST 展示医疗免责声明。

#### Scenario: 翻译结果页展示免责声明
- **WHEN** 用户查看任一翻译结果
- **THEN** 系统展示医疗免责声明并标识译文为辅助草稿，需人工确认

#### Scenario: 导出译文携带免责声明
- **WHEN** 用户下载 / 导出译文文件
- **THEN** 导出内容包含医疗免责声明
