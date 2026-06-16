package writeback

// 默认写回策略矩阵（D3 / §9.6 / §9.4）：AI 操作类型 → c02 写回方法 + document_versions.source + 影响范围 + diff 粒度。
// 默认值不可被技能静默覆盖；用户可在确认网关点「生成副本」改走副本路径（StrategyForCopy）。

// 操作类型枚举（与 writeback_confirmations.operation_type CHECK 一致）。
const (
	OpFullPolish  = "全文润色"
	OpSpanPolish  = "选区润色"
	OpProofread   = "校对"
	OpSpanTrans   = "选区翻译"
	OpCitation    = "补引用"
	OpAnnotation  = "插入标注"
	OpLayout      = "AI 论文排版"
)

// Strategy 描述一类操作的默认写回方式。
type Strategy struct {
	OperationType   string `json:"operationType"`
	BridgeMethod    string `json:"bridgeMethod"`    // c02 写回方法
	WritebackSource string `json:"writebackSource"` // document_versions.source
	ImpactScope     string `json:"impactScope"`     // 影响范围（selection / document / positions）
	DiffKind        string `json:"diffKind"`        // diff 粒度：inline / sidebyside / suggestions / layout
}

// strategyMatrix 是「应用到文档」路径的默认映射（§9.6）。
var strategyMatrix = map[string]Strategy{
	OpSpanPolish: {OpSpanPolish, "replaceSelection", "ai_writeback", "selection", "inline"},
	OpSpanTrans:  {OpSpanTrans, "replaceSelection", "ai_writeback", "selection", "inline"},
	OpFullPolish: {OpFullPolish, "createNewDocument", "ai_writeback", "document", "sidebyside"},
	OpProofread:  {OpProofread, "insertComment", "ai_writeback", "positions", "suggestions"},
	OpCitation:   {OpCitation, "insertCitation", "ai_writeback", "positions", "suggestions"},
	OpAnnotation: {OpAnnotation, "insertComment", "ai_writeback", "positions", "suggestions"},
	OpLayout:     {OpLayout, "applyStyle", "ai_writeback", "document", "layout"},
}

// IsKnownOperation 校验 operationType 是否在矩阵内。
func IsKnownOperation(op string) bool {
	_, ok := strategyMatrix[op]
	return ok
}

// StrategyFor 返回「应用到文档」的默认策略；未知操作返回零值与 false。
func StrategyFor(op string) (Strategy, bool) {
	s, ok := strategyMatrix[op]
	return s, ok
}

// StrategyForCopy 返回「生成副本」策略：任何操作先 createNewDocument 复制原文档再写入，原文档不变（D3）。
func StrategyForCopy(op string) Strategy {
	base, ok := strategyMatrix[op]
	scope := "document"
	diff := "sidebyside"
	if ok {
		diff = base.DiffKind
	}
	return Strategy{
		OperationType:   op,
		BridgeMethod:    "createNewDocument",
		WritebackSource: "ai_writeback",
		ImpactScope:     scope,
		DiffKind:        diff,
	}
}
