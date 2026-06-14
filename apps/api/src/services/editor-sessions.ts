import { randomBytes } from "node:crypto";
import { config } from "../config.js";

/** 写回保存意图：saveDocument authorize 创建，plugin Api.Save 后 arm */
export interface PendingWritebackSave {
  source: string;
  saveIntentId: string;
  armedAt: number;
  armed: boolean;
}

export interface EditorSession {
  openToken: string;
  callbackToken: string;
  bridgeToken: string;
  documentId: string;
  documentKey: string;
  tenantId: string;
  userId: string;
  versionId: string;
  revision: string;
  expiresAt: number;
  pendingWritebackSave?: PendingWritebackSave;
}

const sessions = new Map<string, EditorSession>();
const callbackIndex = new Map<string, string>();
const bridgeIndex = new Map<string, string>();

const OPEN_TTL_MS = config.onlyoffice.openTokenTtlSeconds * 1000;
const CALLBACK_TTL_MS = config.onlyoffice.callbackTokenTtlSeconds * 1000;

const WRITEBACK_INTENT_TTL_MS = 2 * 60 * 1000;
const WRITEBACK_ARM_WINDOW_MS = 30 * 1000;

function purgeExpired() {
  const now = Date.now();
  for (const [token, session] of sessions) {
    if (session.expiresAt < now) {
      sessions.delete(token);
      callbackIndex.delete(session.callbackToken);
      bridgeIndex.delete(session.bridgeToken);
    }
  }
}

function newToken(): string {
  return randomBytes(24).toString("hex");
}

export function createEditorSession(input: {
  documentId: string;
  documentKey: string;
  tenantId: string;
  userId: string;
  versionId: string;
  revision: string;
}): EditorSession {
  purgeExpired();
  const session: EditorSession = {
    openToken: newToken(),
    callbackToken: newToken(),
    bridgeToken: newToken(),
    expiresAt: Date.now() + CALLBACK_TTL_MS,
    ...input,
  };
  sessions.set(session.openToken, session);
  callbackIndex.set(session.callbackToken, session.openToken);
  bridgeIndex.set(session.bridgeToken, session.openToken);
  return session;
}

export function getSessionByOpenToken(token: string): EditorSession | null {
  purgeExpired();
  return sessions.get(token) ?? null;
}

export function getSessionByCallbackToken(token: string): EditorSession | null {
  purgeExpired();
  const open = callbackIndex.get(token);
  return open ? sessions.get(open) ?? null : null;
}

export function getSessionByBridgeToken(token: string): EditorSession | null {
  purgeExpired();
  const open = bridgeIndex.get(token);
  return open ? sessions.get(open) ?? null : null;
}

export function touchEditorSession(session: EditorSession): void {
  session.expiresAt = Date.now() + CALLBACK_TTL_MS;
}

export function extendCallbackSession(session: EditorSession): void {
  touchEditorSession(session);
}

export function updateSessionRevision(
  session: EditorSession,
  versionId: string,
  revision: string,
  documentKey: string,
): void {
  session.versionId = versionId;
  session.revision = revision;
  session.documentKey = documentKey;
}

/** saveDocument authorize：创建未 armed 的写回意图 */
export function createSaveIntent(
  session: EditorSession,
  source: string,
): string {
  const saveIntentId = randomBytes(16).toString("hex");
  session.pendingWritebackSave = {
    source,
    saveIntentId,
    armedAt: 0,
    armed: false,
  };
  return saveIntentId;
}

/** plugin Api.Save 成功后 arm，窄窗口内 forcesave 回调可消费 */
export function armWritebackSaveIntent(
  session: EditorSession,
  saveIntentId: string,
): boolean {
  const pending = session.pendingWritebackSave;
  if (!pending || pending.saveIntentId !== saveIntentId) return false;
  if (pending.armed) return false;
  pending.armed = true;
  pending.armedAt = Date.now();
  return true;
}

function isPendingWritebackActive(
  pending: PendingWritebackSave,
  requireForcesave: boolean,
): boolean {
  if (!pending.armed) return false;
  if (Date.now() - pending.armedAt > WRITEBACK_ARM_WINDOW_MS) return false;
  return true;
}

/** 回调处理前只读 peek；仅 forcesave(status=6) 且已 armed 时返回 source */
export function peekPendingWritebackSave(
  session: EditorSession,
  callbackStatus: number,
): string | undefined {
  const pending = session.pendingWritebackSave;
  if (!pending) return undefined;
  if (callbackStatus !== 6) return undefined;
  if (!isPendingWritebackActive(pending, true)) return undefined;
  if (Date.now() - pending.armedAt > WRITEBACK_INTENT_TTL_MS) return undefined;
  return pending.source;
}

/** 落库成功后确认消费 */
export function confirmPendingWritebackSave(session: EditorSession): void {
  session.pendingWritebackSave = undefined;
}

/** 测试用：清空会话 */
export function clearAllEditorSessions(): void {
  sessions.clear();
  callbackIndex.clear();
  bridgeIndex.clear();
}
