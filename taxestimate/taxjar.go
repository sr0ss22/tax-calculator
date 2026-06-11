package taxestimate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultTaxJarBaseURL is the TaxJar rate endpoint base. The ZIP is appended.
const DefaultTaxJarBaseURL = "https://api.taxjar.com/v2/rates/"

// maxTaxJarResponseBytes caps how much of the response we read. A rate response
// is a few hundred bytes; the cap bounds memory if the endpoint misbehaves.
const maxTaxJarResponseBytes = 1 << 20 // 1 MiB

// flexFloat parses a JSON value that may be either a number or a numeric string.
// The TaxJar rate API returns rate fields as strings (for example "0.0825"); the
// prototype handled this with parseFloat. An empty or null value parses to zero.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("invalid numeric value %q: %w", s, err)
	}
	*f = flexFloat(v)
	return nil
}

// taxjarRateBody is the shape of the TaxJar /v2/rates/{zip} response we consume.
// combined_rate is a pointer so an absent or null field (rate unknown) is
// distinguishable from a legitimate 0% rate (for example Oregon).
type taxjarRateBody struct {
	Rate struct {
		CombinedRate         *flexFloat `json:"combined_rate"`
		StateRate            flexFloat  `json:"state_rate"`
		CountyRate           flexFloat  `json:"county_rate"`
		CityRate             flexFloat  `json:"city_rate"`
		CombinedDistrictRate flexFloat  `json:"combined_district_rate"`
		City                 string     `json:"city"`
		County               string     `json:"county"`
	} `json:"rate"`
}

// TaxJarProvider is a RateProvider backed by the TaxJar rate API. It is a port of
// the prototype taxjarRate: one live call to the rate endpoint per ZIP, returning
// the combined rate and a jurisdiction breakdown. The token is held server-side
// and never exposed to clients.
type TaxJarProvider struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// NewTaxJarProvider builds a TaxJarProvider. An empty baseURL defaults to the
// TaxJar rate endpoint; a nil httpClient defaults to a client with a 10s timeout.
func NewTaxJarProvider(token, baseURL string, httpClient *http.Client) *TaxJarProvider {
	if baseURL == "" {
		baseURL = DefaultTaxJarBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &TaxJarProvider{token: token, baseURL: baseURL, httpClient: httpClient}
}

// Rate fetches the combined rate for a ZIP from TaxJar. The caller (RateService)
// is responsible for caching and for the flagged-estimate fallback on error.
// Rate defensively re-validates the ZIP so it can never build a request URL from
// un-normalized input even if called outside RateService.
func (p *TaxJarProvider) Rate(ctx context.Context, zip string) (RateResult, error) {
	if p.token == "" {
		return RateResult{}, fmt.Errorf("taxjar: no API token configured")
	}
	normalized, ok := NormalizeZip(zip)
	if !ok {
		return RateResult{}, fmt.Errorf("taxjar: invalid ZIP %q", zip)
	}

	endpoint := p.baseURL + url.PathEscape(normalized) + "?country=US"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return RateResult{}, fmt.Errorf("taxjar: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return RateResult{}, fmt.Errorf("taxjar: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTaxJarResponseBytes))
	if err != nil {
		return RateResult{}, fmt.Errorf("taxjar: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		// The snippet stays in this server-side error only; the caller surfaces a
		// generic, non-blocking warning to clients so an upstream body is not echoed.
		return RateResult{}, fmt.Errorf("taxjar: HTTP %d %s", resp.StatusCode, snippet)
	}

	var parsed taxjarRateBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return RateResult{}, fmt.Errorf("taxjar: decode response: %w", err)
	}
	if parsed.Rate.CombinedRate == nil {
		return RateResult{}, fmt.Errorf("taxjar: response missing combined_rate")
	}

	return RateResult{
		Zip:           normalized,
		CombinedRate:  float64(*parsed.Rate.CombinedRate),
		Jurisdictions: jurisdictionBreakdown(parsed),
		City:          parsed.Rate.City,
		County:        parsed.Rate.County,
	}, nil
}

// jurisdictionBreakdown renders a human-readable rate breakdown, mirroring the
// prototype: each component with a positive rate is shown as "name P%", joined by
// " + ". When no component is positive the breakdown is "n/a".
func jurisdictionBreakdown(b taxjarRateBody) string {
	parts := []struct {
		name string
		rate float64
	}{
		{"state", float64(b.Rate.StateRate)},
		{"county", float64(b.Rate.CountyRate)},
		{"city", float64(b.Rate.CityRate)},
		{"special", float64(b.Rate.CombinedDistrictRate)},
	}
	var rendered []string
	for _, p := range parts {
		if p.rate > 0 {
			rendered = append(rendered, fmt.Sprintf("%s %.3f%%", p.name, p.rate*100))
		}
	}
	if len(rendered) == 0 {
		return "Breakdown: n/a"
	}
	return "Breakdown: " + strings.Join(rendered, " + ")
}
