// Package parsing 复刻 c03 文档解析流水线：chunk 切分、视觉解析、状态机、事件消费、后台 worker。
package parsing

import "sync"

// IndexReadyEvent：design D8 边界终点 indexing_handoff。c03 仅发出，不构建索引（下游 c04/c06）。
type IndexReadyEvent struct {
	TenantID        string
	DocumentID      string
	DocumentVersion int
	JobID           string
	ChunkCount      int
}

var (
	indexReadyMu       sync.Mutex
	indexReadyHandlers []func(IndexReadyEvent)
)

// OnIndexReady 注册订阅者（c04/c06 未建时无订阅者，仅保留接缝 + 持久化契约 index_ready_at）。
func OnIndexReady(h func(IndexReadyEvent)) {
	indexReadyMu.Lock()
	indexReadyHandlers = append(indexReadyHandlers, h)
	indexReadyMu.Unlock()
}

func emitIndexReady(ev IndexReadyEvent) {
	indexReadyMu.Lock()
	hs := append([]func(IndexReadyEvent){}, indexReadyHandlers...)
	indexReadyMu.Unlock()
	for _, h := range hs {
		h(ev)
	}
}
