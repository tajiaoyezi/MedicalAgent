# 数据库迁移编号映射

单一共享迁移序列，按字典序执行（`apps/server/internal/db/migrations/*.sql`，`go:embed` 内嵌，`cmd/migrate` 用 pgx 简单协议逐文件执行）。各 change 申请新编号时查阅本表，**禁止撞号**。SQL 文件与原 `apps/api/src/db/migrations/` 字节一致（换栈仅改实现语言，schema 不变）。

| 编号 | change | 内容 |
|------|--------|------|
| 001 | c01-foundation | 核心表（tenants、users、documents 等） |
| 002 | c02-onlyoffice-bridge | `editor_conversion_cache`（OFD 转换缓存，非 §18 附属表） |
| 003 | c02-onlyoffice-bridge（修复） | `DROP document_parse_jobs` stub（owner 归 c03） |
| 004 | c03-model-and-parse | `model_providers` / `model_routes` / `provider_health_checks` / `visual_parse_providers` + `model:manage` 权限点授予 admin |
| 005 | c03-model-and-parse | `document_parse_jobs` / `document_visual_parse_results` + `document_event_consumptions`（消费侧记账，非 §18） |
| 006 | c03-model-and-parse | `document_chunks`（含 `chunk_acl` 物理列）/ `embeddings`（`chunk_id` 外键回连，无 `tenant_id` 列） |
| 007 | c04-aimed-rag-citation | `conversations` / `messages` / `citations` / `agent_runs` / `agent_steps` / `tool_calls` / `feedbacks`（唯一建表 owner=c04；`feedbacks.subject_id` 多态无 FK；`agent_checkpoints` 不建） |
| 008+ | c05-ai-panel-recent-tasks 起 | （**必须 ≥008**，排在 007 之后） |

## 横切契约

- `document_parse_jobs`：**唯一建表 owner = c03**（migration 005）。c02 仅只读消费（`routes/preview.go` parse-status）。
- parse-status SELECT 用 `document_version` + c03 状态词 + JOIN `document_visual_parse_results`，无 stub 列（`result`/`job_type`/`version_id`）。
- `model:manage` 权限点由 004 创建并授予 admin；新装库经 Go `db.Seed`（`internal/db/seed.go`）一并植入，既有库经 004 幂等补授。
- `embeddings` **无 `tenant_id` 物理列**：租户/`chunk_acl` 维一律经 `chunk_id` 外键从 `document_chunks` 派生（§16.3「embedding 属 chunk 元数据」）。
- `document_events` 闭合 6 类 event_type：c01/c02 产生（`routes/documents.go`、`editor/service.go`），**c03 纯消费**（`parsing/consumer.go`，不产生）。
