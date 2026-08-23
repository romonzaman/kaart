// Package clock provides an injectable time source. Business logic must take a
// Clock rather than calling time.Now(), so scheduling behaviour is testable.
package clock

import (
	"sync"
	"time"
)

// Clock is a source of the current time. Implementations return UTC.
type Clock interface {
	Now() time.Time
}

// Real is the production clock.
type Real struct{}

// Now returns the current wall-clock time in UTC.
func (Real) Now() time.Time { return time.Now().UTC() }

// New returns the production clock.
func New() Clock { return Real{} }

// Fake is a controllable clock for tests. It is safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake clock pinned to t (converted to UTC).
func NewFake(t time.Time) *Fake {
	return &Fake{now: t.UTC()}
}

// Now returns the fake clock's current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set moves the fake clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t.UTC()
}

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
