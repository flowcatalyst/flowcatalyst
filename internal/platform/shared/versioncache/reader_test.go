package versioncache

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	values  map[string]time.Time
	getErr  error
	getCall int
	bumps   []string
}

func newFakeStore() *fakeStore { return &fakeStore{values: map[string]time.Time{}} }

func (s *fakeStore) Bump(_ context.Context, principalID string, at time.Time) {
	s.values[principalID] = at
	s.bumps = append(s.bumps, principalID)
}

func (s *fakeStore) Get(_ context.Context, principalID string) (time.Time, bool, error) {
	s.getCall++
	if s.getErr != nil {
		return time.Time{}, false, s.getErr
	}
	v, ok := s.values[principalID]
	return v, ok, nil
}

func TestReader_FallbackOnColdCache(t *testing.T) {
	store := newFakeStore()
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fallbackCalls := 0
	fallback := func(context.Context, string) (time.Time, error) {
		fallbackCalls++
		return want, nil
	}
	r := NewReader(store, 10, time.Minute, fallback)

	got, err := r.Version(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if fallbackCalls != 1 {
		t.Errorf("expected 1 fallback call, got %d", fallbackCalls)
	}
	if len(store.bumps) != 1 || store.bumps[0] != "p1" {
		t.Errorf("expected fallback result to be written through to store, got bumps=%v", store.bumps)
	}
}

func TestReader_LocalCacheHitSkipsStoreAndFallback(t *testing.T) {
	store := newFakeStore()
	fallbackCalls := 0
	fallback := func(context.Context, string) (time.Time, error) {
		fallbackCalls++
		return time.Now(), nil
	}
	r := NewReader(store, 10, time.Minute, fallback)

	first, err := r.Version(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	storeGetsAfterFirst := store.getCall

	second, err := r.Version(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if !second.Equal(first) {
		t.Errorf("second read %v != first %v", second, first)
	}
	if fallbackCalls != 1 {
		t.Errorf("expected fallback called once (local cache should serve the second read), got %d", fallbackCalls)
	}
	if store.getCall != storeGetsAfterFirst {
		t.Errorf("expected no additional store.Get calls once local cache is warm")
	}
}

func TestReader_StoreHitPopulatesLocalCacheWithoutFallback(t *testing.T) {
	store := newFakeStore()
	want := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store.values["p1"] = want
	fallback := func(context.Context, string) (time.Time, error) {
		t.Fatal("fallback should not be called on a store hit")
		return time.Time{}, nil
	}
	r := NewReader(store, 10, time.Minute, fallback)

	got, err := r.Version(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReader_StoreErrorFallsBackToSourceOfTruth(t *testing.T) {
	store := newFakeStore()
	store.getErr = errors.New("redis down")
	want := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	fallback := func(context.Context, string) (time.Time, error) { return want, nil }
	r := NewReader(store, 10, time.Minute, fallback)

	got, err := r.Version(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v (should degrade to fallback on store error)", got, want)
	}
}

func TestReader_LocalTTLExpiryRechecksStore(t *testing.T) {
	store := newFakeStore()
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	store.values["p1"] = older
	fallback := func(context.Context, string) (time.Time, error) {
		t.Fatal("fallback should not be needed; store always has a value")
		return time.Time{}, nil
	}
	r := NewReader(store, 10, 10*time.Millisecond, fallback)

	got, err := r.Version(context.Background(), "p1")
	if err != nil || !got.Equal(older) {
		t.Fatalf("first read = %v, %v", got, err)
	}

	store.values["p1"] = newer
	time.Sleep(30 * time.Millisecond)

	got, err = r.Version(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if !got.Equal(newer) {
		t.Errorf("after local TTL expiry, got %v, want refreshed %v", got, newer)
	}
}
