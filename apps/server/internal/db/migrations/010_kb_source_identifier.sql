-- c06-knowledge-admin｜kb_documents 补来源标识列（4.6 PubMed/PMC 导入记录 pubmed_id/DOI 来源标识）
-- owner=c06（kb_documents 唯一建表 owner，009 建表）；本迁移仅 ALTER 补列、不重建表、不动他人 owner 的表。
-- §11.5.1 的 10 必录字段不变；source_identifier 为导入记录侧「来源标识」槽位（PubMed=pubmed_id、DOI 导入=DOI），
-- 与 §16.3 chunk 级 pubmed_id/doi（c03 document_chunks，按 chunk 溯源）为不同粒度、不重复（导入记录 vs chunk）。
ALTER TABLE kb_documents ADD COLUMN IF NOT EXISTS source_identifier TEXT;
