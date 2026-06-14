// c03 端到端验收冒烟。需 docker PG+MinIO 已起且已 migrate。内置 mock 模型/视觉服务（私有化/本地）。
// 等价 apps/api/src/scripts/c03-smoke.ts。公网路径本期默认拒绝（c09 未接入），不验证公网放行。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/model"
	"medoffice/server/internal/parsing"
	"medoffice/server/internal/storage"
)

const port = 4733

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
			send(200, map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "[mock-chat] ok"}}}})
		case strings.HasSuffix(p, "/v1/messages"):
			send(200, map[string]any{"content": []any{map[string]any{"text": "[mock-anthropic] ok"}}})
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
				scores = append(scores, 1-float64(i)*0.1)
			}
			send(200, map[string]any{"scores": scores})
		case strings.HasSuffix(p, "/invoke"):
			if body["capability"] == "embed" {
				input, _ := body["input"].([]any)
				vectors := make([]any, 0, len(input))
				for range input {
					vectors = append(vectors, []float64{0.4, 0.5, 0.6})
				}
				send(200, map[string]any{"vectors": vectors})
				return
			}
			send(200, map[string]any{"content": "[mock-thirdparty] ok"})
		case strings.HasSuffix(p, "/parse"):
			send(200, map[string]any{
				"confidence": 0.95,
				"pages": []any{map[string]any{
					"page": 1, "confidence": 0.95,
					"paragraphs": []any{
						map[string]any{"text": "扫描件标题段落", "heading_level": 1},
						map[string]any{"text": "扫描件正文第一段，含可溯源页码。"},
					},
					"tables": []any{map[string]any{"rows": 2, "cols": 2}},
					"images": []any{map[string]any{"bbox": []int{0, 0, 10, 10}}},
				}},
			})
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

func cleanup(db *gorm.DB, tenantID string) {
	var docs []string
	db.Raw(`SELECT document_id FROM documents WHERE tenant_id = ? AND name LIKE 'c03-smoke%'`, tenantID).Scan(&docs)
	for _, id := range docs {
		db.Exec(`DELETE FROM embeddings WHERE chunk_id IN (SELECT id FROM document_chunks WHERE document_id = ?)`, id)
		db.Exec(`DELETE FROM document_chunks WHERE document_id = ?`, id)
		db.Exec(`DELETE FROM document_visual_parse_results WHERE document_id = ?`, id)
		db.Exec(`DELETE FROM document_event_consumptions WHERE event_id IN (SELECT event_id FROM document_events WHERE document_id = ?)`, id)
		db.Exec(`DELETE FROM document_parse_jobs WHERE document_id = ?`, id)
		db.Exec(`DELETE FROM document_events WHERE document_id = ?`, id)
		db.Exec(`UPDATE documents SET current_version_id = NULL WHERE document_id = ?`, id)
		db.Exec(`DELETE FROM document_versions WHERE document_id = ?`, id)
		db.Exec(`DELETE FROM documents WHERE document_id = ?`, id)
	}
	db.Exec(`DELETE FROM model_routes WHERE tenant_id = ? AND provider_id IN (SELECT provider_id FROM model_providers WHERE name LIKE 'c03-smoke%')`, tenantID)
	db.Exec(`DELETE FROM model_providers WHERE tenant_id = ? AND name LIKE 'c03-smoke%'`, tenantID)
	db.Exec(`DELETE FROM visual_parse_providers WHERE tenant_id = ? AND name LIKE 'c03-smoke%'`, tenantID)
}

func makeDocument(db *gorm.DB, store *storage.Storage, tenantID, ownerID, name, mime string, content []byte) string {
	documentID := uuid.NewString()
	versionID := uuid.NewString()
	objectKey := storage.ObjectKeyForVersion(tenantID, documentID, versionID)
	if err := store.Put(context.Background(), objectKey, content, mime); err != nil {
		log.Fatalf("storage.put: %v", err)
	}
	db.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, mime_type, current_version_id) VALUES (?,?,?,?,'my',?,NULL)`,
		documentID, tenantID, ownerID, name, mime)
	fileHash := strings.ReplaceAll(uuid.NewString(), "-", "")
	db.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes) VALUES (?,?,?,1,?,?,'import',?,?)`,
		versionID, documentID, tenantID, fileHash, ownerID, objectKey, len(content))
	db.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, versionID, documentID)
	db.Exec(`INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload) VALUES ('upload_success', ?, ?, ?, '{"source":"c03-smoke"}'::jsonb)`,
		documentID, versionID, tenantID)
	return documentID
}

func auditCount(db *gorm.DB, tenantID, actionType string, since time.Time) int {
	var n int
	db.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id = ? AND action_type = ? AND created_at >= ?`, tenantID, actionType, since).Scan(&n)
	return n
}

func intPtr(i int) *int { return &i }

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
	engine := parsing.NewEngine(store)
	since := time.Now()

	var tenantID string
	gormDB.Raw(`SELECT tenant_id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenantID)
	if tenantID == "" {
		log.Fatal("无租户，请先 migrate")
	}
	var adminID string
	gormDB.Raw(`SELECT user_id FROM users WHERE tenant_id = ? AND username = 'admin'`, tenantID).Scan(&adminID)
	ctx := model.InvokeContext{TenantID: tenantID, ActorID: adminID, ActorRole: "admin"}

	cleanup(gormDB, tenantID)

	fmt.Println("\n[1] 私有化 provider 主动连通性测试（5.x）")
	privChat, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c03-smoke 私有chat", Protocol: "openai_compat", DeploymentKind: "private",
		BaseURL: mockBase + "/ok", Credential: "sk-mock-private", Model: "mock-llm",
		NetworkPolicy: "intranet_only", Enabled: true, DefaultPriority: intPtr(1),
	})
	conn, _ := model.TestModelConnectivity(gormDB, tenantID, privChat, "chat")
	okAssert(conn.Status == "up", fmt.Sprintf("连通性测试成功（latency=%dms）", conn.LatencyMs))
	failProvID, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c03-smoke 不可达", Protocol: "openai_compat", DeploymentKind: "private",
		BaseURL: mockBase + "/fail", Model: "x", NetworkPolicy: "intranet_only", Enabled: true,
		MaxRetries: intPtr(0), DefaultPriority: intPtr(5),
	})
	connFail, _ := model.TestModelConnectivity(gormDB, tenantID, failProvID, "chat")
	okAssert(connFail.Status == "down", "不可达 provider 连通性测试记为失败、不进入可用路由")

	fmt.Println("\n[2] AIMed/翻译私有化路径 + 审计（9.1 / 9.2）")
	model.BindRoute(gormDB, tenantID, "chat", privChat, 1)
	model.BindRoute(gormDB, tenantID, "translate", privChat, 1)
	chatRes, _ := model.InvokeGeneration(gormDB, "chat", model.GenerationRequest{Messages: []model.ChatMessage{{Role: "user", Content: "hi"}}}, ctx)
	okAssert(strings.Contains(chatRes.Content, "mock-chat"), "AIMed(chat) 经私有化 provider 调用成功")
	trRes, _ := model.InvokeGeneration(gormDB, "translate", model.GenerationRequest{Messages: []model.ChatMessage{{Role: "user", Content: "hello"}}}, ctx)
	okAssert(strings.Contains(trRes.Content, "mock-chat"), "医学翻译(translate) 经私有化 provider 调用成功")
	okAssert(auditCount(gormDB, tenantID, "model_invoke", since) >= 2, "model_invoke 成功审计已落库")

	fmt.Println("\n[3] fallback 四要素审计（3.3 / 9.4）")
	fbFailID, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c03-smoke fallback失败源", Protocol: "openai_compat", DeploymentKind: "private",
		BaseURL: mockBase + "/fail", Model: "x", NetworkPolicy: "intranet_only", Enabled: true,
		MaxRetries: intPtr(0), DefaultPriority: intPtr(5),
	})
	model.BindRoute(gormDB, tenantID, "summarize", fbFailID, 1)
	model.BindRoute(gormDB, tenantID, "summarize", privChat, 2)
	sumRes, _ := model.InvokeGeneration(gormDB, "summarize", model.GenerationRequest{Messages: []model.ChatMessage{{Role: "user", Content: "long"}}}, ctx)
	okAssert(strings.Contains(sumRes.Content, "mock-chat"), "高优先级失败后自动 fallback 到下一 provider 成功")
	var fb struct {
		Metadata      datatypes.JSON `gorm:"column:metadata"`
		FailureReason *string        `gorm:"column:failure_reason"`
		CreatedAt     time.Time      `gorm:"column:created_at"`
	}
	gormDB.Raw(`SELECT metadata, failure_reason, created_at FROM audit_logs WHERE tenant_id = ? AND action_type = 'model_fallback' AND created_at >= ? ORDER BY created_at DESC LIMIT 1`, tenantID, since).Scan(&fb)
	okAssert(!fb.CreatedAt.IsZero(), "fallback 审计记录存在")
	var meta map[string]any
	_ = json.Unmarshal(fb.Metadata, &meta)
	_, hasTo := meta["toProvider"]
	okAssert(meta["fromProvider"] != nil && fb.FailureReason != nil && *fb.FailureReason != "" && hasTo && !fb.CreatedAt.IsZero(),
		"fallback 四要素齐全（provider / 失败原因 / 切换目标 / 时间戳 created_at）")

	fmt.Println("\n[4] 用途未绑定时拒绝调用（3.2）")
	_, rerr := model.InvokeRerank(gormDB, model.RerankRequest{Query: "q", Documents: []string{"a"}}, ctx)
	var cue *model.CapabilityUnavailableError
	okAssert(errors.As(rerr, &cue), "Rerank 未配置可用 provider 时返回明确不可用错误")

	fmt.Println("\n[5] 公网默认拒绝并切私有化（D6 本期保守降级）")
	pubProof, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c03-smoke 公网proof", Protocol: "openai_compat", DeploymentKind: "public",
		BaseURL: mockBase + "/ok", Model: "pub", Enabled: true,
	})
	model.BindRoute(gormDB, tenantID, "proofread", pubProof, 1)
	model.BindRoute(gormDB, tenantID, "proofread", privChat, 2)
	proofRes, _ := model.InvokeGeneration(gormDB, "proofread", model.GenerationRequest{Messages: []model.ChatMessage{{Role: "user", Content: "x"}}}, ctx)
	okAssert(strings.Contains(proofRes.Content, "mock-chat"), "公网被脱敏门禁跳过、改走私有化成功")
	okAssert(auditCount(gormDB, tenantID, "model_redaction_block", since) >= 1, "公网默认拒绝落 model_redaction_block 审计")

	fmt.Println("\n[6] Anthropic 协议不可绑定 Embedding/Rerank（2.3）")
	anthProv, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c03-smoke anthropic", Protocol: "anthropic_messages", DeploymentKind: "private",
		BaseURL: mockBase + "/ok", Model: "claude-x", NetworkPolicy: "intranet_only", Enabled: true,
	})
	bindErr := model.BindRoute(gormDB, tenantID, "embed", anthProv, 1)
	var rbe *model.RouteBindError
	okAssert(errors.As(bindErr, &rbe), "Anthropic 绑定 Embedding 被配置层拒绝")

	fmt.Println("\n[7] 禁用公网时主闭环经私有化/离线完成：文本 + 扫描两条解析链路（9.3 / 7.x）")
	privEmbed, _ := model.CreateModelProvider(gormDB, tenantID, model.ModelProviderInput{
		Name: "c03-smoke 私有embed", Protocol: "openai_compat", DeploymentKind: "private",
		BaseURL: mockBase + "/ok", Model: "mock-embed", NetworkPolicy: "intranet_only", Enabled: true,
	})
	model.BindRoute(gormDB, tenantID, "embed", privEmbed, 1)
	model.CreateVisualProvider(gormDB, tenantID, model.VisualProviderInput{
		Name: "c03-smoke 私有视觉", BackendKind: "private_service", DeploymentKind: "private",
		BaseURL: mockBase + "/ok", NetworkPolicy: "intranet_only", Enabled: true,
	})

	textDoc := makeDocument(gormDB, store, tenantID, adminID, "c03-smoke-note.md", "text/markdown",
		[]byte("# 标题一\n\n第一段正文。\n\n第二段正文，用于切分。"))
	scanDoc := makeDocument(gormDB, store, tenantID, adminID, "c03-smoke-scan.png", "image/png",
		[]byte("fake-png-bytes-for-poc"))

	_, dispatched, _ := engine.ParseTick(gormDB)
	okAssert(dispatched >= 2, fmt.Sprintf("事件消费创建作业（dispatched=%d）", dispatched))

	var textJob struct {
		Status       string     `gorm:"column:status"`
		IndexReadyAt *time.Time `gorm:"column:index_ready_at"`
	}
	gormDB.Raw(`SELECT status, index_ready_at FROM document_parse_jobs WHERE document_id = ?`, textDoc).Scan(&textJob)
	okAssert(textJob.Status == "succeeded", "文本型文档解析作业 succeeded")
	okAssert(textJob.IndexReadyAt != nil, "文本作业发出『索引就绪』(index_ready_at)")

	var textChunks []struct {
		ID       string         `gorm:"column:id"`
		ChunkACL datatypes.JSON `gorm:"column:chunk_acl"`
		ChunkID  string         `gorm:"column:chunk_id"`
		Dim      int            `gorm:"column:dim"`
	}
	gormDB.Raw(`SELECT c.id, c.chunk_acl, e.chunk_id, e.dim FROM document_chunks c JOIN embeddings e ON e.chunk_id = c.id WHERE c.document_id = ? AND c.superseded = FALSE`, textDoc).Scan(&textChunks)
	okAssert(len(textChunks) >= 2, "文本切分写入 document_chunks 且 embeddings 经 chunk_id 外键回连")
	allACL := true
	for _, ch := range textChunks {
		if len(ch.ChunkACL) == 0 || string(ch.ChunkACL) == "null" {
			allACL = false
		}
	}
	okAssert(allACL, "每个 chunk 携带 chunk_acl（默认继承文档级）")

	var scanStatus string
	gormDB.Raw(`SELECT status FROM document_parse_jobs WHERE document_id = ?`, scanDoc).Scan(&scanStatus)
	okAssert(scanStatus == "succeeded", "扫描件经私有化视觉解析 succeeded")
	var visCount int
	gormDB.Raw(`SELECT COUNT(*)::int FROM document_visual_parse_results WHERE document_id = ? AND failure_reason IS NULL`, scanDoc).Scan(&visCount)
	okAssert(visCount >= 1, "扫描件结构化结果写入 document_visual_parse_results")

	fmt.Println("\n[8] 1.5a：parse-status 查询无列错误且返回真实状态")
	var ps string
	gormDB.Raw(`SELECT j.status FROM document_parse_jobs j
		LEFT JOIN document_visual_parse_results r ON r.document_id = j.document_id AND r.document_version = j.document_version
		WHERE j.document_id = ? AND j.tenant_id = ? ORDER BY j.created_at DESC LIMIT 1`, textDoc, tenantID).Scan(&ps)
	okAssert(ps == "succeeded", "parse-status SELECT 返回真实状态 succeeded（非 pending、无列错误）")

	fmt.Println("\n[9] 结构校验：embeddings 无 tenant_id / document_chunks 有 chunk_acl（1.7）")
	var embCols, chunkCols []string
	gormDB.Raw(`SELECT column_name FROM information_schema.columns WHERE table_name = 'embeddings'`).Scan(&embCols)
	okAssert(!contains(embCols, "tenant_id"), "embeddings 表无独立 tenant_id 列")
	gormDB.Raw(`SELECT column_name FROM information_schema.columns WHERE table_name = 'document_chunks'`).Scan(&chunkCols)
	okAssert(contains(chunkCols, "chunk_acl"), "document_chunks 含 chunk_acl 物理列")

	cleanup(gormDB, tenantID)
	fmt.Println("\n✅ c03 冒烟全部通过")
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
