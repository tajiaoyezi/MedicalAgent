// c03｜「索引就绪」事件总线（design D8 边界终点 indexing_handoff）
// c03 仅发出该事件、不构建任何检索索引；下游唯一检索索引构建方=c04，知识库收尾=c06。
// 持久化契约 = document_parse_jobs.index_ready_at；本进程内事件供同进程订阅者即时感知。
import { EventEmitter } from "node:events";

export interface IndexReadyEvent {
  tenantId: string;
  documentId: string;
  documentVersion: number;
  jobId: string;
  chunkCount: number;
}

export const parseEvents = new EventEmitter();

export function emitIndexReady(ev: IndexReadyEvent): void {
  parseEvents.emit("index-ready", ev);
}
