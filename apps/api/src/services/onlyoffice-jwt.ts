import jwt from "jsonwebtoken";
import { config } from "../config.js";

export function signOnlyofficePayload(payload: Record<string, unknown>): string {
  if (!config.onlyoffice.jwtEnabled) {
    return "";
  }
  return jwt.sign(payload, config.onlyoffice.jwtSecret, { expiresIn: "1h" });
}

export function verifyOnlyofficeToken(token: string): Record<string, unknown> | null {
  if (!config.onlyoffice.jwtEnabled) {
    return {};
  }
  try {
    return jwt.verify(token, config.onlyoffice.jwtSecret) as Record<string, unknown>;
  } catch {
    return null;
  }
}

export function wrapOnlyofficeConfig(
  editorConfig: Record<string, unknown>,
): Record<string, unknown> {
  if (!config.onlyoffice.jwtEnabled) {
    return editorConfig;
  }
  const token = signOnlyofficePayload(editorConfig);
  return { ...editorConfig, token };
}
