# Quote Tax Estimate (standalone)

A self-contained sales-tax estimate calculator for window-covering quotes, deployable to Vercel. It runs the same tax engine as the Brite RDIS-135 work (US taxability matrix with the THD blended rate, static Canada province rates with the BC and MB exceptions, optional TaxJar US rate lookup, and the measure/installation fee handling) with no Brite service dependencies.

This is an estimate. SAP remains the system of record for actual tax.

## Layout

- `taxestimate/` - the pure tax engine (embedded `data/tax_data.json`).
- `estimate/` - orchestration: turns an entered quote into an estimate (no protos).
- `api/estimate.go` - the Vercel serverless function (POST `/api/estimate`).
- `public/index.html` - the calculator page (served at `/`).

## Deploy to Vercel

1. Import this repo in Vercel (zero-config: it detects the Go function in `api/` and serves `public/`).
2. Optional: set `TAXJAR_API_TOKEN` in Vercel project env to enable live US rate lookup by ZIP. Without it, US quotes use the manual rate override and Canada works fully offline.
3. Deploy. The page is at `/`, the API at `/api/estimate`.

Or from the CLI: `vercel` (preview) / `vercel --prod` (production).

## Local development

```
vercel dev      # runs the function + static page locally at http://localhost:3000
```

## API

`POST /api/estimate`

```json
{
  "channel": "THD",
  "state": "TX",
  "zip": "78664",
  "rateOverride": 0.0825,
  "measureFee": 120,
  "installFee": 800,
  "lines": [
    {"name": "Custom Blinds", "category": "blinds", "amount": 1500},
    {"name": "Custom Shutters", "category": "shutters", "amount": 2000}
  ]
}
```

`state` accepts a US state code or name, or a Canadian province name or code (e.g. `Ontario`, `BC`) to use the static Canada chart. `category` is `blinds`, `shutters`, or `draperies`. `channel` is `THD` or `partners`.

## Tests

```
go test ./...
```
