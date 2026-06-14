# 设计：ui-design-system

## 背景与约束

- 现有 `apps/web` 为 React19 + Vite + react-router，**无 UI 库**，样式为 inline style + 少量全局 CSS；主题经 `lib/theme.ts` 的 `applyTheme(themeId, branding?)` 把 token 写入 `:root` CSS 变量，并支持租户 branding 覆盖（`lib/api.ts` 的 `Branding` 接口）。
- Claude Design 原型 `docs/design/` 为单文件 `dc-runtime` 产物（自定义 `<sc-if>` / `{{ }}` 模板 + 单体 Class Component），**不能直接搬入**，须转写为 React；其 `THEMES`（blue/green/dark）与 `applyTheme`/`navStyle` 是 token 与外壳转写的直接来源。
- 离线优先、医疗安全红线、简体中文界面（技术名词保留英文）等仓库口径不变。

## 关键决策

### D1 token 体系与 applyTheme 兼容路径
扩展现有 `lib/theme.ts` 而非另起炉灶：`ThemeId` 增加 `"dark"`；`THEME_TOKENS` 三套主题补全完整 token（文字 4 级、border / border-strong、bg / surface / surface-2、success / warning / danger / info、shadow sm/md/lg + focus ring、spacing / radius / font 阶梯）。保持 `applyTheme(themeId, branding?)` 签名不变，branding 覆盖优先级 = 主题基础 token < 租户 branding（primary_color / secondary_color / button_radius / font_size / logo）。组件一律读 `var(--token)`，不写死颜色，从而「换主题=换 :root 变量」零侵入。**不引入整套 Tailwind 重做主题**（会与 CSS 变量注入机制冲突、且偏离现有约定）。

### D2 三套主题与 portal-shell 的关系
科技深色为第三套主题，超出 c01「蓝白/绿白」原始 spec。鉴于 `openspec/specs/` 尚未 sync，本 change 不写 MODIFIED delta，而由 `ui-design-system` 的 token 需求承载三套主题；待 c01 sync 后在 sync 阶段把「三套主题」并入 portal-shell 主题需求。导航结构 / 默认进入 AIMed / 数字员工「规划中」入口等不变。

### D3 组件库边界：只做呈现层
基础组件（Button/Input/Card/Tag/Tabs/Breadcrumb/状态态/Modal/Drawer/Toast）与医疗信任原语（CitationChip/RiskBanner/Disclaimer/ComplianceBadge/ConfirmWritebackCard）**只提供呈现层**。数据与判定逻辑仍归各 owner：引用数据与定位→c04；写回 diff 与确认链路→c05；PHI 脱敏门禁与模型环境状态→c09/c03。本 change 的 ConfirmWritebackCard / ComplianceBadge 只接 props 渲染，MUST NOT 自行实现 diff、脱敏或确认落库。这样后续 phase 复用时不会与各自的功能 tasks 抢 owner。

### D4 dc→React 转写策略
逐模块对照原型转写：`{{ var }}` → JSX 插值；`<sc-if>` → 条件渲染；`onClick="{{ h }}"` → `onClick={h}`；`navStyle(id)` 等样式计算函数 → 组件内基于 token 的 className/style。单体 Class Component 拆为 `components/*` + `features/portal/*` + 各 `features/*/Page.tsx`。原型保留在 `docs/design/` 作为像素级参照，不参与构建。

### D5 图标
引入 `lucide-react`（线性图标，与原型风格一致、按需 tree-shake），用于侧栏 / 顶栏 / 组件。这是本 change 唯一新增前端依赖。

### D6 实现范围纪律（避免投机式过度构建）
组件库优先实现**门户外壳与 c01 四个已实现页面实际消费**的组件 + 蹲在外壳/顶栏里的小型信任原语；与未来 phase 强绑定、当前无消费方的重组件（如 ConfirmWritebackCard 的完整 diff 视图）只提供最小呈现骨架，待 c05 等 phase 落地时再补全。tasks 中此类条目可留待消费 phase 勾选，不在本 change 强行做满。

## 验收与回归
- 三套主题运行时切换无刷新、branding 覆盖仍生效；
- c01 底座冒烟（`npm run smoke --workspace=apps/api`）与功能不回归，路由 / API 契约不变；
- 门户首页加载 ≤ 2s（PRD §21）；深色主题与主色文字对比度达 WCAG AA。
