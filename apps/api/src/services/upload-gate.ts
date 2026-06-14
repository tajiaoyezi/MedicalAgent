export interface UploadGateResult {
  allowed: boolean;
  failureReason?: string;
}

/**
 * c09 redaction-gateway 上传闸接缝：可插拔、缺省放行。
 * c09 未接入时按 POC 默认策略放行。
 */
export function checkUploadGate(_fileName: string, _buffer: Buffer): UploadGateResult {
  // 占位：c09 接入后替换为真实 PHI/PII 检测
  return { allowed: true };
}

export function isRedactionGatewayAvailable(): boolean {
  return false;
}
