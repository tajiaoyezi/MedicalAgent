import { createHash } from "node:crypto";
import {
  CreateBucketCommand,
  DeleteObjectCommand,
  GetObjectCommand,
  HeadBucketCommand,
  HeadObjectCommand,
  PutObjectCommand,
  S3Client,
} from "@aws-sdk/client-s3";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";
import { config } from "../config.js";

export interface ObjectStorage {
  put(key: string, body: Buffer, contentType?: string): Promise<void>;
  get(key: string): Promise<Buffer>;
  delete(key: string): Promise<void>;
  headObject(key: string): Promise<{ size: number; contentType?: string }>;
  presignedUrl(key: string, expiresInSeconds?: number): Promise<string>;
}

function buildKey(
  tenantId: string,
  documentId: string,
  versionId: string,
): string {
  return `${tenantId}/${documentId}/${versionId}`;
}

export function objectKeyForVersion(
  tenantId: string,
  documentId: string,
  versionId: string,
): string {
  return buildKey(tenantId, documentId, versionId);
}

export function computeFileHash(content: Buffer): string {
  return createHash("sha256").update(content).digest("hex");
}

export function createObjectStorage(): ObjectStorage {
  const { endpoint, accessKey, secretKey, bucket, region, forcePathStyle } =
    config.storage;

  const client = new S3Client({
    region,
    endpoint,
    forcePathStyle,
    credentials: {
      accessKeyId: accessKey ?? "minioadmin",
      secretAccessKey: secretKey ?? "minioadmin",
    },
  });

  async function ensureBucket() {
    try {
      await client.send(new HeadBucketCommand({ Bucket: bucket }));
    } catch {
      await client.send(new CreateBucketCommand({ Bucket: bucket }));
    }
  }

  return {
    async put(key, body, contentType) {
      await ensureBucket();
      await client.send(
        new PutObjectCommand({
          Bucket: bucket,
          Key: key,
          Body: body,
          ContentType: contentType ?? "application/octet-stream",
        }),
      );
    },

    async get(key) {
      const res = await client.send(
        new GetObjectCommand({ Bucket: bucket, Key: key }),
      );
      const chunks: Buffer[] = [];
      for await (const chunk of res.Body as AsyncIterable<Uint8Array>) {
        chunks.push(Buffer.from(chunk));
      }
      const body = Buffer.concat(chunks);
      return body;
    },

    async delete(key) {
      await client.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }));
    },

    async headObject(key) {
      const res = await client.send(
        new HeadObjectCommand({ Bucket: bucket, Key: key }),
      );
      return {
        size: res.ContentLength ?? 0,
        contentType: res.ContentType,
      };
    },

    async presignedUrl(key, expiresInSeconds = 300) {
      const command = new GetObjectCommand({ Bucket: bucket, Key: key });
      return getSignedUrl(client, command, { expiresIn: expiresInSeconds });
    },
  };
}
