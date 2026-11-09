package auth

import (
	"sync"
	"time"
)

const (
	defaultThrottleBase = time.Second
	defaultThrottleMax  = time.Minute
	// defaultThrottleIdle is how long a username must have had no failed
	// login before its entry is forgotten. Keeping the entry across the short
	// waits is what makes successive failures double the delay.
	defaultThrottleIdle = 5 * time.Minute

	// inFlightWait is reported when another attempt for the same username is
	// already running. It is only a hint for Retry-After.
	inFlightWait = time.Second
)

// Throttle tracks failed login attempts per username in memory. After every
// real failure the next attempt is delayed for twice as long, up to a cap. A
// successful login clears the entry. Tries made during a wait are rejected
// without changing the wait. Entries are forgotten after a period with no
// failed login so memory use stays small.
//
// Allowed reserves one in-flight attempt per username. Call Fail, Success or
// Release to finish a reserved attempt; otherwise further attempts for that
// username are rejected.
type Throttle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
	now     func() time.Time
	base    time.Duration
	max     time.Duration
	idle    time.Duration
}

type throttleEntry struct {
	attempts    int
	nextAllowed time.Time
	lastFailure time.Time
	inFlight    bool
}

// NewThrottle returns a throttle with a one-second base delay, a one-minute
// cap and a five-minute idle timeout.
func NewThrottle() *Throttle {
	return newThrottle(time.Now, defaultThrottleBase, defaultThrottleMax, defaultThrottleIdle)
}

func newThrottle(now func() time.Time, base, max, idle time.Duration) *Throttle {
	return &Throttle{
		entries: make(map[string]*throttleEntry),
		now:     now,
		base:    base,
		max:     max,
		idle:    idle,
	}
}

// Allowed reserves one in-flight login attempt for username. It returns false
// when the username is waiting out a throttle or another attempt is already in
// flight. The returned duration is how long the caller should wait.
func (t *Throttle) Allowed(username string) (bool, time.Duration) {
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	t.pruneLocked(now)

	entry, ok := t.entries[username]
	if !ok {
		t.entries[username] = &throttleEntry{inFlight: true}
		return true, 0
	}
	if entry.inFlight {
		return false, inFlightWait
	}
	if now.Before(entry.nextAllowed) {
		return false, entry.nextAllowed.Sub(now)
	}

	entry.inFlight = true
	return true, 0
}

// Fail records a real failed login and doubles the wait before the next
// attempt. It finishes the reserved attempt and returns the new wait.
func (t *Throttle) Fail(username string) time.Duration {
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	t.pruneLocked(now)

	entry, ok := t.entries[username]
	if !ok {
		entry = &throttleEntry{}
		t.entries[username] = entry
	}

	entry.attempts++
	wait := t.base
	for i := 1; i < entry.attempts && wait < t.max; i++ {
		wait *= 2
	}
	if wait > t.max {
		wait = t.max
	}

	entry.nextAllowed = now.Add(wait)
	entry.lastFailure = now
	entry.inFlight = false
	return wait
}

// Success clears the failure history for a username and finishes the reserved
// attempt.
func (t *Throttle) Success(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.entries, username)
}

// Release finishes a reserved attempt without recording a failure or a
// success. Handlers use it when they cannot complete the login for reasons
// unrelated to the credentials.
func (t *Throttle) Release(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	entry, ok := t.entries[username]
	if !ok {
		return
	}
	entry.inFlight = false
	// An entry with no failures carries no wait state and can be dropped.
	if entry.attempts == 0 {
		delete(t.entries, username)
	}
}

func (t *Throttle) pruneLocked(now time.Time) {
	for username, entry := range t.entries {
		if entry.inFlight {
			continue
		}
		if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) >= t.idle {
			delete(t.entries, username)
		}
	}
}
