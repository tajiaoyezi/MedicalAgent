import { v4 as uuidv4 } from "uuid";
import type { PoolClient } from "pg";
import { config } from "../config.js";
import { writeAudit } from "./audit.js";
import {
  computeFileHash,
  createObjectStorage,
  objectKeyForVersion,
} from "./object-storage.js";
import { buildDocumentKey } from "./editor-config.js";
import type { EditorSession } from "./editor-sessions.js";
import {
  confirmPendingWritebackSave,
  extendCallbackSession,
  peekPendingWritebackSave,
  updateSessionRevision,
} from "./editor-sessions.js";
import {
  recordCallbackAttempt,
  recordCallbackFailure,
  recordCallbackSuccess,
} from "./editor-metrics.js";

const storage = createObjectStorage();

const SAVE_STATUSES = new Set([2, 6]);

export interface CallbackBody {
  key?: string;
  status?: number;
  url?: string;
  users?: string[];
  actions?: { type: number }[];
  token?: string;
}

async function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms));
}

export function assertDsDownloadUrl(url: string): void {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    throw new Error("回调下载 URL 无效");
  }
  const dsBase = new URL(config.onlyoffice.dsUrl);
  if (parsed.hostname !== dsBase.hostname) {
    throw new Error(`回调下载 URL 主机不匹配: ${parsed.hostname}`);
  }
  const dsPort = dsBase.port || (dsBase.protocol === "https:" ? "443" : "80");
  const urlPort = parsed.port || (parsed.protocol === "https:" ? "443" : "80");
  if (urlPort !== dsPort) {
    throw new Error(`回调下载 URL 端口不匹配: ${urlPort}`);
  }
}

async function downloadFromUrl(url: string): Promise<Buffer> {
  assertDsDownloadUrl(url);
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`下载失败: ${res.status}`);
  }
  const arr = await res.arrayBuffer();
  return Buffer.from(arr);
}

async function emitDocumentEvent(
  client: PoolClient,
  session: EditorSession,
  versionId: string,
  eventType: "save_new_version" | "ai_writeback",
  fileHash: string,
  source: "user_edit" | "ai_writeback",
  writebackSource?: string,
): Promise<void> {
  await client.query(
    `INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload)
     VALUES ($1, $2, $3, $4, $5::jsonb)`,
    [
      eventType,
      session.documentId,
      versionId,
      session.tenantId,
      JSON.stringify({
        file_hash: fileHash,
        source,
        writebackSource: writebackSource ?? null,
      }),
    ],
  );
}

async function createVersionFromBuffer(
  client: PoolClient,
  session: EditorSession,
  buffer: Buffer,
  savedBy: string,
  source: "user_edit" | "ai_writeback",
  writebackSource?: string,
): Promise<{
  versionId: string;
  fileHash: string;
  documentVersion: number;
  deduplicated: boolean;
}> {
  const fileHash = computeFileHash(buffer);

  const dup = await client.query(
    `SELECT version_id, document_version FROM document_versions
     WHERE document_id = $1 AND file_hash = $2`,
    [session.documentId, fileHash],
  );
  if (dup.rows.length) {
    const existing = dup.rows[0].version_id as string;
    const documentVersion = dup.rows[0].document_version as number;
    await client.query(
      `UPDATE documents SET current_version_id = $1, updated_at = NOW()
       WHERE document_id = $2`,
      [existing, session.documentId],
    );
    updateSessionRevision(
      session,
      existing,
      fileHash,
      buildDocumentKey(session.documentId, existing),
    );
    return {
      versionId: existing,
      fileHash,
      documentVersion,
      deduplicated: true,
    };
  }

  const versionId = uuidv4();
  const nextRes = await client.query(
    `SELECT COALESCE(MAX(document_version), 0) + 1 AS next
     FROM document_versions WHERE document_id = $1`,
    [session.documentId],
  );
  const documentVersion = nextRes.rows[0].next as number;
  const objectKey = objectKeyForVersion(
    session.tenantId,
    session.documentId,
    versionId,
  );

  const docRes = await client.query(
    `SELECT name, mime_type FROM documents WHERE document_id = $1`,
    [session.documentId],
  );
  const mimeType =
    (docRes.rows[0]?.mime_type as string) ?? "application/octet-stream";

  await storage.put(objectKey, buffer, mimeType);

  try {
    await client.query("BEGIN");
    await client.query(
      `INSERT INTO document_versions (
        version_id, document_id, tenant_id, document_version,
        file_hash, saved_by, saved_at, source, object_key, size_bytes
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9)`,
      [
        versionId,
        session.documentId,
        session.tenantId,
        documentVersion,
        fileHash,
        savedBy,
        source,
        objectKey,
        buffer.length,
      ],
    );

    await client.query(
      `UPDATE documents
       SET current_version_id = $1, updated_at = NOW()
       WHERE document_id = $2`,
      [versionId, session.documentId],
    );

    const eventType =
      source === "ai_writeback" ? "ai_writeback" : "save_new_version";
    await emitDocumentEvent(
      client,
      session,
      versionId,
      eventType,
      fileHash,
      source,
      writebackSource,
    );

    await client.query("COMMIT");
  } catch (err) {
    await client.query("ROLLBACK");
    await storage.delete(objectKey).catch(() => undefined);
    throw err;
  }

  updateSessionRevision(
    session,
    versionId,
    fileHash,
    buildDocumentKey(session.documentId, versionId),
  );

  return { versionId, fileHash, documentVersion, deduplicated: false };
}

export async function processSaveCallback(
  client: PoolClient,
  session: EditorSession,
  body: CallbackBody,
  actorId: string,
  actorRole: string,
): Promise<{ error: number }> {
  recordCallbackAttempt();
  extendCallbackSession(session);

  if (body.key && body.key !== session.documentKey) {
    recordCallbackFailure();
    await writeAudit(client, {
      tenantId: session.tenantId,
      actorId,
      actorRole,
      actionType: "editor_callback",
      targetType: "document",
      targetId: session.documentId,
      result: "失败",
      failureReason: "document.key 不匹配",
    });
    return { error: 1 };
  }

  const status = body.status ?? 0;
  if (!SAVE_STATUSES.has(status)) {
    return { error: 0 };
  }

  if (!body.url) {
    recordCallbackFailure();
    return { error: 1 };
  }

  const writebackSource = peekPendingWritebackSave(session, status);
  const source: "user_edit" | "ai_writeback" = writebackSource
    ? "ai_writeback"
    : "user_edit";

  const maxRetries = 3;
  let lastError: Error | null = null;

  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      const buffer = await downloadFromUrl(body.url);
      const result = await createVersionFromBuffer(
        client,
        session,
        buffer,
        actorId,
        source,
        writebackSource,
      );

      if (writebackSource) {
        confirmPendingWritebackSave(session);
      }

      recordCallbackSuccess();
      await writeAudit(client, {
        tenantId: session.tenantId,
        actorId,
        actorRole,
        actionType: "editor_save",
        targetType: "document",
        targetId: session.documentId,
        result: "成功",
        metadata: {
          deduplicated: result.deduplicated,
          source,
          callbackStatus: status,
        },
      });
      return { error: 0 };
    } catch (err) {
      lastError = err instanceof Error ? err : new Error(String(err));
      if (attempt < maxRetries - 1) {
        await sleep(500 * Math.pow(2, attempt));
      }
    }
  }

  recordCallbackFailure();
  await writeAudit(client, {
    tenantId: session.tenantId,
    actorId,
    actorRole,
    actionType: "editor_callback",
    targetType: "document",
    targetId: session.documentId,
    result: "失败",
    failureReason: lastError?.message ?? "保存回调重试耗尽",
    metadata: { alert: true, retries: maxRetries },
  });

  console.error(
    "[editor] callback exhausted retries",
    session.documentId,
    lastError,
  );
  return { error: 1 };
}
