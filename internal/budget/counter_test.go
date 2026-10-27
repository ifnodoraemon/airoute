package budget

import (
	"context"
	"errors"
	"testing"
	"time"
)

func day(year int, month time.Month, dayOfMonth int) time.Time {
	return time.Date(year, month, dayOfMonth, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
}

func TestMemoryCounterPeriodRolloverAndReset(t *testing.T) {
	counter := NewMemoryCounter()
	ctx := context.Background()
	first := day(2026, time.October, 27)
	next := day(2026, time.October, 28)

	if used, err := counter.Get(ctx, 7, PeriodDaily, first); err != nil || used != 0 {
		t.Fatalf("empty get = %d %v", used, err)
	}
	if used, err := counter.Add(ctx, 7, PeriodDaily, first, 500); err != nil || used != 500 {
		t.Fatalf("add = %d %v", used, err)
	}
	if used, _ := counter.Add(ctx, 7, PeriodDaily, first, -100); used != 400 {
		t.Fatalf("negative delta = %d", used)
	}
	if used, _ := counter.Add(ctx, 7, PeriodDaily, first, -1000); used != 0 {
		t.Fatalf("counter must not go below zero: %d", used)
	}
	if used, _ := counter.Get(ctx, 7, PeriodDaily, next); used != 0 {
		t.Fatalf("next day counter = %d", used)
	}

	// A total budget spans days.
	_, _ = counter.Add(ctx, 8, PeriodTotal, first, 10)
	if used, _ := counter.Get(ctx, 8, PeriodTotal, next); used != 10 {
		t.Fatalf("total counter lost usage: %d", used)
	}

	if err := counter.Reset(ctx, 7, PeriodDaily, first); err != nil {
		t.Fatal(err)
	}
	if used, _ := counter.Get(ctx, 7, PeriodDaily, first); used != 0 {
		t.Fatalf("reset left %d", used)
	}
}

func TestCounterKeyUsesBusinessDate(t *testing.T) {
	at := day(2026, time.October, 27)
	if got := CounterKey(12, PeriodDaily, at); got != "airoute:budget:12:2026-10-27" {
		t.Fatalf("daily key = %q", got)
	}
	if got := CounterKey(12, PeriodMonthly, at); got != "airoute:budget:12:2026-10" {
		t.Fatalf("monthly key = %q", got)
	}
	if got := CounterKey(12, PeriodTotal, at); got != "airoute:budget:12:total" {
		t.Fatalf("total key = %q", got)
	}
}

type fakeKV struct {
	values map[string]int64
	ttls   map[string]time.Duration
	err    error
}

func newFakeKV() *fakeKV {
	return &fakeKV{values: map[string]int64{}, ttls: map[string]time.Duration{}}
}

func (f *fakeKV) IncrByWithTTL(_ context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.values[key] += delta
	f.ttls[key] = ttl
	return f.values[key], nil
}

func (f *fakeKV) GetInt(_ context.Context, key string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.values[key], nil
}

func (f *fakeKV) Delete(_ context.Context, key string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.values, key)
	return nil
}

func TestKVCounterUsesPeriodTTL(t *testing.T) {
	kv := newFakeKV()
	counter := NewKVCounter(kv)
	ctx := context.Background()
	at := day(2026, time.October, 27)

	if used, err := counter.Add(ctx, 3, PeriodDaily, at, 200); err != nil || used != 200 {
		t.Fatalf("add = %d %v", used, err)
	}
	if kv.ttls["airoute:budget:3:2026-10-27"] != 48*time.Hour {
		t.Fatalf("daily ttl = %v", kv.ttls["airoute:budget:3:2026-10-27"])
	}
	if _, err := counter.Add(ctx, 3, PeriodMonthly, at, 1); err != nil {
		t.Fatal(err)
	}
	if kv.ttls["airoute:budget:3:2026-10"] != 45*24*time.Hour {
		t.Fatalf("monthly ttl = %v", kv.ttls["airoute:budget:3:2026-10"])
	}
	if _, err := counter.Add(ctx, 3, PeriodTotal, at, 1); err != nil {
		t.Fatal(err)
	}
	if kv.ttls["airoute:budget:3:total"] != 0 {
		t.Fatalf("total ttl = %v", kv.ttls["airoute:budget:3:total"])
	}

	// The value is shared by budget+period, not by caller.
	if used, _ := counter.Get(ctx, 3, PeriodDaily, at); used != 200 {
		t.Fatalf("get = %d", used)
	}
	if err := counter.Reset(ctx, 3, PeriodDaily, at); err != nil {
		t.Fatal(err)
	}
	if used, _ := counter.Get(ctx, 3, PeriodDaily, at); used != 0 {
		t.Fatalf("reset get = %d", used)
	}
}

func TestKVCounterFailsLoudlyWithoutBackend(t *testing.T) {
	counter := NewKVCounter(nil)
	if _, err := counter.Get(context.Background(), 1, PeriodDaily, day(2026, time.October, 27)); err == nil {
		t.Fatal("nil backend must fail")
	}
	failing := NewKVCounter(&fakeKV{err: errors.New("redis down")})
	if _, err := failing.Add(context.Background(), 1, PeriodDaily, day(2026, time.October, 27), 1); err == nil {
		t.Fatal("backend error must propagate")
	}
}

func TestPeriodTTL(t *testing.T) {
	if PeriodDaily.TTL() != 48*time.Hour || PeriodMonthly.TTL() != 45*24*time.Hour || PeriodTotal.TTL() != 0 {
		t.Fatal("unexpected period TTLs")
	}
}
