# Avalara ZIP rate tables

Drop Avalara's free monthly **sales tax rates by ZIP code** CSV files here. Any
`*.csv` in this folder is embedded into the binary at build time and used as the
offline US rate source (ahead of the per-state average fallback; TaxJar, when a
token is set, still takes precedence).

## How to get the files

1. Go to https://www.avalara.com/taxrates/en/download-tax-tables.html
2. Select the state(s) you need and submit the short form. Avalara emails a CSV
   per state (and refreshes monthly — re-download about once a month).
3. Save each CSV into this directory (any filename ending in `.csv`, e.g.
   `MD.csv`, `TX.csv`). Commit them.

## Expected columns

The loader matches columns by name (case/spacing-insensitive), so the standard
export works as-is:

```
State, ZipCode, TaxRegionName, EstimatedCombinedRate, StateRate,
EstimatedCountyRate, EstimatedCityRate, EstimatedSpecialRate, RiskLevel
```

Only `ZipCode` and `EstimatedCombinedRate` are required; `TaxRegionName` is used
for display. Rates may be fractions (`0.06`) or percents (`6` / `6%`).

## Notes

- ZIP rates are ZIP-centroid estimates, not rooftop-exact. The rate-override
  field in the UI remains the way to key an exact local rate.
- No CSV here is fine — the app falls back to the per-state average automatically.
- This tool is **estimate-only**; SAP remains the system of record for actual tax.
