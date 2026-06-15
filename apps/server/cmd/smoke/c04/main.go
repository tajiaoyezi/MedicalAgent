// c04 端到端验收冒烟。需 docker PG+MinIO 已起且已 migrate（007）。内置智能 mock 模型（chat 改写回显 / embed / rerank）。
// 公网路径本期默认关闭（c09 未接入）；PubMed 走离线缓存闭环。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/aimed"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/citation"
	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/model"
	"medoffice/server/internal/parsing"
	"medoffice/server/internal/pubmed"
	"medoffice/server/internal/rag"
	"medoffice/server/internal/storage"
)

const port = 4734

var mockBase = fmt.Sprintf("http://127.0.0.1:%d", port)

func okAssert(cond bool, msg string) {
	if !cond {
		log.Fatalf("断言失败: %s", msg)
	}
	fmt.Println("  ✓", msg)
}

func startMock() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		send := func(code int, obj any) {
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(code)
			b, _ := json.Marshal(obj)
			w.Write(b)
		}
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/fail"):
			send(500, map[string]any{"error": "mock failure"})
		case strings.HasSuffix(p, "/v1/chat/completions"):
			// 改写提示回显原查询，保证检索词干净；答案提示返回简短 prose
			last := lastUserContent(body)
			content := "[mock-answer] 基于检索证据的分析。"
			if strings.HasPrefix(last, "改写") {
				if i := strings.LastIndex(last, "："); i >= 0 {
					content = strings.TrimSpace(last[i+len("："):])
				}
			}
			send(200, map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
		case strings.HasSuffix(p, "/v1/embeddings"):
			input, _ := body["input"].([]any)
			data := make([]any, 0, len(input))
			for range input {
				data = append(data, map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
			}
			send(200, map[string]any{"model": "mock-embed", "data": data})
		case strings.HasSuffix(p, "/v1/rerank"):
			docs, _ := body["documents"].([]any)
			scores := make([]float64, 0, len(docs))
			for i := range docs {
				scores = append(scores, 1-float64(i)*0.05)
			}
			send(200, map[string]any{"scores": scores})
		default:
			send(404, map[string]any{"error": "not found"})
		}
	})
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Fatalf("mock listen: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return srv
}

func lastUserContent(body map[string]any) string {
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		return ""
	}
	m, _ := msgs[len(msgs)-1].(map[string]any)
	s, _ := m["content"].(string)
	return s
}

func intPtr(i int) *int { return &i }

func makeTextDoc(gormDB *gorm.DB, store *storage.Storage, tenantID, ownerID, name, content string) string {
	documentID := uuid.NewString()
	versionID := uuid.NewString()
	objectKey := storage.ObjectKeyForVersion(tenantID, documentID, versionID)
	if err := store.Put(context.Background(), objectKey, []byte(content), "text/markdown"); err != nil {
		log.Fatalf("storage.put: %v", err)
	}
	gormDB.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, mime_type, current_version_id) VALUES (?,?,?,?,'my','text/markdown',NULL)`,
		documentID, tenantID, ownerID, name)
	fileHash := strings.ReplaceAll(uuid.NewString(), "-", "")
	gormDB.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes) VALUES (?,?,?,1,?,?,'import',?,?)`,
		versionID, documentID, tenantID, fileHash, ownerID, objectKey, len(content))
	gormDB.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, versionID, documentID)
	gormDB.Exec(`INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload) VALUES ('upload_success', ?, ?, ?, '{"source":"c04-smoke"}'::jsonb)`,
		documentID, versionID, tenantID)
	return documentID
}

// insertChunk 直接写 chunk + embedding（绕过解析，用于确定性权限/检索测试）。
func insertChunk(gormDB *gorm.DB, tenantID, docID string, version int, sourceType, title, text, acl string, page *int) {
	var chunkID string
	gormDB.Raw(`INSERT INTO document_chunks (tenant_id, document_id, document_version, source_type, source_title, section, page, paragraph_index, chunk_text, chunk_acl)
		VALUES (?,?,?,?,?,'Methods',?,1,?,?::jsonb) RETURNING id`,
		tenantID, docID, version, sourceType, title, page, text, acl).Scan(&chunkID)
	gormDB.Exec(`INSERT INTO embeddings (chunk_id, vector, model, dim) VALUES (?, '[0.1,0.2,0.3]'::jsonb, 'mock', 3)`, chunkID)
}

func cleanup(gormDB *gorm.DB, tenantID string) {
	// 会话/消息/引用/反馈/agent
	var convs []string
	gormDB.Raw(`SELECT conversation_id FROM conversations WHERE tenant_id = ? AND (title LIKE 'c04-smoke%' OR source = 'AIMed 学术助手')`, tenantID).Scan(&convs)
	for _, cv := range convs {
		var msgs []string
		gormDB.Raw(`SELECT message_id FROM messages WHERE conversation_id = ?`, cv).Scan(&msgs)
		for _, m := range msgs {
			gormDB.Exec(`DELETE FROM feedbacks WHERE subject_id = ?`, m)
		}
		gormDB.Exec(`DELETE FROM agent_runs WHERE conversation_id = ?`, cv)
		gormDB.Exec(`DELETE FROM conversations WHERE conversation_id = ?`, cv) // 级联 messages → citations
	}
	gormDB.Exec(`DELETE FROM feedbacks WHERE tenant_id = ? AND subject_type = 'translation_job'`, tenantID)
	// 文档及解析产物
	var docs []string
	gormDB.Raw(`SELECT document_id FROM documents WHERE tenant_id = ? AND name LIKE 'c04-smoke%'`, tenantID).Scan(&docs)
	for _, id := range docs {
		gormDB.Exec(`DELETE FROM embeddings WHERE chunk_id IN (SELECT id FROM document_chunks WHERE document_id = ?)`, id)
		gormDB.Exec(`DELETE FROM document_chunks WHERE document_id = ?`, id)
		gormDB.Exec(`DELETE FROM document_event_consumptions WHERE event_id IN (SELECT event_id FROM document_events WHERE document_id = ?)`, id)
		gormDB.Exec(`DELETE FROM document_parse_jobs WHERE document_id = ?`, id)
		gormDB.Exec(`DELETE FROM recent_tasks WHERE ref_id::text = ? OR ref_id IN (SELECT conversation_id FROM conversations WHERE tenant_id = ?)`, id, tenantID)
		gormDB.Exec(`DELETE FROM document_events WHERE document_id = ?`, id)
		gormDB.Exec(`UPDATE documents SET current_version_id = NULL WHERE document_id = ?`, id)
		gormDB.Exec(`DELETE FROM document_versions WHERE document_id = ?`, id)
		gormDB.Exec(`DELETE FROM documents WHERE document_id = ?`, id)
	}
	gormDB.Exec(`DELETE FROM recent_tasks WHERE tenant_id = ? AND source = 'AIMed 学术助手'`, tenantID)
	// providers
	gormDB.Exec(`DELETE FROM model_routes WHERE tenant_id = ? AND provider_id IN (SELECT provider_id FROM model_providers WHERE name LIKE 'c04-smoke%')`, tenantID)
	gormDB.Exec(`DELETE FROM model_providers WHERE tenant_id = ? AND name LIKE 'c04-smoke%'`, tenantID)
}

func main() {
	cfg := config.Load()
	model.Init(cfg.Model.CredentialSecret, cfg.Model.HealthTTLSeconds)
	mock := startMock()
	defer mock.Close()
	gormDB, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	store, err := storage.New(context.Background(), cfg.Storage)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	parseEngine := parsing.NewEngine(store)

	var tenantID string
	gormDB.Raw(`SELECT tenant_id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenantID)
	if tenantID == "" {
		log.Fatal("无租户，请先 migrate")
	}
	var adminID, userID string
	gormDB.Raw(`SELECT user_id FROM users WHERE tenant_id = ? AND username = 'admin'`, tenantID).Scan(&adminID)
	gormDB.Raw(`SELECT user_id FROM users WHERE tenant_id = ? AND username = 'user'`, tenantID).Scan(&userID)
	adminUser, _ := auth.LoadUserByID(gormDB, adminID)
	if adminUser == nil {
		log.Fatal("无 admin 用户")
	}

	cleanup(gormDB, tenantID)

	// PubMed 离线 + RAG 引擎 + 索引就绪订阅
	pubSvc := pubmed.NewService(pubmed.NewOnlineProvider("", 2*time.Second), pubmed.NewOfflineProvider(), false)
	ragEngine := rag.NewEngine(pubSvc)
	rag.RegisterIndexConsumer(gormDB)
	aimedSvc := aimed.NewService(ragEngine)
	ictx := model.InvokeContext{TenantID: tenantID, ActorID: adminID, ActorRole: "admin"}

	// ---- [1] 六模式 policy / 占位文案 / 发送状态机 ----
	fmt.Println("\n[1] 六模式数据源约束 / 占位文案 / 发送按钮状态机（§8.2/§8.4/§8.5）")
	okAssert(aimed.GetPolicy(aimed.ModeGeneral).AllowKB && !aimed.GetPolicy(aimed.ModeReviewGen).AllowKB,
		"allow_kb 仅 general=✓、智能综述生成=✗（医疗知识库维）")
	okAssert(!aimed.GetPolicy(aimed.ModeDeepReading).AllowPubmed && aimed.GetPolicy(aimed.ModeDeepReading).UploadRequired,
		"深度文献伴读禁连 PubMed 且强制上传")
	okAssert(aimed.GetPolicy(aimed.ModeEvidenceTracing).Placeholder == "请输入一个临床结论或医学问题，我将为您追溯证据。",
		"循证证据溯源占位文案与 §8.4 一致")
	okAssert(!aimed.GetPolicy(aimed.ModeDeepReading).ShowPubmedTag() && aimed.GetPolicy(aimed.ModeGeneral).ShowPubmedTag(),
		"深度文献伴读隐藏 PubMed 标签、其他模式显示")
	okAssert(!aimed.CanSend(aimed.ModeDeepReading, "解读这篇", nil).CanSend, "深度文献伴读未上传文件置灰")
	okAssert(aimed.CanSend(aimed.ModeGeneral, "什么是肺癌", nil).CanSend, "通用问答有效文本高亮")
	okAssert(!aimed.CanSend(aimed.ModeTrendAnalysis, "   ", nil).CanSend, "纯空格视为空置灰")
	okAssert(!aimed.CanSend(aimed.ModeGeneral, "x", []aimed.FileState{{Status: aimed.FileParsing}}).CanSend, "存在解析中文件禁止发送")

	// ---- [2] 模式切换规则 ----
	fmt.Println("\n[2] 模式切换规则（§8.3）")
	convID, _ := aimed.CreateConversation(gormDB, tenantID, adminID, aimed.ModuleAimed, string(aimed.ModeGeneral), "c04-smoke 切换会话")
	_ = aimed.SetUploadedFiles(gormDB, tenantID, adminID, convID, []aimed.UploadedFile{{FileID: "f1", Name: "x.pdf", Status: aimed.FileParsed, DocumentID: uuid.NewString()}})
	convTrend, _ := aimed.SwitchMode(gormDB, tenantID, adminID, convID, aimed.ModeTrendAnalysis)
	okAssert(len(convTrend.Files()) == 0, "切换到科研态势分析清空已上传文件")
	convDeep, _ := aimed.SwitchMode(gormDB, tenantID, adminID, convID, aimed.ModeDeepReading)
	okAssert(!aimed.GetPolicy(aimed.Mode(convDeep.Mode)).ShowPubmedTag(), "切换到深度文献伴读隐藏 PubMed 标签")

	// ---- [3] 智能模式匹配 ----
	fmt.Println("\n[3] 智能模式匹配（§8.11，仅高亮不强制切换）")
	m1 := aimed.Evaluate(aimed.ModeGeneral, "请用 RCT、Meta 分析验证这个临床结论")
	okAssert(m1.Recommended == aimed.ModeEvidenceTracing && m1.Highlight && m1.Guidance == "检索并验证临床结论，获取高级别证据",
		"关键词触发推荐循证证据溯源并展示引导文案、不自动切换")
	m2 := aimed.Evaluate(aimed.ModeGeneral, "请逐段精读这篇文献，并润色扩写其中的段落")
	okAssert(m2.Recommended == aimed.ModeDeepReading, "多模式命中按优先级取最高（深度文献伴读）")
	okAssert(m2.Compound && len(m2.CompoundSteps) >= 2, "复合任务提示分步操作建议")
	m3 := aimed.Evaluate(aimed.ModeGeneral, "今天天气怎么样，推荐个好吃的餐厅")
	okAssert(m3.Refusal == aimed.RefusalText, "通用问答拒答无关问题（固定文案）")

	// ---- [4] PubMed 在线/离线双路径 + 授权标记 ----
	fmt.Println("\n[4] PubMed 离线缓存闭环 + 取数授权标记（§16.1/§24.1）")
	ps, route, _ := pubSvc.Search(gormDB, ictx, "肺癌 免疫治疗", 5)
	okAssert(route == "offline" && len(ps) >= 1 && ps[0].PubmedID != "", "公网默认关闭→离线检索返回带 PMID 的可溯源条目")
	doiSrc, _ := pubSvc.ImportByID(gormDB, ictx, "doi", "10.1000/lung-immuno-2021")
	okAssert(doiSrc.AuthStatus == pubmed.AuthAuthorized, "白名单 DOI 取数标记 authorized")
	urlSrc, _ := pubSvc.ImportByID(gormDB, ictx, "url", "https://research.example.com/x")
	okAssert(urlSrc.AuthStatus == pubmed.AuthPreviewOnly, "非白名单 URL 标记 preview_only")
	bizSrc, _ := pubSvc.ImportByID(gormDB, ictx, "url", "https://www.cnki.net/article/1")
	okAssert(bizSrc.AuthStatus == pubmed.AuthRejected, "未授权商业库（知网）标记 rejected、不默认抓取")
	// 公网开启但脱敏门禁默认拒绝 → 仍降级离线 + 审计
	pubGated := pubmed.NewService(pubmed.NewOnlineProvider("", 1*time.Second), pubmed.NewOfflineProvider(), true)
	_, route2, _ := pubGated.Search(gormDB, ictx, "含潜在 PHI 的查询", 3)
	okAssert(route2 == "offline", "公网开启但脱敏门禁默认拒绝→降级离线（§16.4 保守降级）")
	okAssert(auditCount(gormDB, tenantID, "pubmed_redaction_block", time.Time{}) >= 1, "脱敏门禁拒绝落 pubmed_redaction_block 审计")

	// ---- [5] 索引就绪事件 → 构建检索索引（真实解析链路） ----
	fmt.Println("\n[5] 消费 c03『索引就绪』事件构建检索索引（§16.2，design D8）")
	privEmbed, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c04-smoke 私有embed", Protocol: "openai_compat", DeploymentKind: "private",
		BaseURL: mockBase + "/ok", Model: "mock-embed", NetworkPolicy: "intranet_only", Enabled: true,
	})
	model.BindRoute(gormDB, tenantID, "embed", privEmbed, 1)
	uploadDoc := makeTextDoc(gormDB, store, tenantID, adminID, "c04-smoke-肺癌.md",
		"# 肺癌免疫治疗\n\nPD-1 检查点抑制剂在非小细胞肺癌中显著改善总生存期，证据来自随机对照试验。")
	_, dispatched, _ := parseEngine.ParseTick(gormDB)
	okAssert(dispatched >= 1, fmt.Sprintf("upload_success 事件被消费创建解析作业（dispatched=%d）", dispatched))
	okAssert(rag.Default().Version(uploadDoc) == 1, "索引就绪事件触发内存检索索引构建（version=1）")
	// 幂等：重复装载不改变版本
	_ = rag.Default().IndexDocument(gormDB, uploadDoc, 1)
	okAssert(rag.Default().Version(uploadDoc) == 1, "重复装载按 (doc,version) 幂等")

	// ---- [6] RAG 检索六维权限过滤 + RRF/model rerank ----
	fmt.Println("\n[6] RAG 六维权限过滤 + rerank/RRF（§11.9，4.3/4.6）")
	chatProv, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c04-smoke 私有chat", Protocol: "openai_compat", DeploymentKind: "private",
		BaseURL: mockBase + "/ok", Model: "mock-llm", NetworkPolicy: "intranet_only", Enabled: true,
	})
	model.BindRoute(gormDB, tenantID, "chat", chatProv, 1)

	// 越权 chunk（chunk_acl deny admin）+ 越权文档（user 拥有、admin 无权）
	denyDoc := makeTextDoc(gormDB, store, tenantID, adminID, "c04-smoke-deny.md", "占位")
	insertChunk(gormDB, tenantID, denyDoc, 1, "upload", "c04-smoke-deny", "肺癌 免疫治疗 越权 chunk 内容", `{"deny_users":["`+adminID+`"]}`, intPtr(2))
	_ = rag.Default().IndexDocument(gormDB, denyDoc, 1)
	userDoc := makeTextDoc(gormDB, store, tenantID, userID, "c04-smoke-userdoc.md", "占位")
	insertChunk(gormDB, tenantID, userDoc, 1, "upload", "c04-smoke-userdoc", "肺癌 免疫治疗 他人文档内容", `{}`, intPtr(3))
	_ = rag.Default().IndexDocument(gormDB, userDoc, 1)

	// RRF 兜底（rerank 路由未绑定）
	rrfRes, _ := ragEngine.Retrieve(gormDB, rag.RetrieveRequest{
		User: *adminUser, ModeLabel: "通用问答", AllowPubmed: true, AllowUpload: true, AllowKB: false,
		Query: "肺癌 免疫治疗", UploadDocIDs: []string{uploadDoc, denyDoc, userDoc}, TopN: 6,
	})
	okAssert(rrfRes.RerankMethod == "rrf", "rerank 不可用时 RRF 兜底排序")
	okAssert(rrfRes.KeyCount >= 1, "检索命中并注入候选")
	for _, c := range rrfRes.Candidates {
		okAssert(c.DocumentID != denyDoc, "越权 chunk 被 chunk_acl 维过滤、不出现在结果")
		okAssert(c.DocumentID != userDoc, "越权文档被 document_acl 维过滤、不出现在结果")
	}
	hasUpload, hasPubmed := false, false
	for _, c := range rrfRes.Candidates {
		if c.SourceType == "upload" && c.DocumentID == uploadDoc {
			hasUpload = true
			okAssert(c.Page != nil || c.ParagraphIndex != nil || c.ChunkID != "", "上传文件 chunk 携带溯源定位字段")
		}
		if c.SourceType == "pubmed" && c.PubmedID != "" {
			hasPubmed = true
		}
	}
	okAssert(hasUpload && hasPubmed, "upload 与 PubMed 候选同构进入合并去重")

	// model rerank 路径
	rerankProv, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c04-smoke 私有rerank", Protocol: "openai_compat", DeploymentKind: "private",
		BaseURL: mockBase + "/ok", Model: "mock-rerank", NetworkPolicy: "intranet_only", Enabled: true,
	})
	model.BindRoute(gormDB, tenantID, "rerank", rerankProv, 1)
	modelRes, _ := ragEngine.Retrieve(gormDB, rag.RetrieveRequest{
		User: *adminUser, ModeLabel: "通用问答", AllowPubmed: true, AllowUpload: false, AllowKB: false,
		Query: "肺癌 免疫治疗", TopN: 5,
	})
	okAssert(modelRes.RerankMethod == "model", "reranker 可用时走模型 rerank")
	// 数据源约束：deep_reading 排除 PubMed/KB
	dataSrcRes, _ := ragEngine.Retrieve(gormDB, rag.RetrieveRequest{
		User: *adminUser, ModeLabel: "深度文献伴读", AllowPubmed: false, AllowUpload: true, AllowKB: false,
		Query: "肺癌 免疫治疗", UploadDocIDs: []string{uploadDoc}, TopN: 5,
	})
	noPubmed := true
	for _, c := range dataSrcRes.Candidates {
		if c.SourceType == "pubmed" {
			noPubmed = false
		}
	}
	okAssert(noPubmed && dataSrcRes.PubmedRoute == "none", "深度文献伴读数据源排除 PubMed")

	// ---- [7] 主问答闭环：带引用角标答案 + 免责声明 + 草稿 ----
	fmt.Println("\n[7] 主问答闭环：带引用角标答案 + 结构化参考 + 免责声明 + 草稿标记（§8.8/§24.7）")
	askConv, _ := aimed.CreateConversation(gormDB, tenantID, adminID, aimed.ModuleAimed, string(aimed.ModeGeneral), "c04-smoke 问答会话")
	conv, _ := aimed.GetConversation(gormDB, tenantID, adminID, askConv)
	_ = aimed.SetUploadedFiles(gormDB, tenantID, adminID, askConv, []aimed.UploadedFile{{FileID: uploadDoc, Name: "肺癌.md", Status: aimed.FileParsed, DocumentID: uploadDoc}})
	conv, _ = aimed.GetConversation(gormDB, tenantID, adminID, askConv)
	ans, aerr := aimedSvc.Answer(gormDB, aimed.AnswerRequest{User: *adminUser, Conversation: conv, Query: "肺癌 免疫治疗 的证据"})
	okAssert(aerr == nil && ans.MessageID != "", "问答产出答案消息")
	okAssert(len(ans.Citations) >= 1 && strings.Contains(ans.Content, "[1]"), "关键结论带 [n] 角标且 citations 一一对应")
	okAssert(strings.Contains(ans.Content, "参考资料"), "答案末尾结构化参考资料")
	okAssert(strings.Contains(ans.Content, aimed.DisclaimerText) && ans.Draft && ans.DraftLabel == aimed.DraftLabel, "答案含医疗免责声明 + 草稿/辅助建议标记")
	okAssert(ans.Stats.Found >= 1 && ans.Stats.Key >= 1, "检索统计：找到 N 篇 / M 篇重点参考")

	// 未检索到资料分支
	emptyConv, _ := aimed.CreateConversation(gormDB, tenantID, adminID, aimed.ModuleAimed, string(aimed.ModeDeepReading), "c04-smoke 空检索会话")
	ec, _ := aimed.GetConversation(gormDB, tenantID, adminID, emptyConv)
	emptyAns, _ := aimedSvc.Answer(gormDB, aimed.AnswerRequest{User: *adminUser, Conversation: ec, Query: "zzzz 不存在的检索词 qqqq"})
	okAssert(emptyAns.NoResults && strings.Contains(emptyAns.Content, aimed.NoResultsText), "未检索到资料返回固定提示、不输出无依据建议")

	// ---- [8] 引用溯源定位（三态 + 异常分支） ----
	fmt.Println("\n[8] 引用点击定位三态 + 异常分支（§8.9）")
	cites, _ := citation.ListByMessage(gormDB, tenantID, ans.MessageID)
	okAssert(len(cites) >= 1, "可按 message 取引用集合")
	var pubmedCite, uploadCite string
	for _, ct := range cites {
		if ct.SourceType == "pubmed" {
			pubmedCite = ct.CitationID
		}
		if ct.SourceType == "upload" {
			uploadCite = ct.CitationID
		}
	}
	if pubmedCite != "" {
		loc, _ := citation.Locate(gormDB, *adminUser, pubmedCite)
		okAssert(loc.OK && loc.Action == "open_pubmed", "点击 PubMed 角标打开文章详情")
	}
	if uploadCite != "" {
		loc, _ := citation.Locate(gormDB, *adminUser, uploadCite)
		okAssert(loc.OK, "点击上传文件角标定位来源（权限复核通过）")
	}
	// 原文已删除 → MsgDeleted
	delDoc := makeTextDoc(gormDB, store, tenantID, adminID, "c04-smoke-del.md", "x")
	gormDB.Exec(`UPDATE documents SET is_deleted = TRUE WHERE document_id = ?`, delDoc)
	delMsg, _ := aimed.AddMessage(gormDB, tenantID, adminID, askConv, "assistant", "x", "general", nil, nil)
	_ = citation.Save(gormDB, tenantID, delMsg, []citation.Input{{CiteIndex: 1, SourceType: "upload", DocumentID: delDoc, SourceTitle: "del", Page: intPtr(1)}})
	delCites, _ := citation.ListByMessage(gormDB, tenantID, delMsg)
	delLoc, _ := citation.Locate(gormDB, *adminUser, delCites[0].CitationID)
	okAssert(!delLoc.OK && delLoc.Message == citation.MsgDeleted, "原文已删除提示「该引用源已删除」")
	// 权限不足 → MsgUnavailable（user 拥有的文档，admin 无权）
	permMsg, _ := aimed.AddMessage(gormDB, tenantID, adminID, askConv, "assistant", "y", "general", nil, nil)
	_ = citation.Save(gormDB, tenantID, permMsg, []citation.Input{{CiteIndex: 1, SourceType: "upload", DocumentID: userDoc, SourceTitle: "u", Page: intPtr(1)}})
	permCites, _ := citation.ListByMessage(gormDB, tenantID, permMsg)
	permLoc, _ := citation.Locate(gormDB, *adminUser, permCites[0].CitationID)
	okAssert(!permLoc.OK && permLoc.Message == citation.MsgUnavailable, "权限不足提示「该引用源暂时不可用」、不暴露内容")
	okAssert(auditCount(gormDB, tenantID, "citation_click", time.Time{}) >= 2, "引用点击写入审计")

	// ---- [9] 答案落地：生成在线 Word（upload_success）+ 离线导出 + recent_tasks ----
	fmt.Println("\n[9] 答案落地：生成在线 Word（§10.6）/ 离线导出 / 最近任务（§8.10/§6.4）")
	land, lerr := aimedSvc.GenerateWord(gormDB, store, *adminUser, conv, ans.MessageID)
	okAssert(lerr == nil && land.DocumentID != "" && land.OpenInOO && land.ExpandPanel, "生成在线 Word 落库并返回 ONLYOFFICE 打开 + 展开面板信号")
	var genEvt int
	gormDB.Raw(`SELECT COUNT(*)::int FROM document_events WHERE document_id = ? AND event_type = 'upload_success'`, land.DocumentID).Scan(&genEvt)
	okAssert(genEvt == 1, "生成文档由 c01 创建入口产生 upload_success（唯一产生方=c01）")
	// 生成文档可被 c03 解析索引
	_, gd, _ := parseEngine.ParseTick(gormDB)
	okAssert(gd >= 1, "生成文档的 upload_success 被 c03 消费解析（可后续检索）")
	saveRes, _ := aimedSvc.SaveAs(gormDB, store, *adminUser, conv, aimed.ScopeCurrent, aimed.FormatMarkdown, ans.MessageID)
	okAssert(!saveRes.OpenInOO && saveRes.ExportText != "" && strings.HasSuffix(saveRes.Filename, ".md"), "保存为 Markdown 走离线导出（不依赖 ONLYOFFICE）")
	var rtCount int
	gormDB.Raw(`SELECT COUNT(*)::int FROM recent_tasks WHERE tenant_id = ? AND user_id = ? AND source = 'AIMed 学术助手' AND ref_type = 'conversation' AND ref_id = ? AND deleted_at IS NULL`, tenantID, adminID, askConv).Scan(&rtCount)
	okAssert(rtCount == 1, "保存内容写入最近任务（source=AIMed 学术助手、ref_type=conversation、幂等）")

	// ---- [10] 反馈 + 高风险前置 + module 枚举 ----
	fmt.Println("\n[10] 反馈 feedbacks / 高风险前置消费 / module 枚举（§8.10.5/§19.2/D1/D8）")
	_ = aimed.WriteFeedback(gormDB, tenantID, adminID, "message", ans.MessageID, "踩", "引用错误", "")
	var fbCount int
	gormDB.Raw(`SELECT COUNT(*)::int FROM feedbacks WHERE subject_type = 'message' AND subject_id = ? AND rating = '踩' AND reason = '引用错误'`, ans.MessageID).Scan(&fbCount)
	okAssert(fbCount == 1, "踩反馈按 subject_type=message 写入并含 §8.10.5 原因枚举")
	_ = aimed.WriteFeedback(gormDB, tenantID, adminID, "translation_job", uuid.NewString(), "4", "术语一致性", "")
	var trFb int
	gormDB.Raw(`SELECT COUNT(*)::int FROM feedbacks WHERE tenant_id = ? AND subject_type = 'translation_job'`, tenantID).Scan(&trFb)
	okAssert(trFb == 1, "feedbacks 泛化承载 c07 翻译质量反馈（subject_type=translation_job）")
	// 高风险：用药/剂量查询 + admin（无 highrisk:confirm）→ 需确认
	riskAns, _ := aimedSvc.Answer(gormDB, aimed.AnswerRequest{User: *adminUser, Conversation: conv, Query: "肺癌 免疫治疗 的推荐用药剂量与医嘱"})
	okAssert(riskAns.HighRisk && riskAns.RequiresConfirmation, "高风险答案对无 highrisk:confirm 用户需医生/审核确认后下发")
	okAssert(auditCount(gormDB, tenantID, "aimed_highrisk_gate", time.Time{}) >= 1, "高风险前置消费 c05 确认链路落审计（subject=message）")
	doctorUser := auth.AuthUser{UserID: adminID, TenantID: tenantID, RoleSlugs: []string{"doctor"}, Permissions: []string{"highrisk:confirm"}}
	okAssert(aimed.CanConfirmHighRisk(doctorUser) && !aimed.CanConfirmHighRisk(*adminUser), "具备 highrisk:confirm（doctor/reviewer）方可确认，普通用户不可")
	// module 枚举不含 translation
	merr := gormDB.Exec(`INSERT INTO conversations (tenant_id, user_id, module, source) VALUES (?, ?, 'translation', 'x')`, tenantID, adminID).Error
	okAssert(merr != nil, "conversations.module 枚举拒绝 translation 取值（CHECK 约束）")

	// ---- [11] 结构校验 ----
	fmt.Println("\n[11] 表结构校验（owner=c04 七表 / agent_checkpoints 不建）")
	okAssert(hasColumn(gormDB, "conversations", "module") && hasColumn(gormDB, "conversations", "source"), "conversations 含 module/source 区分维（kb_qa 复用基座）")
	okAssert(hasColumn(gormDB, "feedbacks", "subject_type") && hasColumn(gormDB, "feedbacks", "subject_id"), "feedbacks 多态 subject_type/subject_id")
	okAssert(!tableExists(gormDB, "agent_checkpoints"), "agent_checkpoints 本期不建（V1.1 预留）")
	for _, t := range []string{"conversations", "messages", "citations", "agent_runs", "agent_steps", "tool_calls", "feedbacks"} {
		okAssert(tableExists(gormDB, t), "owner=c04 表存在："+t)
	}

	cleanup(gormDB, tenantID)
	fmt.Println("\n✅ c04 冒烟全部通过")
}

func auditCount(gormDB *gorm.DB, tenantID, actionType string, _ time.Time) int {
	var n int
	gormDB.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id = ? AND action_type = ?`, tenantID, actionType).Scan(&n)
	return n
}

func hasColumn(gormDB *gorm.DB, table, col string) bool {
	var n int
	gormDB.Raw(`SELECT COUNT(*)::int FROM information_schema.columns WHERE table_name = ? AND column_name = ?`, table, col).Scan(&n)
	return n > 0
}

func tableExists(gormDB *gorm.DB, table string) bool {
	var n int
	gormDB.Raw(`SELECT COUNT(*)::int FROM information_schema.tables WHERE table_name = ?`, table).Scan(&n)
	return n > 0
}
