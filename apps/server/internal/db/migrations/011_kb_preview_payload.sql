-- c06-knowledge-admin｜kb_documents 补预览载荷列（受控公网来源适配器的「确认时物化」隔离，D3）
-- owner=c06（kb_documents 唯一建表 owner）；仅 ALTER 补列、不重建表、不动他人 owner 的表。
-- preview_payload：PubMed/PMC 适配器取数得到的文献预览内容（标题/URL/pubmed_id/doi/journal/year/摘要）暂存于此，
-- 确认入库（ConfirmImport）时才物化为 c01 documents + document_chunks（§16.3）。确保「授权不明仅临时预览」「取消不落库」
-- 从存储层成立——预览阶段不产生任何正式可检索的 c01 文档/chunk（D3「staging 物理隔离」红线）。
ALTER TABLE kb_documents ADD COLUMN IF NOT EXISTS preview_payload JSONB;
