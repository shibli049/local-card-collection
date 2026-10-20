package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scaleCatalog(cards, albums int) *Catalog {
	c := &Catalog{data: CatalogData{Version: 1, Images: map[string]Image{}, Albums: map[string]Album{}}}
	for i := 0; i < cards; i++ {
		folder := fmt.Sprintf("Album %05d", i%albums)
		name := fmt.Sprintf("Card %06d", i)
		path := folder + "/" + name + ".png"
		c.data.Images[path] = Image{Path: path, Filename: name + ".png", Folder: folder, Name: name, Description: "A searchable blue card " + strings.Repeat("detail ", 20), Tags: []string{"Rare"}, Present: true}
	}
	return c
}
func TestUnitLargeCatalogBrowseIsBounded(t *testing.T) {
	c := scaleCatalog(50000, 2000)
	response := request(c, "GET", "/api/images?browse=albums&page=2", "", "")
	var listing Listing
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Mode != "albums" || listing.Count != 50000 || listing.AlbumCount != 2000 || len(listing.Items) != 0 || len(listing.Folders) != 50 || listing.PageCount != 40 {
		t.Fatalf("unbounded or incorrect album browse: %+v", listing)
	}
	if response.Body.Len() > 15000 || strings.Contains(response.Body.String(), "searchable blue card") {
		t.Fatal("album overview includes card metadata")
	}
	index := c.catalogIndex()
	result := c.list(Filters{Folder: "Album 00001"})
	if result.Total != 25 || len(result.Items) != 25 {
		t.Fatal("album selection returned the wrong cards")
	}
	if c.catalogIndex() != index {
		t.Fatal("unchanged queries rebuilt the catalog index")
	}
	result = c.list(Filters{Query: "blue Rare", Page: 2})
	if result.Total != 50000 || len(result.Items) != 60 {
		t.Fatal("search lost pagination or tag matching")
	}
	response = request(c, "GET", "/api/images?summary=0&folder=Album+01999", "", "")
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) > 41 {
		t.Fatal("image responses include the entire album directory")
	}
	found := false
	for _, album := range listing.Folders {
		if album.Path == "Album 01999" {
			found = true
		}
	}
	if !found {
		t.Fatal("selected album metadata was omitted")
	}
}
func TestUnitIndexInvalidatesAfterEditAndReload(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "one.png")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	old := c.catalogIndex()
	if err := c.describe("one.png", "new searchable note"); err != nil {
		t.Fatal(err)
	}
	if c.catalogIndex() == old || c.list(Filters{Query: "searchable"}).Total != 1 {
		t.Fatal("edit left a stale search index")
	}
	if old.position[0].image.Description != "" {
		t.Fatal("edit mutated an in-flight index snapshot")
	}
	writeImage(t, root, "two.png")
	if _, err := c.scan(""); err != nil {
		t.Fatal(err)
	}
	if c.listAlbums("", 1, false).Count != 2 {
		t.Fatal("reload left stale album counts")
	}
}
func TestIntegrationThumbnailCacheAndScanExclusion(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "large.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err = jpeg.Encode(file, image.NewRGBA(image.Rect(0, 0, 1600, 800)), nil); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err = c.scan(root); err != nil {
		t.Fatal(err)
	}
	response := request(c, "GET", "/thumbnail?path=large.jpg", "", "")
	if response.Code != 200 || response.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumbnail failed: %d %s", response.Code, response.Body.String())
	}
	config, err := jpeg.DecodeConfig(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 320 || config.Height != 160 {
		t.Fatalf("thumbnail did not resize original: %+v", config)
	}
	cache := filepath.Join(root, ".card-atlas-cache", "thumbnails")
	files, err := os.ReadDir(cache)
	if err != nil || len(files) != 1 {
		t.Fatalf("thumbnail not cached: %v", err)
	}
	conditional := httptest.NewRequest("GET", "http://127.0.0.1:8765/thumbnail?path=large.jpg", nil)
	conditional.Header.Set("If-None-Match", response.Header().Get("ETag"))
	record := httptest.NewRecorder()
	c.handler().ServeHTTP(record, conditional)
	if record.Code != 304 {
		t.Fatal("thumbnail conditional cache request failed")
	}
	if count, err := c.scan(""); err != nil || count != 1 {
		t.Fatalf("rescan imported thumbnail cache: %d %v", count, err)
	}
	for _, path := range []string{"../private.jpg", "missing.png"} {
		if response := request(c, "GET", "/thumbnail?path="+path, "", ""); response.Code != 404 {
			t.Fatal("thumbnail path boundary failed")
		}
	}
}
func BenchmarkLargeCatalogAlbumBrowse(b *testing.B) {
	c := scaleCatalog(50000, 2000)
	c.catalogIndex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.listAlbums("", 1, false)
	}
}
func BenchmarkLargeCatalogFolderCards(b *testing.B) {
	c := scaleCatalog(50000, 2000)
	c.catalogIndex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.list(Filters{Folder: "Album 00001"})
	}
}
func BenchmarkLargeCatalogSearch(b *testing.B) {
	c := scaleCatalog(50000, 2000)
	c.catalogIndex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.list(Filters{Query: "blue Rare", Page: 2})
	}
}

func TestUnitThumbnailCachePruningPreservesOtherFiles(t *testing.T) {
	directory := t.TempDir()
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("%d.jpg", i)), []byte(strings.Repeat("x", 100)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "keep.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	pruneThumbnailCache(directory, 400, 200)
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	thumbnails := 0
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".jpg") {
			thumbnails++
		}
	}
	if thumbnails != 2 {
		t.Fatalf("cache did not shrink to target: %d files", thumbnails)
	}
	if _, err = os.Stat(filepath.Join(directory, "keep.txt")); err != nil {
		t.Fatal("pruner deleted a non-cache file")
	}
}
