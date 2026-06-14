export type PermissionLevel =
  | "owner"
  | "manage"
  | "edit"
  | "comment"
  | "view"
  | "none";

const LEVEL_ORDER: Record<PermissionLevel, number> = {
  none: 0,
  view: 1,
  comment: 2,
  edit: 3,
  manage: 4,
  owner: 5,
};

export function maxPermissionLevel(
  levels: PermissionLevel[],
): PermissionLevel {
  let best: PermissionLevel = "none";
  for (const l of levels) {
    if (LEVEL_ORDER[l] > LEVEL_ORDER[best]) best = l;
  }
  return best;
}

export function canDownload(level: PermissionLevel): boolean {
  return (
    level === "owner" ||
    level === "manage" ||
    level === "edit" ||
    level === "view"
  );
}

export function canEdit(level: PermissionLevel): boolean {
  return level === "owner" || level === "manage" || level === "edit";
}

export function canManagePermissions(level: PermissionLevel): boolean {
  return level === "owner" || level === "manage";
}

export function canShare(level: PermissionLevel): boolean {
  return level === "owner" || level === "manage";
}

export function canComment(level: PermissionLevel): boolean {
  return (
    level === "owner" ||
    level === "manage" ||
    level === "edit" ||
    level === "comment"
  );
}

export interface DocumentRow {
  document_id: string;
  tenant_id: string;
  owner_id: string;
  name: string;
  space: string;
  app_source: string | null;
  is_deleted: boolean;
  current_version_id?: string | null;
}

export interface AuthUser {
  userId: string;
  tenantId: string;
  username: string;
  displayName: string;
  deptId: string | null;
  roleSlugs: string[];
  permissions: string[];
}

export async function resolveEffectivePermission(
  client: import("pg").PoolClient,
  user: AuthUser,
  document: DocumentRow,
): Promise<PermissionLevel> {
  if (document.tenant_id !== user.tenantId) return "none";

  const levels: PermissionLevel[] = [];

  if (document.owner_id === user.userId) {
    levels.push("owner");
  }

  const { rows } = await client.query(
    `SELECT principal_type, principal_id, permission_level
     FROM document_permissions
     WHERE document_id = $1 AND tenant_id = $2`,
    [document.document_id, user.tenantId],
  );

  for (const row of rows) {
    const principalType = row.principal_type as string;
    const principalId = row.principal_id as string;
    const level = row.permission_level as PermissionLevel;

    if (principalType === "user" && principalId === user.userId) {
      levels.push(level);
    }
    if (
      principalType === "role" &&
      user.roleSlugs.includes(principalId)
    ) {
      levels.push(level);
    }
    if (
      principalType === "dept" &&
      user.deptId &&
      principalId === user.deptId
    ) {
      levels.push(level);
    }
  }

  if (levels.length === 0 && document.space === "my" && document.owner_id === user.userId) {
    levels.push("owner");
  }

  return maxPermissionLevel(levels.length ? levels : ["none"]);
}
