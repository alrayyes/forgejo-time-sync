package ratelimit_test

import (
	"testing"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/ratelimit"
	"github.com/stretchr/testify/require"
)

// fakeClock lets pacer tests assert on sleep durations without a real clock
// ever running: Sleep both records what it was asked to wait for and
// advances Now by that amount, so the pacer sees time pass exactly as if it
// had really slept.
type fakeClock struct {
	now   time.Time
	slept []time.Duration
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Sleep(d time.Duration) {
	f.slept = append(f.slept, d)
	f.now = f.now.Add(d)
}

func TestWait(t *testing.T) {
	t.Parallel()

	t.Run("does not sleep on the very first call", func(t *testing.T) {
		t.Parallel()

		clock := &fakeClock{now: time.Now()}
		p := ratelimit.NewWithClock(time.Minute, clock.Now, clock.Sleep)

		p.Wait()

		require.Empty(t, clock.slept)
	})

	t.Run("sleeps for the full interval when called again immediately", func(t *testing.T) {
		t.Parallel()

		clock := &fakeClock{now: time.Now()}
		p := ratelimit.NewWithClock(time.Minute, clock.Now, clock.Sleep)
		p.Wait()

		p.Wait()

		require.Equal(t, []time.Duration{time.Minute}, clock.slept)
	})

	t.Run("does not sleep if the interval already elapsed", func(t *testing.T) {
		t.Parallel()

		clock := &fakeClock{now: time.Now()}
		p := ratelimit.NewWithClock(time.Minute, clock.Now, clock.Sleep)
		p.Wait()
		clock.now = clock.now.Add(2 * time.Minute)

		p.Wait()

		require.Empty(t, clock.slept)
	})

	t.Run("sleeps only for the remaining time when partially elapsed", func(t *testing.T) {
		t.Parallel()

		clock := &fakeClock{now: time.Now()}
		p := ratelimit.NewWithClock(time.Minute, clock.Now, clock.Sleep)
		p.Wait()
		clock.now = clock.now.Add(20 * time.Second)

		p.Wait()

		require.Equal(t, []time.Duration{40 * time.Second}, clock.slept)
	})
}
