## Why

c01-foundation 已落地，但其前端只是「未经设计的脚手架」：直接套用了深蓝默认侧边栏 + 占位白卡 + 低对比度主按钮，无图标体系、无层次节奏、无医疗品牌识别，也缺少引用溯源、写回确认、合规徽标、免责声明等医疗信任 UI 元素。与此同时，已通过 Claude Design 产出了一份覆盖全部模块、含蓝白/绿白/科技深色三套主题与完整 design token 的高保真原型（`docs/design/`）。

9 个 change 的 tasks 描述的是各页面「功能」（做什么），原型提供的是「视觉」（长什么样），二者正交。若把视觉逐页塞进 c02–c08 的 tasks，会把同一套设计语言复制到 7 个文件造成漂移（横切契约教训的复刻）。因此本 change 把视觉沉淀为**一层共享的 UI 设计系统**：design token + 基础组件库 + 重构后的门户外壳 + 医疗信任 UI 原语，作为横切基础层供全部前端 phase（c01 已实现页面与 c02–c08 后续页面）统一复用，各 phase 的功能 tasks 保持不变。

本 change 在依赖序上属横切 UI 基础层：apply 时机在 c01 之后、c02–c08 前端实现之前；后续各 phase 的页面在其既有功能 tasks 之上消费本设计系统与 `docs/design/` 原型。

## What Changes

- 新增 `ui-design-system` 能力：以 CSS 变量为载体的 design token 体系（文字 4 级、边框、背景/surface、success/warning/danger/info 语义色、阴影与 focus ring、间距/圆角/字号阶梯），内置**蓝白（默认）/ 绿白 / 科技深色三套主题**，沿用并扩展 c01 既有 `applyTheme(themeId, branding?)` 运行时注入机制与租户级 branding 覆盖（Logo / 主色 / 圆角 / 字号），切换无需刷新。
- 新增基础组件库（按钮多形态多状态、输入/下拉、卡片、标签/徽标、Tabs、面包屑、空态/加载/错误/无权限态、模态/抽屉/Toast 等），统一消费 token；门户外壳与 c01 已实现页面 MUST 改用组件库与 token，替换原 inline style。
- 新增医疗信任 UI 原语（仅呈现层，数据与流程仍归各能力 owner）：引用角标 CitationChip + 来源弹层、高风险提示条 RiskBanner、医疗免责声明 Disclaimer、合规徽标 ComplianceBadge（PHI 脱敏门禁 / 模型环境公网·私有化 / 离线降级）、写回确认卡 ConfirmWritebackCard（原文 / 修改后 / 修改说明 / 影响范围 + 应用·副本·取消的呈现骨架，数据由 c05 接入）。
- 重构门户外壳：分组可折叠左侧导航（线性图标 + 三主题切换）、新增顶栏（面包屑 / 全局搜索 / 合规徽标 / 模型环境切换 / 用户菜单），导航结构与「医疗数字员工『规划中』入口」保持不变。
- 重构 c01 已实现的登录 / 文档中心 / 最近任务 / 管理后台页面到新设计系统。
- 将 Claude Design 原型纳入 `docs/design/` 作为规范化视觉参考源。
- 无 **BREAKING** 行为变更：仅视觉与组件层重构，不改 c01 的功能契约、路由结构与后端 API。

## Capabilities

### New Capabilities
- `ui-design-system`：design token 体系与三套主题、基础组件库、门户外壳布局（侧栏 + 顶栏 + ModuleShell）、医疗信任 UI 原语（引用角标 / 风险条 / 免责声明 / 合规徽标 / 写回确认卡呈现骨架）、通用态（空 / 加载 / 错误 / 无权限）。

### Modified Capabilities
- `portal-shell`（owner=c01）：主题维度由「蓝白 / 绿白两套」扩展为「蓝白 / 绿白 / 科技深色三套」，门户外壳视觉由本 change 的设计系统接管。因 `openspec/specs/` 尚未 sync（portal-shell 主规格尚未部署），本扩展不以 MODIFIED delta 表达，而由 `ui-design-system` 的 token 需求承载；待 portal-shell 随 c01 sync 进主规格后，于 sync 阶段把「三套主题」并入 portal-shell 主题需求。导航结构、默认进入 AIMed、数字员工「规划中」入口等 portal-shell 既有需求不变。

## Impact

- 受影响前端：`apps/web` 的 `lib/theme.ts`（扩 token + dark）、`styles/global.css`、门户外壳（`features/portal/*`）、c01 已实现页面（auth / documents / recent / admin）；新增 `apps/web/src/components/*` 组件库与 `features/portal/TopBar.tsx`；新增前端依赖 `lucide-react`（线性图标）。不改 `apps/api` 后端。
- 受影响数据表：无（纯前端视觉与组件层，无 schema 变更、无横切表 owner 变化）。
- 对其它 phase 的依赖关系：本 change 是 c02–c08 前端实现的共享 UI 基础层；各 phase 在其既有功能 tasks 上消费本设计系统的 token、组件与信任原语，其 tasks 不被本 change 改写。
- 医疗安全 / 合规影响：把医疗信任 UI 元素（引用可溯源角标、高风险提示、医疗免责声明、PHI 脱敏门禁 / 模型环境 / 离线降级的合规徽标、写回确认卡呈现骨架）固化为可复用原语，使各能力的可溯源、人工确认、合规状态呈现一致；但其数据与判定逻辑仍归各 owner（引用→c04、写回确认链路→c05、PHI 门禁→c09），本 change 只提供呈现层、MUST NOT 自行实现这些判定。
- 人工确认 / 脱敏影响：ConfirmWritebackCard 仅提供「原文 / 修改后 / 修改说明 / 影响范围 + 应用·副本·取消」的呈现骨架，c05 接入真实 diff 与确认链路；ComplianceBadge 仅展示由 c09/c03 提供的脱敏门禁与模型环境状态。
- 审计影响：无新增审计动作（branding 变更审计已由 c01 落地）。
- 范围说明：三套主题超出 PRD「蓝白 / 绿白」原始范围，属经确认的小幅扩展；「医疗数字员工」仍仅保留「规划中」入口，不设计其页面。
