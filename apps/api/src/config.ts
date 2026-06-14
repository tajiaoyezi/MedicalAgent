import dotenv from "dotenv";
import { resolve } from "node:path";

dotenv.config({ path: resolve(process.cwd(), "../../.env") });
dotenv.config();

export type StorageBackend = "minio" | "s3";

function resolveOnlyofficeJwtSecret(): string {
  const secret = process.env.ONLYOFFICE_JWT_SECRET?.trim();
  if (secret) return secret;
  const nodeEnv = process.env.NODE_ENV ?? "development";
  if (nodeEnv === "production") {
    throw new Error(
      "ONLYOFFICE_JWT_SECRET 必须在生产环境显式配置，禁止使用默认值",
    );
  }
  console.warn(
    "[config] ONLYOFFICE_JWT_SECRET 未设置，开发环境使用本地占位密钥（仅限本机 POC，勿用于部署）",
  );
  return "dev-local-onlyoffice-jwt-do-not-deploy";
}

export const config = {
  nodeEnv: process.env.NODE_ENV ?? "development",
  port: Number(process.env.API_PORT ?? 3001),
  host: process.env.API_HOST ?? "0.0.0.0",
  sessionSecret: process.env.SESSION_SECRET ?? "dev-session-secret-change-me",
  webOrigin: process.env.WEB_ORIGIN ?? "http://localhost:5173",
  databaseUrl:
    process.env.DATABASE_URL ??
    "postgres://medoffice:medoffice@localhost:5432/medoffice",
  storage: {
    backend: (process.env.STORAGE_BACKEND ?? "minio") as StorageBackend,
    endpoint:
      process.env.STORAGE_BACKEND === "s3"
        ? process.env.S3_ENDPOINT
        : process.env.MINIO_ENDPOINT ?? "http://localhost:9000",
    accessKey:
      process.env.STORAGE_BACKEND === "s3"
        ? process.env.S3_ACCESS_KEY
        : process.env.MINIO_ACCESS_KEY ?? "minioadmin",
    secretKey:
      process.env.STORAGE_BACKEND === "s3"
        ? process.env.S3_SECRET_KEY
        : process.env.MINIO_SECRET_KEY ?? "minioadmin",
    bucket:
      process.env.STORAGE_BACKEND === "s3"
        ? process.env.S3_BUCKET ?? "medoffice"
        : process.env.MINIO_BUCKET ?? "medoffice",
    region:
      process.env.STORAGE_BACKEND === "s3"
        ? process.env.S3_REGION ?? "us-east-1"
        : process.env.MINIO_REGION ?? "us-east-1",
    forcePathStyle: process.env.STORAGE_BACKEND !== "s3",
  },
  onlyoffice: {
    /** Document Server 对外 URL（浏览器加载 api.js） */
    dsUrl: process.env.ONLYOFFICE_DS_URL ?? "http://localhost:8080",
    /** DS 回调业务 API 时使用的公网可达地址（容器内需 host.docker.internal） */
    apiPublicUrl:
      process.env.API_PUBLIC_URL ?? "http://host.docker.internal:3001",
    jwtEnabled: process.env.ONLYOFFICE_JWT_ENABLED !== "false",
    jwtSecret: resolveOnlyofficeJwtSecret(),
    pluginUrl:
      process.env.ONLYOFFICE_PLUGIN_URL ??
      `${process.env.WEB_ORIGIN ?? "http://localhost:5173"}/onlyoffice-plugin/`,
    openTokenTtlSeconds: Number(process.env.EDITOR_OPEN_TOKEN_TTL ?? 900),
    downloadTokenTtlSeconds: Number(
      process.env.EDITOR_DOWNLOAD_TOKEN_TTL ?? 300,
    ),
    callbackTokenTtlSeconds: Number(
      process.env.EDITOR_CALLBACK_TTL ?? 7200,
    ),
  },
};
