package editor

import (
	"math"
	"sync/atomic"
)

// Metrics 复刻 services/editor-metrics.ts（线程安全计数）。
type Metrics struct {
	attempts  atomic.Int64
	successes atomic.Int64
	failures  atomic.Int64
}

func NewMetrics() *Metrics { return &Metrics{} }

func (m *Metrics) RecordAttempt() { m.attempts.Add(1) }
func (m *Metrics) RecordSuccess() { m.successes.Add(1) }
func (m *Metrics) RecordFailure() { m.failures.Add(1) }

func (m *Metrics) Snapshot() map[string]any {
	a := m.attempts.Load()
	s := m.successes.Load()
	f := m.failures.Load()
	rate := 1.0
	if a != 0 {
		rate = float64(s) / float64(a)
	}
	return map[string]any{
		"callbackAttempts":           a,
		"callbackSuccesses":          s,
		"callbackFailures":           f,
		"callbackSuccessRate":        rate,
		"callbackSuccessRatePercent": math.Round(rate*10000) / 100,
		"meetsTarget":                rate >= 0.99 || a == 0,
	}
}
