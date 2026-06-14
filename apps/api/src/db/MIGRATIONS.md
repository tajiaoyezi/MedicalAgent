# 数据库迁移编号映射

单一共享迁移序列，按字典序执行。各 change 申请新编号时查阅本表，**禁止撞号**。

| 编号 | change | 内容 |
|------|--------|------|
| 001 | c01-foundation | 核心表（tenants、users、documents 等） |
| 002 | c02-onlyoffice-bridge | `editor_conversion_cache`（OFD 转换缓存，非 §18 附属表） |
| 003 | c02-onlyoffice-bridge（修复） | `DROP document_parse_jobs` stub（owner 归 c03） |
| 004 | c03-model-and-parse | `model_providers` / `model_routes` / `provider_health_checks` / `visual_parse_providers` + `model:manage` 权限点授予 admin |
| 005 | c03-model-and-parse | `document_parse_jobs` / `document_visual_parse_results` + `document_event_consumptions`（消费侧记账，非 §18） |
| 006 | c03-model-and-parse | `document_chunks`（含 `chunk_acl` 物理列）/ `embeddings`（`chunk_id` 外键回连，无 `tenant_id` 列） |
| 007+ | c04-aimed-rag-citation 起 | （**必须 ≥007**，排在 006 之后） |

## 横切契约

- `document_parse_jobs`：**唯一建表 owner = c03**（migration 005，tasks 1.5）。c02 仅只读消费（`preview.ts` parse-status）。
- c03 apply 已完成 tasks **1.5a**：`preview.ts` parse-status SELECT 改用 `document_version` + c03 状态词 + JOIN `document_visual_parse_results`，去 stub 列 `result`/`job_type`/`version_id`。
- `model:manage` 权限点由 004 创建并授予 admin；新装库经 `seedIfNeeded` 一并植入，既有库经 004 幂等补授。
- `embeddings` **无 `tenant_id` 物理列**：租户/`chunk_acl` 维一律经 `chunk_id` 外键从 `document_chunks` 派生（§16.3「embedding 属 chunk 元数据」）。
