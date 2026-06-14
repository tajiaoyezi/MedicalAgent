import type { PoolClient } from "pg";

export type AuditResult = "成功" | "失败";

export interface AuditEntry {
  tenantId: string;
  actorId?: string | null;
  actorRole?: string | null;
  actionType: string;
  targetType?: string | null;
  targetId?: string | null;
  result: AuditResult;
  failureReason?: string | null;
  metadata?: Record<string, unknown>;
}

export async function writeAudit(
  client: PoolClient,
  entry: AuditEntry,
): Promise<string> {
  const res = await client.query(
    `INSERT INTO audit_logs (
      tenant_id, actor_id, actor_role, action_type,
      target_type, target_id, result, failure_reason, metadata
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
    RETURNING audit_id`,
    [
      entry.tenantId,
      entry.actorId ?? null,
      entry.actorRole ?? null,
      entry.actionType,
      entry.targetType ?? null,
      entry.targetId ?? null,
      entry.result,
      entry.failureReason ?? null,
      JSON.stringify(entry.metadata ?? {}),
    ],
  );
  return res.rows[0].audit_id as string;
}
