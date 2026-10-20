package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnitCSVFormulaProtection(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"=SUM(A1:A2)", "'=SUM(A1:A2)"}, {" +CMD()", "' +CMD()"}, {"-1+2", "'-1+2"}, {"@formula", "'@formula"}, {"\tdata", "'\tdata"},
		{"Description, with commas", "Description, with commas"}, {"Line one\nLine two", "Line one\nLine two"}, {"25.00 SEK", "25.00 SEK"}, {"Ångström", "Ångström"}, {"", ""},
	} {
		if got := csvText(tc.value); got != tc.want {
			t.Errorf("csvText(%q)=%q, want %q", tc.value, got, tc.want)
		}
	}
}
func TestIntegrationExportMultipleWholeAlbums(t *testing.T) {
	directory := t.TempDir()
	root := t.TempDir()
	c, err := openCatalog(directory)
	if err != nil {
		t.Fatal(err)
	}
	// More than one UI result page ensures CSV is not limited to 60 cards.
	for i := 1; i <= 65; i++ {
		writeImage(t, root, fmt.Sprintf("Album/card-%d-1-1.png", i))
	}
	writeImage(t, root, "Box/plain.png")
	writeImage(t, root, "Excluded/secret.png")
	if _, err = c.scan(root); err != nil {
		t.Fatal(err)
	}
	price := "25.00 SEK"
	note := "Ångström, a \"rare\" card\nSecond line"
	if err = c.editDetails("Album/card-1-1-1.png", note, &price); err != nil {
		t.Fatal(err)
	}
	writeImage(t, root, "Box/removed.png")
	if _, err = c.scan(""); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, "Box", "removed.png")); err != nil {
		t.Fatal(err)
	}
	if _, err = c.scan(""); err != nil {
		t.Fatal(err)
	}
	_, admin := startIntegrationServer(t, directory)
	// Export is an administrator action; visitors retain search-only access.
	admin.call(t, "GET", "/api/export?album=Album", nil, "", http.StatusUnauthorized)
	admin.login(t)
	raw, headers := admin.call(t, "GET", "/api/export?album=Album&album=Box&album=Album&q=nomatch&page=2", nil, "", 200)
	if !strings.HasPrefix(headers.Get("Content-Type"), "text/csv") || !strings.Contains(headers.Get("Content-Disposition"), "CardAtlas-export.csv") {
		t.Fatal("missing CSV download headers")
	}
	if !strings.HasPrefix(string(raw), "\ufeff") {
		t.Fatal("missing UTF-8 marker for Excel")
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 67 {
		t.Fatalf("got %d CSV records, want header + 66 cards", len(records))
	}
	if strings.Join(records[0], "|") != "Album|Filename|Page|Row|Column|Description|Price|Relative path" {
		t.Fatal("wrong CSV columns")
	}
	if records[1][1] != "card-1-1-1.png" || records[1][5] != note || records[1][6] != price {
		t.Fatalf("details did not round-trip CSV: %q", records[1])
	}
	if records[65][2] != "65" {
		t.Fatal("CSV order is not numeric by position")
	}
	last := records[66]
	if last[0] != "Box" || last[2] != "" || last[3] != "" || last[4] != "" || last[6] != "" {
		t.Fatal("unpositioned or unpriced fields should be blank")
	}
	for _, record := range records[1:] {
		if record[0] == "Excluded" || strings.Contains(record[7], "removed") {
			t.Fatal("export included an unselected album or removed image")
		}
	}
	admin.call(t, "GET", "/api/export", nil, "", 400)
	admin.call(t, "GET", "/api/export?album="+url.QueryEscape("Unknown album"), nil, "", 400)
}
