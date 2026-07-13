# Avalara ZIP rate table

`us-zip-rates.csv` is the offline US rate source: a consolidated `ZipCode,
EstimatedCombinedRate, TaxRegionName` table embedded into the binary at build
time and used ahead of the per-state average (TaxJar, when a token is set, still
takes precedence). Any `*.csv` in this folder is loaded, so the single
consolidated file is all that's needed.

## Refreshing it monthly

Avalara publishes the free **TAXRATES_ZIP5** bundle (one CSV per state, ~monthly)
at https://www.avalara.com/taxrates/en/download-tax-tables.html — submit the
short form, download `TAXRATES_ZIP5.zip`, then regenerate:

```sh
go run ./tools/buildrates -zip ~/Downloads/TAXRATES_ZIP5.zip
# or from an unzipped folder of CSVs:
go run ./tools/buildrates -src ~/Downloads/avalara-csvs
```

That rewrites `us-zip-rates.csv` (one file in the diff). Commit and deploy.

## AZ city rate table (reference / validation)

`az-city-rates.csv` (`city, rate, jurisdiction`) is HD's authoritative **AZ Sales
tax by City** chart. It is a **reference and validation** dataset, not part of the
ZIP rate path: it has no `ZipCode` column, so the Avalara loader skips it, and the
calculator keeps resolving Arizona rates by ZIP (TaxJar / the ZIP table). A city
can appear more than once (different jurisdictions and rates). It is loaded by
`estimate/az_rates.go` and exists to cross-check the ZIP path and serve as an
offline AZ reference.

## Notes

- The importer matches columns by name, so Avalara's standard export
  (`State, ZipCode, TaxRegionName, EstimatedCombinedRate, ...`) loads as-is.
- ZIP rates are ZIP-centroid estimates, not rooftop-exact. The rate-override
  field in the UI remains the way to key an exact local rate.
- No CSV here is fine — the app falls back to the per-state average automatically.
- This tool is **estimate-only**; SAP remains the system of record for actual tax.
