## ADDED Requirements

### Requirement: design token 体系与三套主题
系统 SHALL 提供以 CSS 变量为载体的统一 design token 体系，至少覆盖：主色与主色 hover、辅助色、success / warning / danger / info 语义色、文字 4 级（主 / 次 / 弱 / 禁用）、border / border-strong、bg / surface / surface-2、shadow（sm/md/lg）与 focus ring、间距 / 圆角 / 字号阶梯。系统 MUST 内置蓝白（默认）、绿白、科技深色三套主题，三套主题各自给出上述全部 token 取值；门户外壳与各模块宿主页 MUST 统一消费这些 token、MUST NOT 在组件中写死颜色值。

#### Scenario: 三套主题各自提供完整 token
- **WHEN** 系统加载蓝白 / 绿白 / 科技深色任一主题
- **THEN** 该主题在 `:root` 提供完整 token 集（主色 / 语义色 / 文字 4 级 / 边框 / 背景 / 阴影 / 间距圆角字号阶梯）
- **AND** 组件经 `var(--token)` 读取，无写死颜色

### Requirement: 三套主题运行时切换无刷新
系统 SHALL 沿用 `applyTheme(themeId, branding?)` 在运行时把所选主题 token 写入 `:root` 并同步 `data-theme`，用户或租户切换主题时门户与各模块宿主页 MUST 立即按新主题呈现、MUST NOT 需要刷新页面。科技深色主题正文与主色文字对比度 MUST 达到 WCAG AA。

#### Scenario: 切换主题立即生效
- **WHEN** 已登录用户在蓝白 / 绿白 / 科技深色之间切换主题
- **THEN** 门户与当前模块界面立即按新主题呈现，无需刷新
- **AND** `:root` 的 CSS 变量与 `data-theme` 同步更新

#### Scenario: 深色主题对比度达标
- **WHEN** 用户切换到科技深色主题
- **THEN** 正文文字与主色按钮文字对当前背景的对比度达到 WCAG AA

### Requirement: branding 覆盖主题 token
系统 SHALL 支持租户级 branding（Logo / 主色 / 辅助色 / 按钮圆角 / 字体大小）覆盖主题基础 token，覆盖优先级为主题基础 token 低于 branding；覆盖结果 MUST 写入 `:root` 即时生效，且 MUST NOT 破坏未被 branding 指定的其它 token。

#### Scenario: branding 覆盖主色与圆角
- **WHEN** 租户配置了自定义主色与按钮圆角并由门户启动时加载
- **THEN** 界面以 branding 主色与圆角呈现，其余未覆盖 token 仍取当前主题基础值

### Requirement: 基础组件库随主题换肤
系统 SHALL 提供消费 token 的可复用基础组件，至少包含：Button（primary / secondary / ghost / danger 形态及 hover / disabled / loading 状态）、Input / Select、Card、Tag / Badge、Tabs、Breadcrumb、Modal、Toast。所有组件 MUST 仅通过 token 取色，主题切换时 MUST 自动换肤；门户外壳与 c01 已实现页面 MUST 改用本组件库替换 inline style。

#### Scenario: 组件随主题切换换肤
- **WHEN** 主题在三套之间切换
- **THEN** 按钮 / 卡片 / 标签 / 输入框等组件随 token 自动换肤，无需逐组件改色

#### Scenario: 按钮呈现禁用与加载态
- **WHEN** 按钮处于 disabled 或 loading 状态
- **THEN** 按钮呈现可辨识的禁用 / 加载视觉且不可重复触发

### Requirement: 通用态组件
系统 SHALL 提供空状态、加载（骨架）、错误态、无权限态四类通用态组件，供各模块在无数据 / 加载中 / 出错 / 越权时统一呈现。

#### Scenario: 列表无数据展示空状态
- **WHEN** 某模块列表无可见数据
- **THEN** 展示统一的空状态组件（说明 + 可选操作入口），而非空白页

### Requirement: 门户外壳布局
系统 SHALL 提供重构后的门户外壳：左侧分组、可折叠、带线性图标的固定导航（含底部主题切换），以及顶栏（面包屑 / 全局搜索 / 合规徽标 / 模型环境切换 / 用户菜单）。导航结构 MUST 保持 c01 既有口径：包含 AIMed / 医疗知识库 / 医疗数字员工（「规划中」入口）/ 医学翻译 / 医疗模板库 / 文档中心 / 最近任务，管理后台仅管理员可见；默认进入 AIMed 不变。

#### Scenario: 侧边栏分组与折叠
- **WHEN** 用户在门户中折叠 / 展开左侧导航
- **THEN** 导航在分组与图标态 / 完整态之间切换，主工作区随之自适应，导航结构与入口不变

#### Scenario: 顶栏展示当前位置与合规状态
- **WHEN** 用户进入任一模块
- **THEN** 顶栏展示该模块面包屑与全局搜索、合规徽标与模型环境、用户菜单

### Requirement: 引用角标与来源弹层
系统 SHALL 提供引用角标（CitationChip）与来源弹层呈现组件：关键结论旁渲染 `[n]` 角标，点击展开来源信息（标题 / 期刊 / 年份 / PMID / DOI / 页码段落）。该组件 MUST 仅按调用方注入的数据渲染、MUST NOT 自行检索或定位来源（数据与定位逻辑归 c04 / c06）。

#### Scenario: 点击角标展开来源
- **WHEN** 用户点击 AI 结论旁的引用角标
- **THEN** 展开该引用的来源信息弹层（按注入数据渲染标题 / 期刊 / 年份 / 标识 / 定位字段）

### Requirement: 高风险提示与医疗免责声明
系统 SHALL 提供高风险提示条（RiskBanner）与医疗免责声明（Disclaimer）呈现组件，供诊疗 / 用药 / 医嘱类高风险内容与各 AI 产出、生成文档统一附带醒目提示与免责声明。

#### Scenario: AI 产出附带免责声明
- **WHEN** 模块展示一条 AI 生成的医学产出
- **THEN** 该产出附带医疗免责声明组件，标识其为草稿 / 辅助建议

#### Scenario: 高风险内容展示风险条
- **WHEN** 调用方标记某内容为高风险（诊疗 / 用药 / 医嘱）
- **THEN** 展示醒目的高风险提示条

### Requirement: 合规徽标
系统 SHALL 提供合规徽标（ComplianceBadge）呈现组件，用于展示 PHI 脱敏门禁状态、模型环境（公网 / 私有化）与离线降级状态。徽标状态 MUST 由调用方（c09 / c03）注入，本组件 MUST 仅渲染、MUST NOT 自行判定脱敏或模型可用性。

#### Scenario: 展示模型环境与脱敏门禁状态
- **WHEN** 顶栏 / 模块接收到当前模型环境与脱敏门禁状态
- **THEN** 合规徽标按注入状态渲染（如「私有化模型生效」「脱敏已切私有化」「离线缓存」）

### Requirement: 写回确认卡呈现骨架
系统 SHALL 提供写回确认卡（ConfirmWritebackCard）呈现骨架：逐项展示原文 / 修改后 / 修改说明 / 影响范围，并提供「应用到文档 / 生成副本 / 取消」三个操作位与底部医疗免责声明。该组件 MUST 仅按 props 渲染、MUST NOT 自行实现 diff 计算或确认落库（真实 diff 与确认链路归 c05）。

#### Scenario: 渲染四要素与三操作位
- **WHEN** 调用方传入原文 / 修改后 / 修改说明 / 影响范围数据
- **THEN** 确认卡逐项呈现四要素并展示「应用到文档 / 生成副本 / 取消」三个操作位与免责声明
