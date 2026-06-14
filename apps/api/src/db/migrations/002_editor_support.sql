-- c02 editor support: parse job read stub (owner=c03) + conversion cache

CREATE TABLE IF NOT EXISTS document_parse_jobs (
  job_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  document_id UUID NOT NULL REFERENCES documents(document_id) ON DELETE CASCADE,
  version_id UUID NOT NULL REFERENCES document_versions(version_id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  job_type TEXT NOT NULL CHECK (job_type IN ('visual', 'text', 'full')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (
    status IN ('pending', 'running', 'completed', 'failed')
  ),
  result JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (document_id, version_id, job_type)
);

CREATE TABLE IF NOT EXISTS editor_conversion_cache (
  source_hash TEXT PRIMARY KEY,
  target_object_key TEXT NOT NULL,
  target_mime TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
