## ADDED Requirements

### Requirement: 按文件类型选择并加载对应的 ONLYOFFICE 编辑器
系统 SHALL 在用户从文档中心打开文档时，按文件扩展名路由到对应的 ONLYOFFICE 编辑器：docx/doc → Document Editor、xlsx/xls → Spreadsheet Editor、pptx/ppt → Presentation Editor、pdf → PDF Editor / Viewer。系统 MUST 通过 ONLYOFFICE Document Service 生成带签名的编辑器配置（含 document、documentType、editorConfig、callbackUrl），并以当前用户的文档 ACL 决定 mode 为 edit 或 view。系统 MUST NOT 对无法识别的扩展名静默回退到任意编辑器，而是返回不支持类型的明确错误。

#### Scenario: 打开 docx 进入文字编辑器
- **WHEN** 用户在文档中心点击一个扩展名为 docx 的文档且对其拥有「可编辑」及以上权限
- **THEN** 系统加载 ONLYOFFICE Document Editor，documentType 为 word，mode 为 edit，并注入指向本租户保存服务的 callbackUrl

#### Scenario: 打开 xlsx 与 pptx 路由到正确编辑器
- **WHEN** 用户打开扩展名为 xlsx 的文档
- **THEN** 系统加载 Spreadsheet Editor（documentType=cell）
- **AND** 用户打开扩展名为 pptx 的文档时系统加载 Presentation Editor（documentType=slide）

#### Scenario: 不支持的文件类型被拒绝
- **WHEN** 用户尝试打开一个系统未支持在线编辑的扩展名（例如 zip）
- **THEN** 系统不加载任何编辑器并提示「不支持的文件类型」，不生成编辑器配置

### Requirement: 在线打开与保存的性能约束
系统 SHALL 保证普通 docx / pptx / xlsx 文档从点击打开到编辑器可交互 ≤ 5 秒，普通 docx 的保存回调端到端处理 ≤ 10 秒。系统 MUST 暴露 ONLYOFFICE 保存回调成功率指标，且该成功率 MUST ≥ 99%。

#### Scenario: 普通文档在 5 秒内打开
- **WHEN** 用户打开一个体积处于普通范围的 docx 文档
- **THEN** 编辑器在 ≤ 5 秒内完成加载并可编辑

#### Scenario: 普通文档保存回调在 10 秒内完成
- **WHEN** 用户在 Document Editor 中编辑普通 docx 后触发保存
- **THEN** 系统在 ≤ 10 秒内完成从回调到新版本落库的链路，并在指标中累计一次成功的保存回调

### Requirement: PDF 预览并提供 AI 处理入口
系统 SHALL 使用 ONLYOFFICE PDF Editor / Viewer 打开 pdf 文件用于预览，并在 PDF 视图中提供医疗 AI 处理入口（医学翻译、AIMed 学术助手等下游能力的发起入口）。系统 MUST 保留 pdf 的页码信息，以便下游引用溯源能按页定位；pdf 的原生在线编辑在 V1.0 仅部分支持，系统 MUST NOT 因不支持的编辑操作而损坏原文件。

#### Scenario: PDF 可预览且可发起 AI 处理
- **WHEN** 用户打开一个 pdf 文档
- **THEN** 系统以 PDF Viewer 渲染该文档并提供「发起 AIMed」「发起医学翻译」等 AI 处理入口

#### Scenario: PDF 页码信息保留以供溯源
- **WHEN** 下游能力请求 pdf 当前页或某段文本所在页
- **THEN** 系统返回准确页码，使引用定位页码误差 ≤ 1 页

### Requirement: OFD 转 PDF 只读预览
系统 SHALL 通过将 ofd 文件转换为 pdf 来提供只读预览，V1.0 MUST NOT 提供 OFD 原生在线编辑。转换后的预览 MUST 标识为只读，并允许在其上发起医学翻译与 AIMed 学术助手等基于文本/PDF 的下游能力。

#### Scenario: OFD 以转 PDF 方式只读预览
- **WHEN** 用户打开一个 ofd 文档
- **THEN** 系统将其转换为 pdf 并以只读视图展示，界面标明「只读预览（OFD 转 PDF）」

#### Scenario: OFD 不提供原生在线编辑
- **WHEN** 用户在 OFD 预览中尝试进入编辑模式
- **THEN** 系统拒绝并提示 V1.0 不支持 OFD 原生在线编辑，可改为转 PDF 后处理

### Requirement: 图片预览并提供视觉解析入口
系统 SHALL 对 png / jpg 文件提供图片预览，并在预览界面提供「文档视觉解析」入口以展示/刷新该图片的视觉解析作业状态与结果。系统 MUST NOT 将图片当作可在线编辑的 Office 文档加载到编辑器。图片的视觉解析复用 c01 文档上传时产生的 `upload_success` 事件 → c03 事件驱动消费模型自动创建 `document_parse_jobs` 并执行（owner=c03），c02 MUST NOT 自行向 c03 投递解析任务、MUST NOT 自造按需解析触发接口；c02 的「视觉解析」入口职责终点是按 image `document_id` 读取并展示/刷新 c03 既有解析作业的状态/结果，与 c03 事件驱动消费模型一致（不悬空自造触发接口）。

#### Scenario: 图片可预览并提供视觉解析入口
- **WHEN** 用户打开一个 png 或 jpg 文件
- **THEN** 系统以图片预览方式展示并提供「视觉解析」入口

#### Scenario: 展示并刷新已投递的视觉解析状态
- **WHEN** 用户在图片预览中点击「视觉解析」
- **THEN** 系统按该 image 的 `document_id` 读取由 c01 `upload_success` 触发、c03 创建的 `document_parse_jobs` 视觉解析作业，并展示/刷新其状态与结果（状态刷新 ≤ 3 秒）
- **AND** c02 不向下游另行投递任务、不自造按需解析触发接口，作业的创建与执行均由 c03 事件驱动消费承担

### Requirement: 打开与编辑受文档 ACL 与租户隔离约束
系统 SHALL 在加载任何编辑器前按 tenant_id 与文档级 ACL 校验当前用户权限，并据 ACL 推导注入 ONLYOFFICE `editorConfig.permissions`：仅「可编辑」及以上以 edit 模式打开（`permissions.edit=true`）；「可评论」以评论/审阅模式打开（`permissions.edit=false`、`permissions.comment=true`，可在编辑器内插批注、不可改正文、不注入改正文 callbackUrl）；「可查看」以只读 view 模式打开（`permissions.edit=false`、`permissions.comment=false`）；无任何权限则拒绝打开。系统 MUST 由 ACL 推导并注入 `permissions.copy`（对齐 PRD §10.4「可评论 = 评论、查看、复制文本」「可查看 = 查看、下载受权限控制」）：「可评论」及以上=`permissions.copy=true`，「可查看」=`permissions.copy=false`，从而在编辑器层落实「复制文本属可评论及以上专属」的区分。系统 MUST NOT 让用户访问其它租户的文档。打开/编辑属访问类操作，其审计 MUST 仅写入 `audit_logs`（操作类型=open），MUST NOT 写入 `document_events`——`document_events` 仅承载 PRD §10.6 的 6 类重新解析/索引触发事件（owner=c01），不承载访问类审计。

#### Scenario: 仅查看权限以只读模式打开
- **WHEN** 用户对某文档仅有「可查看」权限并打开它
- **THEN** 系统以 view 模式加载编辑器，禁用保存与写回，不注入可写 callbackUrl

#### Scenario: 可评论权限以评论模式打开
- **WHEN** 用户对某文档仅有「可评论」权限并打开它
- **THEN** 系统以评论/审阅模式加载编辑器（`permissions.edit=false`、`permissions.comment=true`），不注入改正文 callbackUrl，用户可在编辑器内插入批注但不可改写正文

#### Scenario: 复制文本能力按 ACL 注入 permissions.copy
- **WHEN** 仅「可查看」用户打开文档
- **THEN** 编辑器以 `permissions.copy=false` 加载，编辑器内复制文本被禁用
- **AND** 当「可评论」（及以上）用户打开同一文档时编辑器以 `permissions.copy=true` 加载、复制文本可用

#### Scenario: 跨租户访问被拒绝
- **WHEN** 用户尝试打开不属于其 tenant_id 的文档
- **THEN** 系统拒绝并返回无权限错误，不生成编辑器配置，并记录一条审计日志

#### Scenario: 打开事件仅写 audit_logs
- **WHEN** 用户成功打开任一文档
- **THEN** 系统仅向 `audit_logs` 写入一条记录（操作者、`role`、`tenant_id`、操作类型=open、对象=document、时间、`result`），不向 `document_events` 写入任何记录
