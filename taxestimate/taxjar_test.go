package taxestimate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTaxJarProvider_Rate_Success(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		// TaxJar returns rate fields as JSON strings, not numbers.
		_, _ = w.Write([]byte(`{"rate":{"combined_rate":"0.0825","state_rate":"0.0625","county_rate":"0.005","city_rate":"0.01","combined_district_rate":"0","city":"HOUSTON","county":"HARRIS"}}`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("test-token", srv.URL+"/v2/rates/", srv.Client())
	got, err := p.Rate(context.Background(), "77002")
	if err != nil {
		t.Fatalf("Rate() error = %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token")
	}
	if !strings.HasSuffix(gotPath, "/v2/rates/77002") {
		t.Errorf("request path = %q, want it to end with /v2/rates/77002", gotPath)
	}
	if got.CombinedRate != 0.0825 {
		t.Errorf("CombinedRate = %v, want 0.0825", got.CombinedRate)
	}
	if got.City != "HOUSTON" || got.County != "HARRIS" {
		t.Errorf("city/county = %q/%q, want HOUSTON/HARRIS", got.City, got.County)
	}
	// state 6.250% + county 0.500% + city 1.000% present; special (0) omitted.
	wantBreakdown := "Breakdown: state 6.250% + county 0.500% + city 1.000%"
	if got.Jurisdictions != wantBreakdown {
		t.Errorf("Jurisdictions = %q, want %q", got.Jurisdictions, wantBreakdown)
	}
}

func TestTaxJarProvider_Rate_NoTax_BreakdownNA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A legitimate 0% rate (e.g. Oregon): combined_rate present as "0".
		_, _ = w.Write([]byte(`{"rate":{"combined_rate":"0","state_rate":"0","county_rate":"0","city_rate":"0","combined_district_rate":"0","city":"PORTLAND","county":"MULTNOMAH"}}`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("tok", srv.URL+"/", srv.Client())
	got, err := p.Rate(context.Background(), "97201")
	if err != nil {
		t.Fatalf("Rate() error = %v", err)
	}
	if got.CombinedRate != 0 {
		t.Errorf("CombinedRate = %v, want 0", got.CombinedRate)
	}
	if got.Jurisdictions != "Breakdown: n/a" {
		t.Errorf("Jurisdictions = %q, want %q", got.Jurisdictions, "Breakdown: n/a")
	}
}

// TestTaxJarProvider_Rate_NumericFieldsTolerated proves the parser also accepts
// number-encoded rate fields, so the provider is robust to either wire shape.
func TestTaxJarProvider_Rate_NumericFieldsTolerated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rate":{"combined_rate":0.0825,"state_rate":0.0625,"city_rate":0.02,"city":"X","county":"Y"}}`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("tok", srv.URL+"/", srv.Client())
	got, err := p.Rate(context.Background(), "77002")
	if err != nil {
		t.Fatalf("Rate() error = %v", err)
	}
	if got.CombinedRate != 0.0825 {
		t.Errorf("CombinedRate = %v, want 0.0825", got.CombinedRate)
	}
	if got.Jurisdictions != "Breakdown: state 6.250% + city 2.000%" {
		t.Errorf("Jurisdictions = %q", got.Jurisdictions)
	}
}

// TestTaxJarProvider_Rate_MissingCombinedRate ensures an absent combined_rate is
// treated as unknown (an error -> flagged estimate), not a silent 0%.
func TestTaxJarProvider_Rate_MissingCombinedRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rate":{"state_rate":"0.0625","city":"X","county":"Y"}}`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("tok", srv.URL+"/", srv.Client())
	_, err := p.Rate(context.Background(), "77002")
	if err == nil {
		t.Fatalf("Rate() error = nil, want an error for a missing combined_rate")
	}
	if !strings.Contains(err.Error(), "combined_rate") {
		t.Errorf("error = %q, want it to mention combined_rate", err.Error())
	}
}

// TestTaxJarProvider_Rate_UnparseableRate treats a non-numeric rate string as a
// decode error (-> flagged estimate) rather than a silent zero.
func TestTaxJarProvider_Rate_UnparseableRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rate":{"combined_rate":"abc","city":"X","county":"Y"}}`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("tok", srv.URL+"/", srv.Client())
	_, err := p.Rate(context.Background(), "77002")
	if err == nil {
		t.Fatalf("Rate() error = nil, want a decode error for a non-numeric rate")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, want it to mention decode", err.Error())
	}
}

// TestTaxJarProvider_Rate_NullComponent treats a null component rate as zero
// (excluded from the breakdown) while a valid combined_rate is still returned.
func TestTaxJarProvider_Rate_NullComponent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rate":{"combined_rate":"0.0625","state_rate":"0.0625","county_rate":null,"city":"X","county":"Y"}}`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("tok", srv.URL+"/", srv.Client())
	got, err := p.Rate(context.Background(), "77002")
	if err != nil {
		t.Fatalf("Rate() error = %v", err)
	}
	if got.CombinedRate != 0.0625 {
		t.Errorf("CombinedRate = %v, want 0.0625", got.CombinedRate)
	}
	if got.Jurisdictions != "Breakdown: state 6.250%" {
		t.Errorf("Jurisdictions = %q, want only the state component", got.Jurisdictions)
	}
}

// TestTaxJarProvider_Rate_InvalidZip rejects un-normalized input before building a URL.
func TestTaxJarProvider_Rate_InvalidZip(t *testing.T) {
	p := NewTaxJarProvider("tok", "https://example.invalid/", nil)
	_, err := p.Rate(context.Background(), "../etc/passwd")
	if err == nil {
		t.Fatalf("Rate() error = nil, want an error for an invalid ZIP")
	}
	if !strings.Contains(err.Error(), "invalid ZIP") {
		t.Errorf("error = %q, want it to mention an invalid ZIP", err.Error())
	}
}

func TestTaxJarProvider_Rate_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized","detail":"bad token"}`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("tok", srv.URL+"/", srv.Client())
	_, err := p.Rate(context.Background(), "77002")
	if err == nil {
		t.Fatalf("Rate() error = nil, want an error on HTTP 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want it to mention HTTP 401", err.Error())
	}
}

func TestTaxJarProvider_Rate_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	p := NewTaxJarProvider("tok", srv.URL+"/", srv.Client())
	_, err := p.Rate(context.Background(), "77002")
	if err == nil {
		t.Fatalf("Rate() error = nil, want a decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, want it to mention decode", err.Error())
	}
}

func TestTaxJarProvider_Rate_NoToken(t *testing.T) {
	p := NewTaxJarProvider("", "", nil)
	_, err := p.Rate(context.Background(), "77002")
	if err == nil {
		t.Fatalf("Rate() error = nil, want an error when no token is configured")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error = %q, want it to mention the missing token", err.Error())
	}
}

func TestNewTaxJarProvider_Defaults(t *testing.T) {
	p := NewTaxJarProvider("tok", "", nil)
	if p.baseURL != DefaultTaxJarBaseURL {
		t.Errorf("baseURL = %q, want default %q", p.baseURL, DefaultTaxJarBaseURL)
	}
	if p.httpClient == nil {
		t.Errorf("httpClient is nil, want a default client")
	}
}
