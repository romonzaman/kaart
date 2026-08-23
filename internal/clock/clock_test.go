package clock_test

import (
	"testing"
	"time"

	"github.com/romonzaman/kaart/internal/clock"
)

func TestRealNowIsUTC(t *testing.T) {
	got := clock.New().Now()
	if got.Location() != time.UTC {
		t.Fatalf("Real.Now() location = %v, want UTC", got.Location())
	}
	if time.Since(got) > time.Minute {
		t.Fatalf("Real.Now() = %v, which is not close to now", got)
	}
}

func TestFake(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := clock.NewFake(start)

	if !f.Now().Equal(start) {
		t.Fatalf("Now() = %v, want %v", f.Now(), start)
	}

	f.Advance(90 * time.Minute)
	want := start.Add(90 * time.Minute)
	if !f.Now().Equal(want) {
		t.Fatalf("after Advance, Now() = %v, want %v", f.Now(), want)
	}

	reset := time.Date(2030, 12, 31, 23, 0, 0, 0, time.UTC)
	f.Set(reset)
	if !f.Now().Equal(reset) {
		t.Fatalf("after Set, Now() = %v, want %v", f.Now(), reset)
	}
}

func TestFakeNormalisesToUTC(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	local := time.Date(2026, 5, 5, 10, 0, 0, 0, loc)
	f := clock.NewFake(local)

	if f.Now().Location() != time.UTC {
		t.Fatalf("Fake.Now() location = %v, want UTC", f.Now().Location())
	}
	if !f.Now().Equal(local) {
		t.Fatalf("Fake.Now() = %v, should be the same instant as %v", f.Now(), local)
	}
}

// Fake must satisfy Clock.
var _ clock.Clock = (*clock.Fake)(nil)
