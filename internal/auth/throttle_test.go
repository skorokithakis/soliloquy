package auth

import (
	"sync"
	"testing"
	"time"
)

func TestThrottleDoublesAcrossWaitsAndPrunesAfterIdle(t *testing.T) {
	now := time.Unix(1000, 0)
	throttle := newThrottle(func() time.Time { return now }, time.Second, time.Minute, 5*time.Minute)
	const username = "alice"

	// Each real failure doubles the next wait, even when the previous wait was
	// respected before the next attempt.
	wants := []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		time.Minute,
		time.Minute,
	}
	for i, want := range wants {
		allowed, remaining := throttle.Allowed(username)
		if !allowed {
			t.Fatalf("attempt %d was throttled for %v", i, remaining)
		}
		if got := throttle.Fail(username); got != want {
			t.Fatalf("failure %d wait = %v, want %v", i+1, got, want)
		}

		// Move to exactly when the next attempt is allowed.
		now = now.Add(want)
	}

	// Tries made during a wait are rejected and do not extend the wait.
	if allowed, _ := throttle.Allowed(username); !allowed {
		t.Fatal("attempt after the wait ended was throttled")
	}
	if got := throttle.Fail(username); got != time.Minute {
		t.Fatalf("wait after cap = %v, want %v", got, time.Minute)
	}
	now = now.Add(400 * time.Millisecond)
	allowed, remaining := throttle.Allowed(username)
	if allowed {
		t.Fatal("attempt during the wait was allowed")
	}
	if remaining != 59*time.Second+600*time.Millisecond {
		t.Errorf("remaining = %v, want %v", remaining, 59*time.Second+600*time.Millisecond)
	}
	// A second rejected try must not change the remaining wait.
	if allowed, again := throttle.Allowed(username); allowed || again != remaining {
		t.Errorf("repeated try = (%v, %v), want (false, %v)", allowed, again, remaining)
	}

	// After the idle period with no failed login the entry is forgotten, so
	// the next failure starts again from the base delay.
	now = now.Add(time.Minute)
	now = now.Add(5 * time.Minute)
	if allowed, _ := throttle.Allowed(username); !allowed {
		t.Fatal("username was still throttled after its idle period")
	}
	if got := throttle.Fail(username); got != time.Second {
		t.Fatalf("wait after idle pruning = %v, want %v", got, time.Second)
	}
}

func TestThrottleSuccessResetsWait(t *testing.T) {
	throttle := newThrottle(func() time.Time { return time.Unix(1000, 0) }, time.Second, time.Minute, 5*time.Minute)
	const username = "alice"

	if allowed, _ := throttle.Allowed(username); !allowed {
		t.Fatal("fresh username was throttled")
	}
	throttle.Fail(username)
	throttle.Success(username)

	if allowed, _ := throttle.Allowed(username); !allowed {
		t.Fatal("username was throttled after a successful login")
	}
	throttle.Release(username)
}

func TestThrottleConcurrentAttemptsReserveOne(t *testing.T) {
	const username = "alice"
	const attempts = 50

	throttle := newThrottle(time.Now, time.Second, time.Minute, 5*time.Minute)
	start := make(chan struct{})
	results := make(chan bool, attempts)

	var workers sync.WaitGroup
	workers.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer workers.Done()
			<-start
			allowed, _ := throttle.Allowed(username)
			results <- allowed
		}()
	}

	close(start)
	workers.Wait()
	close(results)

	allowedCount := 0
	for allowed := range results {
		if allowed {
			allowedCount++
		}
	}
	if allowedCount != 1 {
		t.Fatalf("concurrent attempts allowed = %d, want 1", allowedCount)
	}

	// Releasing the in-flight attempt lets the next one in.
	throttle.Release(username)
	if allowed, _ := throttle.Allowed(username); !allowed {
		t.Fatal("attempt was throttled after the in-flight attempt finished")
	}
}
