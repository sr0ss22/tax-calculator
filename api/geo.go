// Package handler also serves GET /api/geo, which reports the caller's location
// from Vercel's edge geo headers (no third-party service, no API key). It returns
// the full region name the calculator expects so the UI can preselect the state.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/sr0ss22/tax-calculator/estimate"
)

// Geo is the Vercel entrypoint for /api/geo.
func Geo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	country := r.Header.Get("x-vercel-ip-country")
	regionCode := r.Header.Get("x-vercel-ip-country-region")
	city := r.Header.Get("x-vercel-ip-city")

	_ = json.NewEncoder(w).Encode(map[string]string{
		"country":    country,
		"regionCode": regionCode,
		"region":     estimate.ResolveRegion(country, regionCode),
		"city":       city,
	})
}
