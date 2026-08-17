// Package ratelimit paces calls to a spaced minimum interval, blocking the
// caller rather than rejecting the call.
package ratelimit

import (
	"sync"
	"time"
)

// Pacer enforces a minimum interval between successive calls to Wait.
type Pacer struct {
	interval time.Duration

	mu   sync.Mutex
	last time.Time

	now   func() time.Time
	sleep func(time.Duration)
}

// New builds a Pacer that lets calls through no more often than once per
// interval, using the real clock.
func New(interval time.Duration) *Pacer {
	return NewWithClock(interval, time.Now, time.Sleep)
}

// NewWithClock builds a Pacer against an injected clock, for deterministic
// tests of interval-spacing behavior without a real sleep.
func NewWithClock(interval time.Duration, now func() time.Time, sleep func(time.Duration)) *Pacer {
	return &Pacer{interval: interval, now: now, sleep: sleep}
}

// Wait blocks, if necessary, until at least interval has passed since the
// previous call to Wait. Held across the sleep deliberately: a Pacer paces
// one sequential caller, not concurrent ones.
func (p *Pacer) Wait() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if !p.last.IsZero() {
		if elapsed := now.Sub(p.last); elapsed < p.interval {
			p.sleep(p.interval - elapsed)
		}
	}
	p.last = p.now()
}
