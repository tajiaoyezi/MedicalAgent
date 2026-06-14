-- c03-model-and-parse｜chunk 与向量（PRD §16.3 / §18 命名）
-- owner=c03：document_chunks（含 chunk_acl 物理列）/ embeddings（经 chunk_id 外键回连 chunk）
-- 维度口径：chunk_acl=chunk 级物理列（本表）；document_acl=文档级派生维（c01 document_permissions，不落本表列）

-- chunk 元数据（§16.3 全字段；§16.3 单一 acl 正名为 chunk_acl 物理列）
CREATE TABLE IF NOT EXISTS document_chunks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  document_id UUID NOT NULL REFERENCES documents(document_id),
  document_version INTEGER NOT NULL,
  source_type TEXT,
  source_title TEXT,
  source_url TEXT,
  pubmed_id TEXT,
  doi TEXT,
  journal TEXT,
  year INTEGER,
  section TEXT,
  page INTEGER,
  paragraph_index INTEGER,
  chunk_text TEXT NOT NULL,
  -- chunk 级 ACL（默认继承来源文档级，允许 c06 写入严于文档级的范围；不得放宽）
  chunk_acl JSONB NOT NULL DEFAULT '{}'::jsonb,
  -- 重解析时旧 chunk 标记 superseded 保留版本可溯源
  superseded BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_chunks_doc_version
  ON document_chunks (document_id, document_version) WHERE superseded = FALSE;
CREATE INDEX IF NOT EXISTS idx_chunks_tenant
  ON document_chunks (tenant_id);

-- 向量行经 chunk_id 外键回连 chunk，继承 chunk 的 tenant_id / chunk_acl；
-- 本表 MUST NOT 物化 tenant_id 列（避免与 chunk 维双源）；§16.3「embedding 属 chunk 元数据」由该外键关系实现。
-- 注：本期未安装 pgvector，向量以 JSONB 数组存储（本期不含检索/召回，仅写入）。
CREATE TABLE IF NOT EXISTS embeddings (
  embedding_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  chunk_id UUID NOT NULL REFERENCES document_chunks(id) ON DELETE CASCADE,
  vector JSONB NOT NULL,
  model TEXT,
  dim INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_embeddings_chunk
  ON embeddings (chunk_id);
