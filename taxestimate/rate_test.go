package taxestimate

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeProvider is a hand-written RateProvider for service tests.
type fakeProvider struct {
	result RateResult
	err    error
	calls  int
}

func (f *fakeProvider) Rate(_ context.Context, zip string) (RateResult, error) {
	f.calls++
	if f.err != nil {
		return RateResult{}, f.err
	}
	r := f.result
	r.Zip = zip
	return r, nil
}

// fakeCache is a hand-written RateCache for service tests.
type fakeCache struct {
	store    map[string]RateResult
	getErr   error
	setErr   error
	setCalls int
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string]RateResult{}} }

func (f *fakeCache) GetRate(_ context.Context, zip string) (RateResult, bool, error) {
	if f.getErr != nil {
		return RateResult{}, false, f.getErr
	}
	r, ok := f.store[zip]
	return r, ok, nil
}

func (f *fakeCache) SetRate(_ context.Context, zip string, result RateResult) error {
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
	}
	f.store[zip] = result
	return nil
}

var fixedNow = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

func newTestService(p RateProvider, c RateCache) *RateService {
	s := NewRateService(p, c)
	s.now = func() time.Time { return fixedNow }
	return s
}

func TestLookupRate(t *testing.T) {
	tests := []struct {
		name           string
		zip            string
		provider       *fakeProvider
		useNilProvider bool
		useNilCache    bool
		setupCache     func(*fakeCache)
		check          func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache)
	}{
		{
			name:     "cache miss calls provider and writes through",
			zip:      "77002",
			provider: &fakeProvider{result: RateResult{CombinedRate: 0.0825, Jurisdictions: "Breakdown: state 6.250%"}},
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if provider.calls != 1 {
					t.Errorf("provider calls = %d, want 1", provider.calls)
				}
				if got.Estimated {
					t.Errorf("result should not be flagged on a successful lookup: %+v", got)
				}
				if got.Cached {
					t.Errorf("result.Cached = true, want false on a miss")
				}
				if got.CombinedRate != 0.0825 {
					t.Errorf("CombinedRate = %v, want 0.0825", got.CombinedRate)
				}
				if got.Zip != "77002" {
					t.Errorf("Zip = %q, want 77002", got.Zip)
				}
				if !got.FetchedAt.Equal(fixedNow) {
					t.Errorf("FetchedAt = %v, want %v", got.FetchedAt, fixedNow)
				}
				if cache.setCalls != 1 {
					t.Errorf("cache SetRate calls = %d, want 1", cache.setCalls)
				}
				if _, ok := cache.store["77002"]; !ok {
					t.Errorf("result was not written to cache")
				}
			},
		},
		{
			name:     "cache hit skips the provider",
			zip:      "77002-4567",
			provider: &fakeProvider{result: RateResult{CombinedRate: 0.0825}},
			setupCache: func(c *fakeCache) {
				c.store["77002"] = RateResult{Zip: "77002", CombinedRate: 0.0625, Jurisdictions: "cached"}
			},
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if provider.calls != 0 {
					t.Errorf("provider calls = %d, want 0 on a cache hit", provider.calls)
				}
				if !got.Cached {
					t.Errorf("result.Cached = false, want true on a hit")
				}
				if got.CombinedRate != 0.0625 {
					t.Errorf("CombinedRate = %v, want 0.0625 (cached value)", got.CombinedRate)
				}
				if cache.setCalls != 0 {
					t.Errorf("cache SetRate calls = %d, want 0 on a hit", cache.setCalls)
				}
			},
		},
		{
			name:     "invalid ZIP returns a flagged estimate without calling the provider",
			zip:      "not-a-zip",
			provider: &fakeProvider{result: RateResult{CombinedRate: 0.0825}},
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if !got.Estimated {
					t.Errorf("result.Estimated = false, want true for an invalid ZIP")
				}
				if got.Warning == "" {
					t.Errorf("expected a non-blocking warning for an invalid ZIP")
				}
				if got.CombinedRate != 0 {
					t.Errorf("CombinedRate = %v, want 0 for a flagged estimate", got.CombinedRate)
				}
				if provider.calls != 0 {
					t.Errorf("provider should not be called for an invalid ZIP, calls = %d", provider.calls)
				}
			},
		},
		{
			name:     "provider failure returns a flagged estimate and caches nothing",
			zip:      "77002",
			provider: &fakeProvider{err: errors.New("taxjar down")},
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if !got.Estimated {
					t.Errorf("result.Estimated = false, want true on provider failure")
				}
				if got.Warning == "" {
					t.Errorf("expected a non-blocking warning on provider failure")
				}
				if got.Zip != "77002" {
					t.Errorf("Zip = %q, want the normalized ZIP even on failure", got.Zip)
				}
				if cache.setCalls != 0 {
					t.Errorf("nothing should be cached on a provider failure, setCalls = %d", cache.setCalls)
				}
			},
		},
		{
			name:       "cache read error falls through to the provider",
			zip:        "77002",
			provider:   &fakeProvider{result: RateResult{CombinedRate: 0.0825}},
			setupCache: func(c *fakeCache) { c.getErr = errors.New("redis timeout") },
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if provider.calls != 1 {
					t.Errorf("provider calls = %d, want 1 (cache error treated as miss)", provider.calls)
				}
				if got.Estimated {
					t.Errorf("a cache read error must not flag the result: %+v", got)
				}
				if got.CombinedRate != 0.0825 {
					t.Errorf("CombinedRate = %v, want 0.0825", got.CombinedRate)
				}
			},
		},
		{
			name:       "cache write error still returns the rate",
			zip:        "77002",
			provider:   &fakeProvider{result: RateResult{CombinedRate: 0.0825}},
			setupCache: func(c *fakeCache) { c.setErr = errors.New("redis write failed") },
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if got.Estimated {
					t.Errorf("a cache write failure must not flag or block the result: %+v", got)
				}
				if got.CombinedRate != 0.0825 {
					t.Errorf("CombinedRate = %v, want 0.0825 despite the cache write failure", got.CombinedRate)
				}
			},
		},
		{
			name:           "nil provider returns a flagged estimate",
			zip:            "77002",
			useNilProvider: true,
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if !got.Estimated {
					t.Errorf("result.Estimated = false, want true with a nil provider")
				}
				if got.Zip != "77002" {
					t.Errorf("Zip = %q, want the normalized ZIP", got.Zip)
				}
			},
		},
		{
			name:        "nil cache uses the provider directly",
			zip:         "77002",
			provider:    &fakeProvider{result: RateResult{CombinedRate: 0.0825}},
			useNilCache: true,
			check: func(t *testing.T, got RateResult, provider *fakeProvider, cache *fakeCache) {
				if provider.calls != 1 {
					t.Errorf("provider calls = %d, want 1 with a nil cache", provider.calls)
				}
				if got.Estimated || got.CombinedRate != 0.0825 {
					t.Errorf("unexpected result with nil cache: %+v", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newFakeCache()
			if tt.setupCache != nil {
				tt.setupCache(cache)
			}
			// Pass true nil interfaces (not a typed nil) for the nil-dependency cases
			// so the service sees the dependency as absent.
			var provider RateProvider
			if !tt.useNilProvider {
				provider = tt.provider
			}
			var rateCache RateCache = cache
			if tt.useNilCache {
				rateCache = nil
			}
			svc := newTestService(provider, rateCache)

			got := svc.LookupRate(context.Background(), tt.zip)
			tt.check(t, got, tt.provider, cache)
		})
	}
}
