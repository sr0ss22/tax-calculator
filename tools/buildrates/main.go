// Command buildrates consolidates Avalara's free "TAXRATES_ZIP5" monthly export
// into a single CSV the estimator embeds (estimate/ratedata/us-zip-rates.csv).
//
// Avalara ships one CSV per state inside TAXRATES_ZIP5.zip with columns:
//
//	State, ZipCode, TaxRegionName, EstimatedCombinedRate, StateRate,
//	EstimatedCountyRate, EstimatedCityRate, EstimatedSpecialRate, RiskLevel
//
// This tool keeps only ZipCode, EstimatedCombinedRate, and a TaxRegionName
// (with state) so the embedded file stays small. Run it again whenever you
// re-download the monthly bundle.
//
// Usage:
//
//	go run ./tools/buildrates -zip ~/Downloads/TAXRATES_ZIP5.zip
//	go run ./tools/buildrates -src ~/Downloads/avalara-csvs       # a folder of CSVs
//	go run ./tools/buildrates -zip ... -out estimate/ratedata/us-zip-rates.csv
package main

import (
	"archive/zip"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type rate struct {
	combined string // normalized fraction string, e.g. "0.0600"
	region   string // "DORCHESTER, MD"
}

func main() {
	zipPath := flag.String("zip", "", "path to Avalara TAXRATES_ZIP5.zip")
	srcDir := flag.String("src", "", "folder of Avalara per-state CSVs (alternative to -zip)")
	out := flag.String("out", "estimate/ratedata/us-zip-rates.csv", "output consolidated CSV path")
	flag.Parse()

	if *zipPath == "" && *srcDir == "" {
		fmt.Fprintln(os.Stderr, "error: pass -zip <TAXRATES_ZIP5.zip> or -src <folder>")
		os.Exit(2)
	}

	rates := map[string]rate{}
	var files, rows int
	var err error
	if *zipPath != "" {
		files, rows, err = ingestZip(*zipPath, rates)
	} else {
		files, rows, err = ingestDir(*srcDir, rates)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if err := writeOut(*out, rates); err != nil {
		fmt.Fprintln(os.Stderr, "error writing output:", err)
		os.Exit(1)
	}
	fmt.Printf("processed %d CSV file(s), %d row(s) -> %d unique ZIPs\nwrote %s\n",
		files, rows, len(rates), *out)
}

func ingestZip(path string, rates map[string]rate) (files, rows int, err error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, 0, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return files, rows, err
		}
		n := parse(rc, rates)
		rc.Close()
		files++
		rows += n
	}
	return files, rows, nil
}

func ingestDir(dir string, rates map[string]rate) (files, rows int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".csv") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return files, rows, err
		}
		n := parse(f, rates)
		f.Close()
		files++
		rows += n
	}
	return files, rows, nil
}

// parse reads one Avalara CSV, matching columns by name, and adds rows to rates.
// First occurrence of a ZIP wins. Returns the number of data rows read.
func parse(r io.Reader, rates map[string]rate) int {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return 0
	}
	col := func(names ...string) int {
		for i, h := range header {
			hn := norm(h)
			for _, n := range names {
				if hn == n {
					return i
				}
			}
		}
		return -1
	}
	si := col("state")
	zi := col("zipcode", "zip", "postalcode")
	ci := col("estimatedcombinedrate", "combinedrate", "taxrate")
	ni := col("taxregionname", "region")
	if zi < 0 || ci < 0 {
		return 0
	}
	var rows int
	for {
		rec, err := cr.Read()
		if err != nil {
			break
		}
		rows++
		if zi >= len(rec) || ci >= len(rec) {
			continue
		}
		zip5, ok := normZip(rec[zi])
		if !ok {
			continue
		}
		if _, exists := rates[zip5]; exists {
			continue
		}
		frac, ok := parseRate(rec[ci])
		if !ok {
			continue
		}
		region := ""
		if ni >= 0 && ni < len(rec) {
			region = strings.TrimSpace(rec[ni])
		}
		if si >= 0 && si < len(rec) {
			if st := strings.TrimSpace(rec[si]); st != "" {
				if region != "" {
					region += ", "
				}
				region += st
			}
		}
		rates[zip5] = rate{combined: strconv.FormatFloat(frac, 'f', -1, 64), region: region}
	}
	return rows
}

func writeOut(path string, rates map[string]rate) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"ZipCode", "EstimatedCombinedRate", "TaxRegionName"}); err != nil {
		return err
	}
	zips := make([]string, 0, len(rates))
	for z := range rates {
		zips = append(zips, z)
	}
	sort.Strings(zips)
	for _, z := range zips {
		if err := w.Write([]string{z, rates[z].combined, rates[z].region}); err != nil {
			return err
		}
	}
	return w.Error()
}

func norm(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normZip(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) > 5 && s[5] == '-' {
		s = s[:5]
	}
	if len(s) != 5 {
		return "", false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return s, true
}

func parseRate(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	if v > 1 {
		v /= 100
	}
	return v, true
}
