import dotenv from "dotenv";
import { resolve } from "node:path";

dotenv.config({ path: resolve(process.cwd(), "../../.env") });
dotenv.config();

export type StorageBackend = "minio" | "s3";

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
};
