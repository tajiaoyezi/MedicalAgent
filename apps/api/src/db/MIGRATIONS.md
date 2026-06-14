# 数据库迁移编号映射

单一共享迁移序列，按字典序执行。各 change 申请新编号时查阅本表，**禁止撞号**。

| 编号 | change | 内容 |
|------|--------|------|
| 001 | c01-foundation | 核心表（tenants、users、documents 等） |
| 002 | c02-onlyoffice-bridge | `editor_conversion_cache`（OFD 转换缓存，非 §18 附属表） |
| 003 | c02-onlyoffice-bridge（修复） | `DROP document_parse_jobs` stub（owner 归 c03） |
| 004+ | c03-model-and-parse | `document_parse_jobs` 等（**必须 ≥004**，排在 003 DROP 之后） |

## 横切契约

- `document_parse_jobs`：**唯一建表 owner = c03**（tasks 1.5）。c02 仅只读消费（`preview.ts` parse-status）。
- c03 apply 时必须完成 tasks **1.5a**（列对齐 + `preview.ts` SELECT 更新），见 `openspec/changes/c03-model-and-parse/tasks.md`。
