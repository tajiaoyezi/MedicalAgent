-- c05 ai-panel-recent-tasks：写回确认记录表（新建）+ recent_tasks 补列（ALTER，不重复建表）。
-- writeback_confirmations 与 risk_type 分类器唯一 owner=c05；c09 引用式消费做统一验收/审计，不新造独立 confirmation 表。
-- recent_tasks 唯一建表 owner=c01（001_initial.sql），本期仅 ADD COLUMN（UNIQUE(tenant_id,user_id,ref_type,ref_id) 与
-- idx_recent_tasks_user_updated 排序索引已由 001 建好，本期不重复创建）。

-- ── writeback_confirmations（§19.2 全字段 + doc_ai §6.6 恢复载体列）──
CREATE TABLE IF NOT EXISTS writeback_confirmations (
  confirmation_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  -- subject 多态键：document / message / translation_job，subject_id 承载
  -- document_id / messages.message_id / translation_jobs.job_id（与 §19.2「document_id / message_id」对齐并泛化覆盖译文文书）
  subject_type TEXT NOT NULL CHECK (subject_type IN ('document', 'message', 'translation_job')),
  subject_id UUID NOT NULL,
  confirmed_by UUID NOT NULL REFERENCES users(user_id),
  -- 高风险确认角色，取值 ∈ {doctor, reviewer}（c01 auth-rbac 唯一真值）；非高风险确认可为 NULL
  confirmed_role TEXT CHECK (confirmed_role IS NULL OR confirmed_role IN ('doctor', 'reviewer')),
  confirmed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  -- 本次操作选区/影响范围定位（承载 §6.6 doc_ai「选区」）
  confirmed_scope TEXT,
  -- 高风险命中类别摘要（命中诊疗/用药/医嘱/临床文书/患者个体信息）；非高风险为 NULL
  risk_type TEXT,
  before_content_hash TEXT,
  after_content_hash TEXT,
  -- 确认动作：apply（应用到文档）/ copy（生成副本）/ submit_review（提交审核）/ dispatch（message/译文下发）
  confirmation_action TEXT NOT NULL CHECK (confirmation_action IN ('apply', 'copy', 'submit_review', 'dispatch')),
  audit_log_id UUID,
  -- doc_ai §6.6 恢复载体：AI 操作类型 + 输出结果版本
  operation_type TEXT CHECK (
    operation_type IS NULL OR operation_type IN (
      '全文润色', '选区润色', '校对', '选区翻译', '补引用', '插入标注', 'AI 论文排版'
    )
  ),
  output_version_id UUID REFERENCES document_versions(version_id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 多态 subject 回源查询（恢复编排按 subject_type+subject_id 取确认记录）
CREATE INDEX IF NOT EXISTS idx_writeback_confirmations_subject
  ON writeback_confirmations (tenant_id, subject_type, subject_id);
-- doc_ai 最近任务回源（recent_tasks.ref_id=writeback_ref=confirmation_id）
CREATE INDEX IF NOT EXISTS idx_writeback_confirmations_tenant_created
  ON writeback_confirmations (tenant_id, created_at DESC);

-- ── recent_tasks 补列（ALTER 补 D4 所需列；建表/唯一约束/排序索引归 c01）──
ALTER TABLE recent_tasks ADD COLUMN IF NOT EXISTS title_preview TEXT;
ALTER TABLE recent_tasks ADD COLUMN IF NOT EXISTS status TEXT;
ALTER TABLE recent_tasks ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE recent_tasks ADD COLUMN IF NOT EXISTS related_document_id UUID;
