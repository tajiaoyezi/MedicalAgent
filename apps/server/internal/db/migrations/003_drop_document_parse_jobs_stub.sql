-- 清理 c02 误建 document_parse_jobs stub；正式 DDL 由 c03 004+ 迁移创建
-- 全仓扫描确认：无任何代码 INSERT/UPDATE document_parse_jobs，表恒空
DROP TABLE IF EXISTS document_parse_jobs;
