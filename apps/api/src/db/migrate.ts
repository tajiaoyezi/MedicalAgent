import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import bcrypt from "bcryptjs";
import { pool } from "./pool.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

async function runMigrations() {
  const client = await pool.connect();
  try {
    await client.query(`
      CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      )
    `);

    const migrationsDir = join(__dirname, "migrations");
    const files = ["001_initial.sql"].sort();

    for (const file of files) {
      const version = file.replace(".sql", "");
      const { rows } = await client.query(
        "SELECT 1 FROM schema_migrations WHERE version = $1",
        [version],
      );
      if (rows.length > 0) {
        console.log(`Skip migration ${version}`);
        continue;
      }

      const sql = readFileSync(join(migrationsDir, file), "utf8");
      await client.query(sql);
      await client.query(
        "INSERT INTO schema_migrations (version) VALUES ($1)",
        [version],
      );
      console.log(`Applied migration ${version}`);
    }

    await seedIfNeeded(client);
  } finally {
    client.release();
    await pool.end();
  }
}

async function seedIfNeeded(client: import("pg").PoolClient) {
  const { rows } = await client.query("SELECT tenant_id FROM tenants LIMIT 1");
  if (rows.length > 0) {
    console.log("Seed data already exists, skipping");
    return;
  }

  const tenantRes = await client.query(
    `INSERT INTO tenants (name, org_type, enabled_modules, branding)
     VALUES ($1, $2, $3::jsonb, $4::jsonb)
     RETURNING tenant_id`,
    [
      "MedOffice 演示医院",
      "hospital",
      JSON.stringify([
        "aimed",
        "knowledge",
        "translation",
        "templates",
        "documents",
        "admin",
      ]),
      JSON.stringify({
        logo_url: null,
        primary_color: "#1677ff",
        secondary_color: "#69b1ff",
        login_background: null,
        nav_style: "default",
        button_radius: "6px",
        font_size: "14px",
        default_theme: "blue-white",
      }),
    ],
  );
  const tenantId = tenantRes.rows[0].tenant_id as string;

  const permissions = [
    { name: "document:read", description: "读取文档" },
    { name: "document:write", description: "写入文档" },
    { name: "document:share", description: "分享文档" },
    { name: "user:manage", description: "用户管理" },
    { name: "audit:view", description: "查看审计" },
    { name: "admin:console", description: "管理后台" },
    { name: "highrisk:confirm", description: "高风险确认" },
    { name: "template:manage", description: "模板管理" },
    { name: "kb:create", description: "创建知识库" },
  ];

  const permIds: Record<string, string> = {};
  for (const p of permissions) {
    const res = await client.query(
      `INSERT INTO permissions (name, description) VALUES ($1, $2)
       ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
       RETURNING permission_id`,
      [p.name, p.description],
    );
    permIds[p.name] = res.rows[0].permission_id;
  }

  const roleDefs: Array<{
    slug: string;
    name: string;
    perms: string[];
  }> = [
    {
      slug: "admin",
      name: "管理员",
      perms: [
        "document:read",
        "document:write",
        "document:share",
        "user:manage",
        "audit:view",
        "admin:console",
        "template:manage",
        "kb:create",
      ],
    },
    { slug: "user", name: "普通用户", perms: ["document:read", "document:write"] },
    { slug: "dept", name: "科室", perms: ["document:read", "document:write"] },
    {
      slug: "doctor",
      name: "医生",
      perms: ["document:read", "document:write", "highrisk:confirm"],
    },
    {
      slug: "reviewer",
      name: "授权审核",
      perms: ["document:read", "document:write", "highrisk:confirm"],
    },
  ];

  const roleIds: Record<string, string> = {};
  for (const r of roleDefs) {
    const res = await client.query(
      `INSERT INTO roles (tenant_id, name, slug) VALUES ($1, $2, $3) RETURNING role_id`,
      [tenantId, r.name, r.slug],
    );
    roleIds[r.slug] = res.rows[0].role_id;
    for (const perm of r.perms) {
      await client.query(
        `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)
         ON CONFLICT DO NOTHING`,
        [roleIds[r.slug], permIds[perm]],
      );
    }
  }

  const adminHash = await bcrypt.hash("admin123", 10);
  const userHash = await bcrypt.hash("user123", 10);

  const adminRes = await client.query(
    `INSERT INTO users (tenant_id, username, password_hash, display_name, dept_id)
     VALUES ($1, 'admin', $2, '演示管理员', 'dept-demo')
     RETURNING user_id`,
    [tenantId, adminHash],
  );
  const userRes = await client.query(
    `INSERT INTO users (tenant_id, username, password_hash, display_name, dept_id)
     VALUES ($1, 'user', $2, '演示用户', 'dept-demo')
     RETURNING user_id`,
    [tenantId, userHash],
  );

  await client.query(
    `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
    [adminRes.rows[0].user_id, roleIds.admin],
  );
  await client.query(
    `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
    [userRes.rows[0].user_id, roleIds.user],
  );

  console.log("Seed data inserted (admin/admin123, user/user123)");
}

runMigrations().catch((err) => {
  console.error(err);
  process.exit(1);
});
