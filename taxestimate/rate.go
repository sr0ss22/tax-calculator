package taxestimate

import (
	"context"
	"fmt"
	"time"
)

// RateResult is the combined sales tax rate for a ZIP, plus provenance the UI and
// audit need. The rate is an estimate; SAP remains the system of record for
// actual tax.
type RateResult struct {
	// Zip is the normalized 5-digit ZIP the rate applies to.
	Zip string `json:"zip"`
	// CombinedRate is the combined state+local rate as a fraction (0.0825 = 8.25%).
	CombinedRate float64 `json:"combined_rate"`
	// Jurisdictions is a human-readable breakdown, e.g. "Breakdown: state 6.250% + city 2.000%".
	Jurisdictions string `json:"jurisdictions"`
	City          string `json:"city"`
	County        string `json:"county"`
	// Cached is true when the rate was served from cache rather than the provider.
	Cached bool `json:"cached"`
	// FetchedAt is when the underlying provider value was fetched.
	FetchedAt time.Time `json:"fetched_at"`
	// Estimated is true when this is a flagged fallback (invalid ZIP or provider
	// failure) rather than a real provider rate. When set, CombinedRate is 0 and
	// Warning explains why; callers surface the warning without hard-blocking.
	Estimated bool `json:"estimated"`
	// Warning is a non-blocking message set when Estimated is true.
	Warning string `json:"warning,omitempty"`
}

// RateProvider returns the combined rate for a normalized ZIP. TaxJar is the
// production implementation; the interface keeps the provider swappable (a
// downloaded rate table later) behind the same cache seam. An implementation may
// leave RateResult.FetchedAt zero; the RateService stamps it.
type RateProvider interface {
	Rate(ctx context.Context, zip string) (RateResult, error)
}

// RateCache is a durable, read-through cache of rates keyed by normalized ZIP.
// found is false on a cache miss (a normal condition, not an error); err is set
// only for a real cache failure, which the caller treats as a miss and proceeds.
// The zip passed to GetRate/SetRate MUST be pre-normalized (see NormalizeZip);
// RateService is the single funnel that guarantees this.
type RateCache interface {
	GetRate(ctx context.Context, zip string) (result RateResult, found bool, err error)
	SetRate(ctx context.Context, zip string, result RateResult) error
}

// DefaultProviderTimeout bounds a single rate-provider call so a stalled lookup
// degrades to a flagged estimate quickly at quote time, regardless of the
// provider's own transport timeout.
const DefaultProviderTimeout = 3 * time.Second

// RateService resolves a ZIP to a combined rate: normalize, read cache, call the
// provider only on a miss, write the result back, and never hard-block. Any
// failure (invalid ZIP, provider error) returns a flagged estimate instead of an
// error so a tax lookup can never block a quote.
type RateService struct {
	provider        RateProvider
	cache           RateCache
	providerTimeout time.Duration
	now             func() time.Time
}

// NewRateService builds a RateService from a provider and cache.
func NewRateService(provider RateProvider, cache RateCache) *RateService {
	return &RateService{
		provider:        provider,
		cache:           cache,
		providerTimeout: DefaultProviderTimeout,
		now:             time.Now,
	}
}

// LookupRate returns the combined rate for a ZIP. It always returns a usable
// RateResult and never an error: a flagged estimate (Estimated=true, Warning set)
// is returned on invalid input or provider failure so the quote is never blocked.
func (s *RateService) LookupRate(ctx context.Context, rawZip string) RateResult {
	zip, ok := NormalizeZip(rawZip)
	if !ok {
		return s.flagged("", fmt.Sprintf("invalid ZIP %q; rate not estimated", rawZip))
	}
	if s.provider == nil {
		return s.flagged(zip, "rate estimate unavailable")
	}

	// Read-through: a cache hit short-circuits the provider. A cache error is not
	// fatal; treat it as a miss and fall through to the provider.
	if s.cache != nil {
		if cached, found, err := s.cache.GetRate(ctx, zip); err == nil && found {
			cached.Cached = true
			return cached
		}
	}

	// Bound the provider call so a stall degrades to a flagged estimate quickly.
	callCtx, cancel := context.WithTimeout(ctx, s.providerTimeout)
	defer cancel()
	result, err := s.provider.Rate(callCtx, zip)
	if err != nil {
		// The detailed provider error is intentionally not surfaced to clients
		// (it can echo an upstream response body); the warning stays generic.
		return s.flagged(zip, "rate estimate unavailable")
	}
	result.Zip = zip
	result.Cached = false
	result.Estimated = false
	if result.FetchedAt.IsZero() {
		result.FetchedAt = s.now()
	}

	// Write-on-miss is best effort: a cache write failure must never break a quote.
	if s.cache != nil {
		_ = s.cache.SetRate(ctx, zip, result)
	}
	return result
}

// flagged builds a non-blocking fallback result.
func (s *RateService) flagged(zip, warning string) RateResult {
	return RateResult{
		Zip:       zip,
		Estimated: true,
		Warning:   warning,
		FetchedAt: s.now(),
	}
}
