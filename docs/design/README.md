# MedOffice AI 设计系统参考

本目录是 `c10-ui-design-system` 的**规范化视觉参考源**。`MedOffice AI 门户.dc.html` 是 Claude Design 产出的高保真原型（`dc-runtime` 单文件产物，仅作像素级参照，**不参与构建**）。实现已转写进 `apps/web`，下表为对照关系。

## 组件 ↔ 原型模块对照

| 原型区块 | apps/web 实现 |
|---|---|
| 左侧分组导航 + 主题切换 | `features/portal/PortalLayout.tsx` + `features/portal/nav.tsx` |
| 顶栏（面包屑 / 搜索 / 合规徽标 / 用户菜单） | `features/portal/TopBar.tsx` |
| 模块容器 | `features/portal/ModuleShell.tsx` |
| 登录（左品牌区 + 右登录卡） | `features/auth/LoginPage.tsx` |
| 按钮 / 输入 / 卡片 / 标签 / Tabs / 表格 / 模态 / 通用态 | `components/*`（`ui.css` 承载交互态） |
| 引用角标 / 风险条 / 免责声明 / 合规徽标 / 写回确认卡 | `components/medical.tsx` |
| 三套主题 token | `lib/theme.ts` |

各功能模块页（AIMed / 知识库 / 翻译 / 模板 / 文档 / 最近任务 / 管理后台）在其所属 phase（c04–c08、c01）的功能 tasks 之上消费本设计系统与本原型作为视觉参照。

## 三套主题 token

token 以 CSS 变量承载，经 `applyTheme(themeId, branding?)` 注入 `:root`，运行时切换无需刷新；租户 branding（主色 / 辅助色 / 圆角 / 字号 / Logo）覆盖主题基础值。完整取值见 `apps/web/src/lib/theme.ts`，主键如下：

- 主题 ID：`blue-white`（临床蓝，默认）/ `green-white`（人文绿）/ `dark`（科技深色）。
- 色彩：`--color-primary[-hover|-active|-soft|-softer]`、`--color-bg`、`--color-surface[-2|-3]`、`--color-nav-*`、`--color-text[|-2|-3|-disabled]`、`--color-border[-strong]`、`--color-divider`、`--color-success|warning|danger|info[-soft]`。
- 其它：`--shadow-sm|md|lg`、`--ring`、`--button-radius`、`--font-size-base`。

## dc → React 转写约定

- `{{ var }}` → JSX 插值；`<sc-if>` → 条件渲染；`<sc-for>` → `.map()`。
- `onClick="{{ handler }}"` → `onClick={handler}`；`style-hover` / `style-focus` → `ui.css` 中的 `:hover` / `:focus` 类。
- 原型内 `navStyle/modeStyle/...` 等样式计算函数 → 组件内基于 token 的 className/inline style。
- 原型写死的色值一律改读 `var(--token)`；图标用 `lucide-react` 等价替换原型的 inline SVG。
- 边界：组件只做呈现层，引用数据→c04、写回确认链路→c05、PHI 门禁 / 模型环境→c09 / c03 注入。
