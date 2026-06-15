-- c04-aimed-rag-citation｜AIMed 会话/消息/引用/Agent 追踪/反馈（PRD §18 命名）
-- owner=c04（唯一建表方）：conversations / messages / citations / agent_runs / agent_steps / tool_calls / feedbacks
-- 本 phase 不建任何上游表：documents/document_versions(c01)、document_chunks/embeddings/document_parse_jobs(c03)、
--   knowledge_bases/kb_documents(c06)、recent_tasks/audit_logs(c01)、privacy_redaction_events(c09) 均由各自 owner 所建，本 phase 仅消费/写入。
-- agent_checkpoints V1.0 不建（§18 注记：仅 V1.1 长任务断点续跑预留）。

-- AIMed/kb_qa 会话（design D1 / Decision B）。module 枚举恰为 {aimed, kb_qa}，MUST NOT 含 translation。
CREATE TABLE IF NOT EXISTS conversations (
  conversation_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  user_id UUID NOT NULL REFERENCES users(user_id),
  -- 区分 AIMed 与 c06 知识库问答会话；c06 经本表接口写 kb_qa（不另建会话表）
  module TEXT NOT NULL DEFAULT 'aimed' CHECK (module IN ('aimed', 'kb_qa')),
  -- 来源规范值：'AIMed 学术助手' / '医疗知识库问答'
  source TEXT NOT NULL,
  -- AIMed 六模式枚举之一（kb_qa 下不强制取六模式枚举）
  mode TEXT,
  title TEXT NOT NULL DEFAULT '新会话',
  -- 数据源约束（§8.2 mode_policy 的会话级快照）。allow_kb 仅 general=TRUE
  allow_pubmed BOOLEAN NOT NULL DEFAULT TRUE,
  allow_upload BOOLEAN NOT NULL DEFAULT TRUE,
  allow_kb BOOLEAN NOT NULL DEFAULT FALSE,
  allow_current_doc BOOLEAN NOT NULL DEFAULT FALSE,
  -- 会话级文件清单（轻量缓存；不替代 documents 持久化）
  uploaded_files JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_conversations_tenant_user
  ON conversations (tenant_id, user_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_module
  ON conversations (tenant_id, module);

-- 消息：用户输入与每个答案版本各一条。parent_message_id 构成重新生成版本链。
CREATE TABLE IF NOT EXISTS messages (
  message_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  conversation_id UUID NOT NULL REFERENCES conversations(conversation_id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  user_id UUID NOT NULL REFERENCES users(user_id),
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
  content TEXT NOT NULL DEFAULT '',
  mode TEXT,
  parent_message_id UUID REFERENCES messages(message_id),
  -- §24.7：AI 生成内容标记草稿/辅助建议（owner=c09，本 phase 消费、随消息保留）；思考秒数/统计/免责声明等
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation
  ON messages (conversation_id, created_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_messages_tenant
  ON messages (tenant_id);

-- 引用溯源（design D5）。每条绑定 message_id + source_type 三态定位指针。
CREATE TABLE IF NOT EXISTS citations (
  citation_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  message_id UUID NOT NULL REFERENCES messages(message_id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  cite_index INTEGER NOT NULL,                 -- 角标序号 [n]
  source_type TEXT NOT NULL CHECK (source_type IN ('pubmed', 'upload', 'kb')),
  -- pubmed 定位
  pubmed_id TEXT,
  doi TEXT,
  source_url TEXT,
  -- upload 定位
  document_id UUID,
  page INTEGER,
  paragraph_index INTEGER,
  section TEXT,
  -- kb 定位
  kb_id UUID,
  chunk_id UUID,
  -- 渲染字段
  source_title TEXT,
  journal TEXT,
  year INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (message_id, cite_index)
);
CREATE INDEX IF NOT EXISTS idx_citations_message
  ON citations (message_id);

-- AIMed/RAG 内部 AI 任务追踪（design D3/D7）：一次 RAG 编排=一条 agent_run、每节点一条 agent_step。
CREATE TABLE IF NOT EXISTS agent_runs (
  run_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  user_id UUID NOT NULL REFERENCES users(user_id),
  conversation_id UUID REFERENCES conversations(conversation_id) ON DELETE SET NULL,
  message_id UUID REFERENCES messages(message_id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ended_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_agent_runs_tenant
  ON agent_runs (tenant_id, started_at DESC);

CREATE TABLE IF NOT EXISTS agent_steps (
  step_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES agent_runs(run_id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  step_name TEXT NOT NULL,
  input_summary TEXT,
  output_summary TEXT,
  metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_steps_run
  ON agent_steps (run_id, created_at);

CREATE TABLE IF NOT EXISTS tool_calls (
  tool_call_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  step_id UUID NOT NULL REFERENCES agent_steps(step_id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  tool_name TEXT NOT NULL,
  args JSONB NOT NULL DEFAULT '{}'::jsonb,
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tool_calls_step
  ON tool_calls (step_id);

-- feedbacks 多来源泛化（design D8）：subject_type∈{message,translation_job}，subject_id 承载 message_id 或 translation_jobs.job_id。
-- subject_id 故意无 FK（translation_jobs 归 c07，跨 phase 多态）。c07 仅写入、不建表、不 ALTER。
CREATE TABLE IF NOT EXISTS feedbacks (
  feedback_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  user_id UUID NOT NULL REFERENCES users(user_id),
  subject_type TEXT NOT NULL CHECK (subject_type IN ('message', 'translation_job')),
  subject_id UUID NOT NULL,
  rating TEXT NOT NULL,        -- '赞' / '踩' 或翻译质量评分
  reason TEXT,                 -- §8.10.5 踩原因 7 项之一 ∪ 翻译质量维度
  comment TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_feedbacks_subject
  ON feedbacks (tenant_id, subject_type, subject_id);
