// c03｜文档视觉解析服务（design D5，§16.5）：可插拔后端、统一结构化契约、公网/私有化双路径、脱敏门控接缝。
import type { PoolClient } from "pg";
import { writeAudit } from "../audit.js";
import {
  isFallbackable,
  providerFetch,
  ProviderError,
  type ProviderConnection,
} from "../model/http.js";
import { redactionGateway } from "../model/redaction-gateway.js";
import { recordHealth, resolveVisualChain } from "../model/provider-store.js";

/** 低于此置信度的页/结果被标记为低置信度，供下游决定是否人工复核（task 6.8） */
export const LOW_CONFIDENCE_THRESHOLD = 0.6;

export interface VisualParagraph {
  paragraphIndex: number;
  text: string;
  bbox?: number[];
  headingLevel?: number;
}

export interface VisualPage {
  page: number;
  paragraphs: VisualParagraph[];
  tables: unknown[];
  images: unknown[];
  header?: string;
  footer?: string;
  confidence: number;
  lowConfidence: boolean;
}

export interface VisualParseResult {
  fullText: string;
  pages: VisualPage[];
  /** chunk 定位信息：供 document-parsing 切分与引用回 chunk */
  chunkLocators: Array<{ page: number; paragraphIndex: number }>;
  confidence: number;
  backendKind: string;
  deploymentKind: string;
}

export class VisualParseFailedError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "VisualParseFailedError";
  }
}

export class VisualProviderMissingError extends Error {}

interface RawVisualResponse {
  pages?: Array<{
    page?: number;
    paragraphs?: Array<{ text?: string; bbox?: number[]; heading_level?: number }>;
    tables?: unknown[];
    images?: unknown[];
    header?: string;
    footer?: string;
    confidence?: number;
  }>;
  confidence?: number;
  failure_reason?: string | null;
}

function normalize(raw: RawVisualResponse, conn: ProviderConnection): VisualParseResult {
  const pages: VisualPage[] = (raw.pages ?? []).map((p, pi) => {
    const pageNo = p.page ?? pi + 1;
    const paragraphs: VisualParagraph[] = (p.paragraphs ?? []).map((para, idx) => ({
      paragraphIndex: idx,
      text: para.text ?? "",
      bbox: para.bbox,
      headingLevel: para.heading_level,
    }));
    const conf = typeof p.confidence === "number" ? p.confidence : (raw.confidence ?? 1);
    return {
      page: pageNo,
      paragraphs,
      tables: p.tables ?? [],
      images: p.images ?? [],
      header: p.header,
      footer: p.footer,
      confidence: conf,
      lowConfidence: conf < LOW_CONFIDENCE_THRESHOLD,
    };
  });
  const chunkLocators: Array<{ page: number; paragraphIndex: number }> = [];
  const textParts: string[] = [];
  for (const pg of pages) {
    for (const para of pg.paragraphs) {
      if (para.text.trim()) {
        chunkLocators.push({ page: pg.page, paragraphIndex: para.paragraphIndex });
        textParts.push(para.text);
      }
    }
  }
  const confidence =
    typeof raw.confidence === "number"
      ? raw.confidence
      : pages.length
        ? pages.reduce((s, p) => s + p.confidence, 0) / pages.length
        : 0;
  return {
    fullText: textParts.join("\n"),
    pages,
    chunkLocators,
    confidence,
    backendKind: conn.backendKind ?? "unknown",
    deploymentKind: conn.deploymentKind,
  };
}

export interface VisualParseTarget {
  tenantId: string;
  documentId: string;
  documentVersion: number;
  objectKey: string;
  filename: string;
  mime: string;
  jobId?: string | null;
  actorId?: string | null;
  actorRole?: string | null;
}

async function callBackend(conn: ProviderConnection, target: VisualParseTarget): Promise<VisualParseResult> {
  const raw = (await providerFetch(
    conn,
    "parse",
    {
      document_id: target.documentId,
      object_key: target.objectKey,
      filename: target.filename,
      mime: target.mime,
    },
    conn.credential ? { authorization: `Bearer ${conn.credential}` } : {},
  )) as RawVisualResponse;

  if (raw.failure_reason) {
    // 内容质量问题（如清晰度过低）：不输出伪结果，直接失败（非 provider 故障、不 fallback）
    throw new VisualParseFailedError(raw.failure_reason);
  }
  const result = normalize(raw, conn);
  if (!result.fullText.trim()) {
    throw new VisualParseFailedError("视觉解析未产出可用文本（疑似清晰度过低）");
  }
  return result;
}

/** 解析并落 document_visual_parse_results；公网后端先过 c09 脱敏门禁；fallback/审计齐全。 */
export async function runVisualParse(
  client: PoolClient,
  target: VisualParseTarget,
): Promise<VisualParseResult> {
  const chain = await resolveVisualChain(client, target.tenantId);
  if (!chain.length) {
    throw new VisualProviderMissingError("未配置可用的视觉解析 provider");
  }

  for (let i = 0; i < chain.length; i++) {
    const conn = chain[i];
    const next = chain[i + 1] ?? null;

    // 公网解析前置脱敏门禁（task 6.6）：未通过则跳过公网，改走私有化
    if (conn.deploymentKind === "public") {
      const verdict = await redactionGateway.evaluate({ tenantId: target.tenantId, text: "<document>" });
      if (!verdict.available || !verdict.passed) {
        await writeAudit(client, {
          tenantId: target.tenantId,
          actorId: target.actorId,
          actorRole: target.actorRole,
          actionType: "visual_redaction_block",
          targetType: "document",
          targetId: target.documentId,
          result: "失败",
          failureReason: verdict.reason,
          metadata: { provider: conn.name, deploymentKind: "public", switchTo: next?.name ?? null },
        });
        continue;
      }
    }

    try {
      const result = await callBackend(conn, target);
      await client.query(
        `INSERT INTO document_visual_parse_results
           (tenant_id, document_id, document_version, job_id, full_text, pages, chunk_locators,
            confidence, failure_reason, backend_kind, deployment_kind)
         VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,NULL,$9,$10)`,
        [
          target.tenantId,
          target.documentId,
          target.documentVersion,
          target.jobId ?? null,
          result.fullText,
          JSON.stringify(result.pages),
          JSON.stringify(result.chunkLocators),
          result.confidence,
          result.backendKind,
          result.deploymentKind,
        ],
      );
      await writeAudit(client, {
        tenantId: target.tenantId,
        actorId: target.actorId,
        actorRole: target.actorRole,
        actionType: "visual_parse",
        targetType: "document",
        targetId: target.documentId,
        result: "成功",
        metadata: {
          provider: conn.name,
          deploymentKind: conn.deploymentKind,
          backendKind: conn.backendKind,
          confidence: result.confidence,
          lowConfidence: result.confidence < LOW_CONFIDENCE_THRESHOLD,
        },
      });
      return result;
    } catch (e) {
      if (e instanceof VisualParseFailedError) {
        // 内容质量失败：记结果失败原因（无伪结果）+ 审计，并上抛使作业转失败
        await client.query(
          `INSERT INTO document_visual_parse_results
             (tenant_id, document_id, document_version, job_id, full_text, pages, chunk_locators,
              confidence, failure_reason, backend_kind, deployment_kind)
           VALUES ($1,$2,$3,$4,NULL,'[]'::jsonb,'[]'::jsonb,0,$5,$6,$7)`,
          [
            target.tenantId,
            target.documentId,
            target.documentVersion,
            target.jobId ?? null,
            e.message,
            conn.backendKind ?? null,
            conn.deploymentKind,
          ],
        );
        await writeAudit(client, {
          tenantId: target.tenantId,
          actorId: target.actorId,
          actorRole: target.actorRole,
          actionType: "visual_parse",
          targetType: "document",
          targetId: target.documentId,
          result: "失败",
          failureReason: e.message,
          metadata: { provider: conn.name, deploymentKind: conn.deploymentKind },
        });
        throw e;
      }
      const err = e instanceof ProviderError ? e : new ProviderError("unknown", (e as Error).message);
      await writeAudit(client, {
        tenantId: target.tenantId,
        actorId: target.actorId,
        actorRole: target.actorRole,
        actionType: "visual_parse",
        targetType: "document",
        targetId: target.documentId,
        result: "失败",
        failureReason: err.message,
        metadata: { provider: conn.name, deploymentKind: conn.deploymentKind, errorClass: err.errorClass, switchTo: next?.name ?? null },
      });
      if (!isFallbackable(err.errorClass)) throw err;
      await recordHealth(client, {
        tenantId: target.tenantId,
        providerId: conn.providerId,
        providerKind: "visual",
        checkKind: "passive",
        status: "down",
        error: err.message,
      });
      // 继续 fallback 下一 provider
    }
  }

  throw new VisualProviderMissingError("所有视觉解析 provider 依次失败或被脱敏门禁拒绝");
}

// ——— 质量指标计算（task 6.8）：c03 仅自验指标计算逻辑，三项阈值最终达标判定随 c09 Evals 落地 ———

export interface VisualEvalCase {
  predictedPage: number;
  expectedPage: number;
  tableStructureCorrect: boolean;
}

export interface VisualQualityMetrics {
  pageLocalizationRate: number; // 页码定位成功率（误差 ≤ 1 页计成功）
  tableStructureRate: number; // 表格结构识别成功率
  maxPageError: number; // 引用源页码最大误差
  count: number;
}

export function computeVisualMetrics(cases: VisualEvalCase[]): VisualQualityMetrics {
  if (!cases.length) {
    return { pageLocalizationRate: 0, tableStructureRate: 0, maxPageError: 0, count: 0 };
  }
  let pageOk = 0;
  let tableOk = 0;
  let maxErr = 0;
  for (const c of cases) {
    const err = Math.abs(c.predictedPage - c.expectedPage);
    if (err <= 1) pageOk++;
    if (c.tableStructureCorrect) tableOk++;
    if (err > maxErr) maxErr = err;
  }
  return {
    pageLocalizationRate: pageOk / cases.length,
    tableStructureRate: tableOk / cases.length,
    maxPageError: maxErr,
    count: cases.length,
  };
}
