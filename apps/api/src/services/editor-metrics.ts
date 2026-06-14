let callbackAttempts = 0;
let callbackSuccesses = 0;
let callbackFailures = 0;

export function recordCallbackAttempt(): void {
  callbackAttempts += 1;
}

export function recordCallbackSuccess(): void {
  callbackSuccesses += 1;
}

export function recordCallbackFailure(): void {
  callbackFailures += 1;
}

export function getCallbackMetrics() {
  const rate =
    callbackAttempts === 0
      ? 1
      : callbackSuccesses / callbackAttempts;
  return {
    callbackAttempts,
    callbackSuccesses,
    callbackFailures,
    callbackSuccessRate: rate,
    callbackSuccessRatePercent: Math.round(rate * 10000) / 100,
    meetsTarget: rate >= 0.99 || callbackAttempts === 0,
  };
}
