// c03｜model_providers 凭据加解密与掩码（task 1.1 敏感字段加密/掩码存储、task 8.3 掩码返回）
import {
  createCipheriv,
  createDecipheriv,
  createHash,
  randomBytes,
} from "node:crypto";
import { config } from "../../config.js";

const KEY = createHash("sha256")
  .update(config.model.credentialSecret)
  .digest(); // 32 bytes for aes-256-gcm

/** 加密凭据为 `iv.tag.ciphertext`（均 base64）。空值返回 null（未配置凭据）。 */
export function encryptCredential(plain: string | null | undefined): string | null {
  if (!plain) return null;
  const iv = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", KEY, iv);
  const enc = Buffer.concat([cipher.update(plain, "utf8"), cipher.final()]);
  const tag = cipher.getAuthTag();
  return [iv.toString("base64"), tag.toString("base64"), enc.toString("base64")].join(".");
}

/** 解密凭据；密文非法或为空返回空串。仅在向 provider 实际发请求时调用，绝不回前端。 */
export function decryptCredential(stored: string | null | undefined): string {
  if (!stored) return "";
  const parts = stored.split(".");
  if (parts.length !== 3) return "";
  try {
    const [ivB64, tagB64, dataB64] = parts;
    const decipher = createDecipheriv(
      "aes-256-gcm",
      KEY,
      Buffer.from(ivB64, "base64"),
    );
    decipher.setAuthTag(Buffer.from(tagB64, "base64"));
    return Buffer.concat([
      decipher.update(Buffer.from(dataB64, "base64")),
      decipher.final(),
    ]).toString("utf8");
  } catch {
    return "";
  }
}

/** 掩码：仅暴露是否已配置 + 末尾少量字符，绝不返回明文。 */
export function maskCredential(stored: string | null | undefined): string | null {
  if (!stored) return null;
  const plain = decryptCredential(stored);
  if (!plain) return "****";
  if (plain.length <= 6) return "****";
  return `${plain.slice(0, 3)}****${plain.slice(-2)}`;
}
