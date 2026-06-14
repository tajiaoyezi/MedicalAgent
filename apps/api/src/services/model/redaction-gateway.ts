// c03｜公网调用前置 PHI/PII 脱敏门禁「接缝」（design D6）
// 唯一 owner=c09（phase 9）。本期仅预留接缝 + 公网默认拒绝的保守降级：
// redaction-gateway 未接入前，对公网 provider 一律按「识别服务不可用」处理而拒绝/降级。
// c09 落地时只需替换 redactionGateway 实现（真实 PHI/PII 识别脱敏），router/visual 调用点不变。

export interface RedactionInput {
  tenantId: string;
  /** 待外发原文（公网调用前由本门禁判定/脱敏） */
  text: string;
}

export interface RedactionVerdict {
  /** c09 识别服务是否可用（本期恒 false：c09 未接入） */
  available: boolean;
  /** 识别+脱敏是否通过且置信度达标 */
  passed: boolean;
  confidence: number;
  /** 通过时的脱敏后文本（公网以此外发） */
  redactedText?: string;
  reason: string;
}

export interface RedactionGateway {
  evaluate(input: RedactionInput): Promise<RedactionVerdict>;
}

/**
 * 本期默认实现：c09 redaction-gateway 未接入 → 公网默认拒绝。
 * 始终返回 available=false / passed=false，使 router 跳过公网 provider、改走私有化或拒绝。
 */
export const redactionGateway: RedactionGateway = {
  async evaluate(): Promise<RedactionVerdict> {
    return {
      available: false,
      passed: false,
      confidence: 0,
      reason:
        "c09 redaction-gateway 未接入（phase 9 落地）；本期公网调用按识别服务不可用处理、默认拒绝/降级私有化",
    };
  },
};
