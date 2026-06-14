-- c03-model-and-parse｜解析作业与视觉解析结果（PRD §16.5 / §9.8 / §18 命名）
-- owner=c03：document_parse_jobs / document_visual_parse_results
-- 附属（消费侧记账，非 §18）：document_event_consumptions —— c03 作为 document_events 纯消费方的去重游标

-- 解析作业（PK=job_id；不保留 job_type：作业统一覆盖 detect→visual?→chunk→embed→handoff 全流程，
-- 路径由 detect 子状态决定，对外四态 pending/parsing/succeeded/failed）
CREATE TABLE IF NOT EXISTS document_parse_jobs (
  job_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  document_id UUID NOT NULL REFERENCES documents(document_id),
  document_version INTEGER NOT NULL,
  -- 对外四态
  status TEXT NOT NULL DEFAULT 'pending' CHECK (
    status IN ('pending', 'parsing', 'succeeded', 'failed')
  ),
  -- 内部子状态（对外归并为 parsing）：detecting/visual_parsing/chunking/embedding/indexing_handoff
  substatus TEXT,
  failure_reason TEXT,
  -- 触发来源：6 类 document_events event_type 之一，或 manual_retry
  triggered_by TEXT NOT NULL,
  actor_id UUID,
  index_ready_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_parse_jobs_doc_version
  ON document_parse_jobs (document_id, document_version);
CREATE INDEX IF NOT EXISTS idx_parse_jobs_pending
  ON document_parse_jobs (tenant_id, status, created_at);

-- 视觉解析结构化结果（§16.5）。访问控制继承来源文档级权限（由 c01 document_permissions 派生），
-- 本表不引入独立 chunk_acl 物理列、亦不作为 §11.9 六维检索过滤维。
CREATE TABLE IF NOT EXISTS document_visual_parse_results (
  result_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  document_id UUID NOT NULL REFERENCES documents(document_id),
  document_version INTEGER NOT NULL,
  job_id UUID REFERENCES document_parse_jobs(job_id) ON DELETE SET NULL,
  -- §16.5 结构化契约
  full_text TEXT,
  -- pages：每页 { page, paragraphs:[{paragraph_index,text,bbox,heading_level}], tables, images, header, footer, confidence }
  pages JSONB NOT NULL DEFAULT '[]'::jsonb,
  -- chunk 定位信息（供 document-parsing 切分与引用回 chunk）
  chunk_locators JSONB NOT NULL DEFAULT '[]'::jsonb,
  confidence NUMERIC,
  failure_reason TEXT,
  backend_kind TEXT,
  deployment_kind TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_visual_results_doc
  ON document_visual_parse_results (document_id, document_version);

-- c03 作为 document_events 纯消费方的消费去重游标（消费侧记账，不修改 owner 表）
CREATE TABLE IF NOT EXISTS document_event_consumptions (
  event_id UUID NOT NULL REFERENCES document_events(event_id),
  consumer TEXT NOT NULL,
  consumed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (event_id, consumer)
);
