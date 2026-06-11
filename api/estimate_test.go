package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_TexasOverride(t *testing.T) {
	body := `{"channel":"THD","state":"TX","rateOverride":0.0825,"lines":[{"name":"Blinds","category":"blinds","amount":1500},{"name":"Shutters","category":"shutters","amount":2000}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/estimate", strings.NewReader(body))
	rec := httptest.NewRecorder()

	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	// 1500 blinds taxable at 8.25% = 123.75; shutters exempt in TX.
	if !strings.Contains(out, `"totalTax":123.75`) {
		t.Errorf("response missing expected totalTax 123.75: %s", out)
	}
	if !strings.Contains(out, `"blended":true`) {
		t.Errorf("response should flag the THD blended case: %s", out)
	}
}

func TestHandler_CanadaOntario(t *testing.T) {
	body := `{"state":"Ontario","lines":[{"name":"Blinds","category":"blinds","amount":1000},{"name":"Drapes","category":"draperies","amount":1000}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/estimate", strings.NewReader(body))
	rec := httptest.NewRecorder()

	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"totalTax":260`) {
		t.Errorf("Ontario 13%% of 2000 should be 260: %s", rec.Body.String())
	}
}

func TestHandler_Rejects(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{"GET not allowed", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"bad JSON", http.MethodPost, "{not json", http.StatusBadRequest},
		{"no state", http.MethodPost, `{"lines":[{"name":"B","category":"blinds","amount":1}]}`, http.StatusBadRequest},
		{"unknown category", http.MethodPost, `{"state":"TX","lines":[{"name":"B","category":"rugs","amount":1}]}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/estimate", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			Handler(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
