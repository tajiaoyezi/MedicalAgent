import type { FastifyRequest } from "fastify";
import { revokedUserIds } from "./session-revoke.js";
import type { AuthUser } from "../services/document-permissions.js";

export interface SessionData {
  user?: AuthUser;
  revokedUserIds?: string[];
}

export function getSessionUser(request: FastifyRequest): AuthUser | null {
  const session = request.session as SessionData;
  if (!session.user) return null;
  if (revokedUserIds.has(session.user.userId)) return null;
  return session.user;
}

export function requireAuth(request: FastifyRequest): AuthUser {
  const user = getSessionUser(request);
  if (!user) {
    const err = new Error("未登录") as Error & { statusCode: number };
    err.statusCode = 401;
    throw err;
  }
  return user;
}

export function requirePermission(user: AuthUser, permission: string): void {
  if (!user.permissions.includes(permission)) {
    const err = new Error("无权限") as Error & { statusCode: number };
    err.statusCode = 403;
    throw err;
  }
}

export async function loadUserById(
  client: import("pg").PoolClient,
  userId: string,
): Promise<AuthUser | null> {
  const { rows } = await client.query(
    `SELECT u.user_id, u.tenant_id, u.username, u.display_name, u.dept_id, u.is_enabled
     FROM users u WHERE u.user_id = $1`,
    [userId],
  );
  if (!rows.length || !rows[0].is_enabled) return null;

  const roleRes = await client.query(
    `SELECT r.slug FROM roles r
     JOIN user_roles ur ON ur.role_id = r.role_id
     WHERE ur.user_id = $1`,
    [userId],
  );
  const permRes = await client.query(
    `SELECT DISTINCT p.name FROM permissions p
     JOIN role_permissions rp ON rp.permission_id = p.permission_id
     JOIN user_roles ur ON ur.role_id = rp.role_id
     WHERE ur.user_id = $1`,
    [userId],
  );

  return {
    userId: rows[0].user_id,
    tenantId: rows[0].tenant_id,
    username: rows[0].username,
    displayName: rows[0].display_name,
    deptId: rows[0].dept_id,
    roleSlugs: roleRes.rows.map((r) => r.slug as string),
    permissions: permRes.rows.map((r) => r.name as string),
  };
}
