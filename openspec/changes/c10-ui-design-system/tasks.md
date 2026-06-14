## 1. 设计 token 体系与三套主题

- [ ] 1.1 从 `docs/design/` 原型提取三套主题 `THEMES`（blue/green/dark），扩展 `apps/web/src/lib/theme.ts`：`ThemeId` 增加 `"dark"`，`THEME_TOKENS` 补全完整 token（文字 4 级、border/border-strong、bg/surface/surface-2、success/warning/danger/info、shadow sm/md/lg + focus ring、spacing/radius/font 阶梯）（对应 spec「design token 体系与三套主题」）
- [ ] 1.2 保持 `applyTheme(themeId, branding?)` 签名不变并扩展支持 dark；验证租户 branding（主色/辅助色/圆角/字号/Logo）覆盖优先级 = 主题基础 token < branding，覆盖后写入 `:root` 即时生效（对应 spec「branding 覆盖主题 token」）
- [ ] 1.3 三套主题运行时切换无需刷新：切换后 `:root` CSS 变量更新、`data-theme` 同步，门户与各模块宿主页统一跟随；深色主题与主色文字对比度达 WCAG AA（对应 spec「三套主题运行时切换」）
- [ ] 1.4 在 `styles/global.css` 以 token 重写基础元素（body/a/button/input 默认），移除写死颜色

## 2. 基础组件库

- [ ] 2.1 新建 `apps/web/src/components/`：Button（primary/secondary/ghost/danger + hover/disabled/loading 态）、Input/Select、Card、Tag/Badge、Tabs、Breadcrumb，全部读 token、随主题换肤（对应 spec「基础组件库随主题换肤」「按钮状态」）
- [ ] 2.2 通用态组件：EmptyState、Skeleton（加载）、ErrorState、NoPermission（对应 spec「通用态」）
- [ ] 2.3 叠层组件按当前消费方需要实现：Modal（删除/权限二次确认等）、Toast（操作反馈）；Drawer 留待有消费方时补（对应 spec「基础组件库随主题换肤」，遵循 design D6 范围纪律）

## 3. 医疗信任 UI 原语（仅呈现层）

- [ ] 3.1 CitationChip + 来源弹层：角标 `[n]`、点击展开来源（标题/期刊/年份/PMID/DOI/页码段落）的呈现组件，数据由调用方（c04/c06）注入（对应 spec「引用角标与来源弹层」）
- [ ] 3.2 RiskBanner（高风险提示条）、Disclaimer（医疗免责声明），可挂在 AI 产出与生成文档底部（对应 spec「高风险提示与医疗免责声明」）
- [ ] 3.3 ComplianceBadge：展示 PHI 脱敏门禁 / 模型环境（公网·私有化）/ 离线降级状态，状态由 c09/c03 提供，本组件仅渲染（对应 spec「合规徽标」）
- [ ] 3.4 ConfirmWritebackCard 呈现骨架：原文/修改后/修改说明/影响范围 + 应用到文档·生成副本·取消三按钮；只接 props 渲染，真实 diff 与确认链路由 c05 接入（对应 spec「写回确认卡呈现骨架」，遵循 design D3 边界）

## 4. 门户外壳重构

- [ ] 4.1 重构 `features/portal/PortalLayout.tsx`：分组可折叠左侧导航 + 线性图标（lucide-react）+ 底部三主题切换；导航结构与「医疗数字员工『规划中』入口」「管理后台仅管理员可见」不变（对应 spec「门户外壳布局」）
- [ ] 4.2 新增 `features/portal/TopBar.tsx`：面包屑 + 全局搜索 + 合规徽标 + 模型环境切换 + 用户菜单（对应 spec「门户外壳布局」）
- [ ] 4.3 `features/portal/ModuleShell.tsx` 适配新外壳与 token，保留模块标题/面包屑/工具条插槽契约

## 5. 重构 c01 已实现页面到新设计系统

- [ ] 5.1 `features/auth/LoginPage.tsx`：左品牌区 + 右登录卡，主按钮高对比 + loading 态，底部医疗免责声明（对应 spec「基础组件库随主题换肤」）
- [ ] 5.2 `features/documents/DocumentsPage.tsx`：表格/卡片切换视图 + 空态，文件操作用组件库；功能与权限行为不变
- [ ] 5.3 `features/recent/RecentTasksPage.tsx`：任务卡 + 进度 + 状态徽标 + 时间分组；功能不变
- [ ] 5.4 `features/admin/AdminPage.tsx`：RBAC / Provider（占位）/ Branding / Audit 分页（Tabs）；功能与权限不变

## 6. 设计源归档与文档

- [ ] 6.1 将 Claude Design 原型 `UI/` 移入 `docs/design/`，新增 `docs/design/README.md`：组件↔原型模块对照、三套主题 token 表、dc→React 转写约定

## 7. 依赖、构建与验收

- [ ] 7.1 `apps/web/package.json` 增 `lucide-react`；`npm run build --workspace=apps/web` 通过、无类型错误
- [ ] 7.2 验收：三套主题切换无刷新生效 + branding 覆盖生效；c01 底座冒烟（`npm run smoke --workspace=apps/api`）不回归；门户首页加载 ≤ 2s；逐页对照 `docs/design/` 原型确认还原度
