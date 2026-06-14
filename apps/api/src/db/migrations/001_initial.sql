-- MedOffice AI foundation schema (c01) — idempotent where possible

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- tenants
CREATE TABLE IF NOT EXISTS tenants (
  tenant_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  org_type TEXT NOT NULL DEFAULT 'hospital',
  enabled_modules JSONB NOT NULL DEFAULT '[]'::jsonb,
  storage_quota_bytes BIGINT NOT NULL DEFAULT 10737418240,
  branding JSONB NOT NULL DEFAULT '{
    "logo_url": null,
    "primary_color": "#1677ff",
    "secondary_color": "#69b1ff",
    "login_background": null,
    "nav_style": "default",
    "button_radius": "6px",
    "font_size": "14px",
    "default_theme": "blue-white"
  }'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- roles (RBAC 唯一真值)
CREATE TABLE IF NOT EXISTS roles (
  role_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  UNIQUE (tenant_id, slug)
);

-- permissions
CREATE TABLE IF NOT EXISTS permissions (
  permission_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  description TEXT
);

-- role_permissions
CREATE TABLE IF NOT EXISTS role_permissions (
  role_id UUID NOT NULL REFERENCES roles(role_id) ON DELETE CASCADE,
  permission_id UUID NOT NULL REFERENCES permissions(permission_id) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission_id)
);

-- users
CREATE TABLE IF NOT EXISTS users (
  user_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  username TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  display_name TEXT NOT NULL,
  dept_id TEXT,
  is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, username)
);

-- user_roles
CREATE TABLE IF NOT EXISTS user_roles (
  user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  role_id UUID NOT NULL REFERENCES roles(role_id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, role_id)
);

-- documents
CREATE TABLE IF NOT EXISTS documents (
  document_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  owner_id UUID NOT NULL REFERENCES users(user_id),
  name TEXT NOT NULL,
  space TEXT NOT NULL CHECK (space IN ('my', 'team', 'app')),
  app_source TEXT CHECK (
    app_source IS NULL OR app_source IN ('aimed', 'translation', 'template', 'digital_staff', 'kb')
  ),
  mime_type TEXT,
  is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
  is_favorited BOOLEAN NOT NULL DEFAULT FALSE,
  current_version_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- document_versions
CREATE TABLE IF NOT EXISTS document_versions (
  version_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  document_id UUID NOT NULL REFERENCES documents(document_id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  document_version INTEGER NOT NULL,
  file_hash TEXT NOT NULL,
  saved_by UUID NOT NULL REFERENCES users(user_id),
  saved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  source TEXT NOT NULL CHECK (
    source IN ('user_edit', 'ai_writeback', 'translation', 'import', 'template')
  ),
  object_key TEXT NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  UNIQUE (document_id, document_version)
);

ALTER TABLE documents
  DROP CONSTRAINT IF EXISTS documents_current_version_fk;
ALTER TABLE documents
  ADD CONSTRAINT documents_current_version_fk
  FOREIGN KEY (current_version_id) REFERENCES document_versions(version_id);

-- document_permissions
CREATE TABLE IF NOT EXISTS document_permissions (
  permission_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  document_id UUID NOT NULL REFERENCES documents(document_id) ON DELETE CASCADE,
  principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'role', 'dept')),
  principal_id TEXT NOT NULL,
  permission_level TEXT NOT NULL CHECK (
    permission_level IN ('owner', 'manage', 'edit', 'comment', 'view', 'none')
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (document_id, principal_type, principal_id)
);

-- document_events (仅 6 类 event_type)
CREATE TABLE IF NOT EXISTS document_events (
  event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type TEXT NOT NULL CHECK (
    event_type IN (
      'upload_success',
      'save_new_version',
      'ai_writeback',
      'translation_done',
      'template_created',
      'manual_reindex'
    )
  ),
  document_id UUID NOT NULL REFERENCES documents(document_id),
  version_id UUID NOT NULL REFERENCES document_versions(version_id),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  payload JSONB NOT NULL DEFAULT '{}'::jsonb
);

-- recent_tasks
CREATE TABLE IF NOT EXISTS recent_tasks (
  task_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  user_id UUID NOT NULL REFERENCES users(user_id),
  source TEXT NOT NULL CHECK (
    source IN (
      'AIMed 学术助手',
      '医疗知识库问答',
      '医疗数字员工',
      '医学翻译',
      '在线文档 AI 操作',
      '模板生成文档'
    )
  ),
  title TEXT NOT NULL,
  ref_type TEXT CHECK (
    ref_type IS NULL OR ref_type IN ('conversation', 'document', 'translation_job', 'writeback_confirmation')
  ),
  ref_id UUID,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (tenant_id, user_id, ref_type, ref_id)
);

CREATE INDEX IF NOT EXISTS idx_recent_tasks_user_updated
  ON recent_tasks (tenant_id, user_id, updated_at DESC)
  WHERE deleted_at IS NULL;

-- audit_logs
CREATE TABLE IF NOT EXISTS audit_logs (
  audit_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  actor_id UUID,
  actor_role TEXT,
  action_type TEXT NOT NULL,
  target_type TEXT,
  target_id TEXT,
  result TEXT NOT NULL CHECK (result IN ('成功', '失败')),
  failure_reason TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created
  ON audit_logs (tenant_id, created_at DESC);
