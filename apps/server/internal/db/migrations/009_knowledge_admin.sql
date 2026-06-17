-- c06-knowledge-admin｜知识库管理基表（owner=c06，对齐 PRD §18 命名）
-- 唯一建表 owner=c06：knowledge_bases / kb_documents / source_whitelist_rules（c01/c02/c03 均不建）。
-- 仅消费/写入而不重建他人 owner 的表：document_chunks·embeddings 及 chunk_acl 列(c03)、citations(c04)、
-- conversations·messages(c04，module=kb_qa)、document_permissions·audit_logs·recent_tasks·document_events(c01)、
-- privacy_redaction_events(c09)。本迁移不创建上述任一张表（spec「不重复建他人 owner 的表」Scenario）。

-- ── knowledge_bases（13 库主表 + 卡片物化字段，§18 / D8 / §24.3）──
CREATE TABLE IF NOT EXISTS knowledge_bases (
  kb_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_by UUID REFERENCES users(user_id),         -- 平台预置库可空（系统种子）；用户创建库填创建人
  is_seed BOOLEAN NOT NULL DEFAULT FALSE,            -- §11.2 预置 13 库标记
  is_pinned BOOLEAN NOT NULL DEFAULT FALSE,          -- 管理员置顶（卡片字段「置顶状态」）
  manual_weight INTEGER,                             -- 手动权重，可空：NULLS LAST 精确区分「无配置」与显式 0（D2）
  data_source TEXT NOT NULL DEFAULT '',              -- 数据源标签
  member_count INTEGER NOT NULL DEFAULT 0,           -- 物化：知识库 ACL/document_permissions 授权用户去重计数（D2，刷新归 c06 ACL 阶段）
  document_count INTEGER NOT NULL DEFAULT 0,         -- 物化：index_status=indexed 文档计数（D2，刷新由 c03 索引就绪事件触发）
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_tenant ON knowledge_bases (tenant_id);
-- 确定性多级排序键（§11.3 / D2）：置顶 → 手动权重降序(NULLS LAST) → 更新时间倒序 → 创建时间倒序
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_sort
  ON knowledge_bases (tenant_id, is_pinned DESC, manual_weight DESC NULLS LAST, updated_at DESC, created_at DESC);

-- ── source_whitelist_rules（来源白名单配置，D4；POC 默认平台级，tenant_id 可空）──
CREATE TABLE IF NOT EXISTS source_whitelist_rules (
  whitelist_rule_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID REFERENCES tenants(tenant_id),       -- 可空 = 平台级全局规则（POC 默认）
  source_identifier TEXT NOT NULL,                    -- 域名 / 来源标识
  is_allowed BOOLEAN NOT NULL DEFAULT TRUE,
  authorization_note TEXT NOT NULL DEFAULT '',        -- 授权说明
  scope TEXT NOT NULL DEFAULT 'platform' CHECK (scope IN ('platform', 'tenant')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, source_identifier)
);

-- ── kb_documents（知识库内文档 + §11.5.1 导入 10 必录字段 + D4 状态机 + D3 staging 隔离）──
CREATE TABLE IF NOT EXISTS kb_documents (
  kb_document_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  kb_id UUID NOT NULL REFERENCES knowledge_bases(kb_id) ON DELETE CASCADE,
  document_id UUID REFERENCES documents(document_id),  -- 关联 c01 documents；staging/预览期可空
  -- §11.5.1 导入 10 必录字段槽位 ──
  source_url TEXT NOT NULL DEFAULT '',                 -- ① 来源 URL / 文件来源
  source_type TEXT NOT NULL,                           -- ② 来源类型：upload / url / pubmed / pmc / whitelist
  imported_by UUID REFERENCES users(user_id),          -- ③ 导入人
  imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),      -- ④ 导入时间
  copyright_status TEXT NOT NULL,                      -- ⑤ 版权 / 授权状态
  source_version TEXT NOT NULL DEFAULT 'v1',           -- ⑥ 版本
  parse_status TEXT NOT NULL DEFAULT 'pending'         -- ⑦ 解析状态：待解析/解析中/解析完成/失败
    CHECK (parse_status IN ('pending', 'parsing', 'parsed', 'failed')),
  index_status TEXT NOT NULL DEFAULT 'pending'         -- ⑧ 索引状态：待索引/索引中/索引完成/失败
    CHECK (index_status IN ('pending', 'indexing', 'indexed', 'failed')),
  whitelist_rule_id UUID REFERENCES source_whitelist_rules(whitelist_rule_id), -- ⑨ 命中白名单规则 ID（条件填充）
  authorized_by UUID REFERENCES users(user_id),        -- ⑩ 授权确认人（条件填充）
  -- D4 授权状态机 + D3 staging 物理隔离 ──
  authorization_status TEXT NOT NULL DEFAULT 'pending_preview'
    CHECK (authorization_status IN ('pending_preview', 'authorized', 'preview_only', 'rejected')),
  is_staging BOOLEAN NOT NULL DEFAULT TRUE,            -- 暂存预览=true，确认入正式库=false（D3 物理隔离）
  title TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_documents_kb ON kb_documents (tenant_id, kb_id);
-- 正式库已索引文档计数（document_count 刷新源）：仅计入正式（非 staging）且 indexed
CREATE INDEX IF NOT EXISTS idx_kb_documents_indexed
  ON kb_documents (kb_id) WHERE index_status = 'indexed' AND is_staging = FALSE;
CREATE INDEX IF NOT EXISTS idx_kb_documents_document ON kb_documents (document_id);
