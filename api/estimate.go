// Package handler is the Vercel serverless function for the tax estimate API.
// POST /api/estimate with an estimate.Request JSON body returns an estimate.Result.
package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/sr0ss22/tax-calculator/estimate"
)

var (
	est     *estimate.Estimator
	initErr error
	once    sync.Once
)

// estimator builds (once per cold start) the estimator from the embedded data.
// TAXJAR_API_TOKEN is optional; without it, US quotes need a manual rate override
// and Canada works fully offline.
func estimator() (*estimate.Estimator, error) {
	once.Do(func() { est, initErr = estimate.New(os.Getenv("TAXJAR_API_TOKEN")) })
	return est, initErr
}

// Handler is the Vercel entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "use POST with a JSON quote body")
		return
	}

	e, err := estimator()
	if err != nil {
		log.Printf("taxcalc: estimator init failed: %v", err)
		writeErr(w, http.StatusInternalServerError, "tax engine unavailable")
		return
	}

	var req estimate.Request
	if derr := json.NewDecoder(r.Body).Decode(&req); derr != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+derr.Error())
		return
	}

	result, eerr := e.Estimate(context.Background(), req)
	if eerr != nil {
		writeErr(w, http.StatusBadRequest, eerr.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
