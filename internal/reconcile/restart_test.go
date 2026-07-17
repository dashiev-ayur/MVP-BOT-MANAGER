package reconcile

import (
	"testing"
	"time"
)

func TestRestartTracker_BackoffThenAllow(t *testing.T) {
	tr := newRestartTracker()
	p := RestartPolicy{MaxAttempts: 3, Base: time.Second, Max: 8 * time.Second}
	now := time.Unix(1000, 0)

	allow, wait, attempt := tr.recordFailure("rt1", p, now)
	if allow || wait != time.Second || attempt != 1 {
		t.Fatalf("first: allow=%v wait=%v attempt=%d", allow, wait, attempt)
	}

	// Слишком рано.
	allow, wait, _ = tr.recordFailure("rt1", p, now.Add(500*time.Millisecond))
	if allow || wait <= 0 {
		t.Fatalf("early: allow=%v wait=%v", allow, wait)
	}

	// После backoff — разрешаем.
	allow, wait, attempt = tr.recordFailure("rt1", p, now.Add(time.Second))
	if !allow || wait != 0 || attempt != 1 {
		t.Fatalf("after wait: allow=%v wait=%v attempt=%d", allow, wait, attempt)
	}

	// Новый краш → attempt 2, backoff 2s.
	allow, wait, attempt = tr.recordFailure("rt1", p, now.Add(2*time.Second))
	if allow || wait != 2*time.Second || attempt != 2 {
		t.Fatalf("second fail: allow=%v wait=%v attempt=%d", allow, wait, attempt)
	}
}

func TestRestartTracker_Exhausted(t *testing.T) {
	tr := newRestartTracker()
	p := RestartPolicy{MaxAttempts: 1, Base: time.Millisecond, Max: time.Second}
	now := time.Unix(2000, 0)

	_, _, _ = tr.recordFailure("rt", p, now)                    // schedule #1
	allow, _, _ := tr.recordFailure("rt", p, now.Add(time.Millisecond)) // do #1
	if !allow {
		t.Fatal("expected allow for attempt 1")
	}
	// Ещё один краш — лимит 1, не планируем.
	allow, wait, attempt := tr.recordFailure("rt", p, now.Add(2*time.Millisecond))
	if allow || wait != 0 || attempt != 1 {
		t.Fatalf("exhausted: allow=%v wait=%v attempt=%d", allow, wait, attempt)
	}
}

func TestRestartTracker_Disabled(t *testing.T) {
	tr := newRestartTracker()
	p := RestartPolicy{MaxAttempts: 0, Base: time.Second, Max: time.Second}
	allow, wait, _ := tr.recordFailure("rt", p, time.Now())
	if allow || wait != 0 {
		t.Fatalf("disabled: allow=%v wait=%v", allow, wait)
	}
}

func TestRestartPolicy_BackoffCap(t *testing.T) {
	p := RestartPolicy{Base: time.Second, Max: 5 * time.Second}
	if p.backoffFor(1) != time.Second {
		t.Fatal(p.backoffFor(1))
	}
	if p.backoffFor(2) != 2*time.Second {
		t.Fatal(p.backoffFor(2))
	}
	if p.backoffFor(10) != 5*time.Second {
		t.Fatalf("cap: %v", p.backoffFor(10))
	}
}

func TestRestartTracker_Reset(t *testing.T) {
	tr := newRestartTracker()
	p := RestartPolicy{MaxAttempts: 2, Base: time.Millisecond, Max: time.Second}
	now := time.Now()
	_, _, _ = tr.recordFailure("rt", p, now)
	tr.resetAfterSuccess("rt")
	allow, wait, attempt := tr.recordFailure("rt", p, now)
	if allow || attempt != 1 || wait != time.Millisecond {
		t.Fatalf("after reset: allow=%v wait=%v attempt=%d", allow, wait, attempt)
	}
}
