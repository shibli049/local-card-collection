package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUnitParsePosition(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want []int
	}{
		{"name-with-hyphens-12-3-2.PNG", []int{12, 3, 2}},
		{"photo-001-02-03.jpg", []int{1, 2, 3}},
		{"plain.jpg", nil}, {"photo-1-2.jpg", nil}, {"photo-1-2-3-extra.png", nil},
		{"photo-a-2-3.png", nil}, {"photo-999999999999999999999999-2-3.png", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r, c := parsePosition(tc.name)
			if tc.want == nil {
				if p != nil || r != nil || c != nil {
					t.Fatalf("unexpected position %v %v %v", p, r, c)
				}
				return
			}
			if p == nil || r == nil || c == nil || !reflect.DeepEqual([]int{*p, *r, *c}, tc.want) {
				t.Fatalf("wanted %v, got %v %v %v", tc.want, p, r, c)
			}
		})
	}
}
func TestUnitPortableAlbumNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"Stamp album", true}, {"Box 04", true}, {"Ångström", true}, {"nested/album", true}, {".", true},
		{"", false}, {"..", false}, {"../escape", false}, {"/absolute", false}, {`C:\photos`, false},
		{"nested//album", false}, {"trailing.", false}, {" space", false}, {"CON", false}, {"nul.png", false},
		{"COM1", false}, {"LPT9.jpg", false}, {"bad?name", false}, {"bad\x00name", false},
		{".soft-inventory-import-test", false}, {strings.Repeat("a", 121), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validAlbumPath(tc.name); got != tc.valid {
				t.Fatalf("validAlbumPath(%q)=%v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}
func TestUnitImportedFilenamesAndPositions(t *testing.T) {
	t.Parallel()
	paged := Album{Mode: "paged", Pages: 3, Rows: 2, Columns: 3}
	box := Album{Mode: "box"}
	for _, tc := range []struct {
		original string
		index    int
		album    Album
		want     string
	}{
		{"photo.PNG", 0, paged, "photo-1-1-1.png"},
		{"photo.PNG", 2, paged, "photo-1-1-3.png"},
		{"photo.PNG", 3, paged, "photo-1-2-1.png"},
		{"photo.PNG", 6, paged, "photo-2-1-1.png"},
		{"photo-9-9-9.PNG", 7, paged, "photo-2-1-2.png"},
		{"photo-9-9-9.PNG", 0, box, "photo-9-9-9.png"},
		{`C:\source\photo.PNG`, 0, box, "photo.png"},
		{"../photo.PNG", 0, box, "photo.png"},
		{"a:b?.JPG", 0, box, "a_b_.jpg"},
		{"CON.jpg", 0, box, "image-CON.jpg"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := importedName(tc.original, tc.index, tc.album); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
func TestUnitImportOptions(t *testing.T) {
	t.Parallel()
	base := ImportOptions{Name: "Album", Mode: "paged", Pages: 2, Rows: 3, Columns: 4}
	for _, tc := range []struct {
		name   string
		change func(*ImportOptions)
		valid  bool
	}{
		{"default start", func(o *ImportOptions) {}, true},
		{"last position", func(o *ImportOptions) { o.StartPage = 2; o.StartRow = 3; o.StartColumn = 4 }, true},
		{"no pages", func(o *ImportOptions) { o.Pages = 0 }, false},
		{"too many pages", func(o *ImportOptions) { o.Pages = 1001 }, false},
		{"too many rows", func(o *ImportOptions) { o.Rows = 101 }, false},
		{"negative columns", func(o *ImportOptions) { o.Columns = -1 }, false},
		{"start beyond page", func(o *ImportOptions) { o.StartPage = 3 }, false},
		{"start beyond row", func(o *ImportOptions) { o.StartRow = 4 }, false},
		{"start beyond column", func(o *ImportOptions) { o.StartColumn = 5 }, false},
		{"negative start", func(o *ImportOptions) { o.StartRow = -1 }, false},
		{"unknown mode", func(o *ImportOptions) { o.Mode = "other" }, false},
		{"box ignores dimensions", func(o *ImportOptions) { o.Mode = "box"; o.Rows = -1; o.Pages = 9999 }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := base
			tc.change(&options)
			err := options.validate()
			if (err == nil) != tc.valid {
				t.Fatalf("validate returned %v", err)
			}
			if err == nil && options.Mode == "paged" && (options.StartPage < 1 || options.StartRow < 1 || options.StartColumn < 1) {
				t.Fatal("missing default start")
			}
			if err == nil && options.Mode == "box" && (options.Pages != 0 || options.Rows != 0 || options.Columns != 0) {
				t.Fatal("box retained dimensions")
			}
		})
	}
}
func TestUnitCombinedFiltersAndOrdering(t *testing.T) {
	number := func(n int) *int { return &n }
	c := &Catalog{data: CatalogData{Images: map[string]Image{
		"Album/z-2-1-1.png":  {Path: "Album/z-2-1-1.png", Folder: "Album", Filename: "z-2-1-1.png", Page: number(2), Row: number(1), Col: number(1), Description: "Blue stamp", Present: true},
		"Album/a-10-1-1.png": {Path: "Album/a-10-1-1.png", Folder: "Album", Filename: "a-10-1-1.png", Page: number(10), Row: number(1), Col: number(1), Description: "Red stamp", Present: true},
		"Box/plain.png":      {Path: "Box/plain.png", Folder: "Box", Filename: "plain.png", Description: "Blue envelope", Present: true},
		"Box/missing.png":    {Path: "Box/missing.png", Folder: "Box", Filename: "missing.png", Description: "Blue stamp", Present: false},
	}}}
	for _, tc := range []struct {
		name    string
		filters Filters
		want    []string
	}{
		{"numeric position order", Filters{Folder: "Album"}, []string{"z-2-1-1.png", "a-10-1-1.png"}},
		{"filename order", Filters{Folder: "Album", Sort: "name"}, []string{"a-10-1-1.png", "z-2-1-1.png"}},
		{"combined filters", Filters{Query: "BLUE stamp", Folder: "Album", Sheet: "2"}, []string{"z-2-1-1.png"}},
		{"unpositioned", Filters{Sheet: "none"}, []string{"plain.png"}},
		{"all terms required", Filters{Query: "blue red"}, []string{}},
		{"page mismatch", Filters{Folder: "Box", Sheet: "2"}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := c.list(tc.filters)
			got := []string{}
			for _, item := range result.Items {
				got = append(got, item.Filename)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if result.Count != 3 {
				t.Fatal("missing images included in count")
			}
		})
	}
}
func TestUnitDescriptionLimits(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "photo.png")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	note := strings.Repeat("å", 20000)
	if err := c.describe("photo.png", note); err != nil {
		t.Fatalf("character limit counted bytes: %v", err)
	}
	for _, tc := range []struct{ path, note string }{{"photo.png", note + "å"}, {"absent.png", "note"}} {
		if err := c.describe(tc.path, tc.note); err == nil {
			t.Fatal("invalid edit accepted")
		}
	}
	if c.data.Images["photo.png"].Description != note {
		t.Fatal("invalid edit changed saved note")
	}
}
func TestUnitCatalogFormatAndLegacyDetection(t *testing.T) {
	for _, raw := range []string{`{"version":2,"images":{}}`, `{"version":1,"images":null}`} {
		t.Run(raw, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "catalog.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := openCatalog(directory); err == nil {
				t.Fatal("unsupported catalog accepted")
			}
			saved, err := os.ReadFile(path)
			if err != nil || string(saved) != raw {
				t.Fatal("unsupported catalog changed")
			}
		})
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "catalog.sqlite3"), []byte("legacy database"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openCatalog(directory); err == nil || !strings.Contains(err.Error(), "older catalog format") {
		t.Fatalf("missing legacy catalog warning: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "catalog.json")); !os.IsNotExist(err) {
		t.Fatal("legacy catalog silently replaced")
	}
}
