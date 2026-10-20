package main

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

// Quoting alone does not stop spreadsheet apps from interpreting a cell as a
// formula. Prefix dangerous text with an apostrophe, including indented formulas.
func csvText(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if (len(value) > 0 && strings.ContainsRune("\t\r\n", rune(value[0]))) || (len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + value
	}
	return value
}
func csvPosition(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}
func (c *Catalog) exportCSV(w http.ResponseWriter, r *http.Request) {
	c.exportSelectionCSV(w, r, r.URL.Query()["album"])
}
func (c *Catalog) exportSelectionCSV(w http.ResponseWriter, r *http.Request, albums []string) {
	listing := c.listImages(Filters{}, false)
	available := map[string]bool{}
	for _, album := range listing.Folders {
		available[album.Path] = true
	}
	selected := map[string]bool{}
	for _, album := range albums {
		if !available[album] {
			writeJSON(w, 400, map[string]string{"error": "An album no longer exists. Reload the library and select albums again."})
			return
		}
		selected[album] = true
	}
	if len(selected) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Select at least one album to export."})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="CardAtlas-export.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	// UTF-8 BOM lets Excel recognize non-ASCII descriptions and album names.
	if _, err := w.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		return
	}
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	if err := writer.Write([]string{"Name", "Filename", "Album", "Page", "Row", "Column", "Price", "Description", "Tags", "Relative path"}); err != nil {
		return
	}
	for _, item := range listing.Items {
		if !selected[item.Folder] {
			continue
		}
		record := []string{csvText(item.Name), csvText(item.Filename), csvText(item.Folder), csvPosition(item.Page), csvPosition(item.Row), csvPosition(item.Col), csvText(item.Price), csvText(item.Description), csvText(strings.Join(item.Tags, "; ")), csvText(item.Path)}
		if err := writer.Write(record); err != nil {
			return
		}
	}
	writer.Flush()
}
