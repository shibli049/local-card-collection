package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnitPriceValidationAndAtomicSave(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "photo.png")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	price := " 25.00 SEK "
	if err := c.editDetails("photo.png", "Original note", &price); err != nil {
		t.Fatal(err)
	}
	if c.data.Images["photo.png"].Price != "25.00 SEK" {
		t.Fatal("price not saved or trimmed")
	}
	price = strings.Repeat("å", 81)
	if err := c.editDetails("photo.png", "Should not save", &price); err == nil {
		t.Fatal("oversized price accepted")
	}
	if item := c.data.Images["photo.png"]; item.Price != "25.00 SEK" || item.Description != "Original note" {
		t.Fatal("invalid price partially changed details")
	}
	price = strings.Repeat("å", 80)
	if err := c.editDetails("photo.png", "Original note", &price); err != nil {
		t.Fatal("price character limit counted bytes")
	}
	// A failed disk write must preserve both fields in memory and on disk.
	c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
	next := "100 EUR"
	if err := c.editDetails("photo.png", "Should not save", &next); err == nil {
		t.Fatal("failed write reported success")
	}
	if item := c.data.Images["photo.png"]; item.Price != price || item.Description != "Original note" {
		t.Fatal("failed write changed details")
	}
}
func TestUnitExistingCatalogWithoutPrices(t *testing.T) {
	directory := t.TempDir()
	raw := `{"version":1,"root":"","images":{"photo.png":{"path":"photo.png","filename":"photo.png","folder":".","description":"Saved note","present":true}}}`
	if err := os.WriteFile(filepath.Join(directory, "catalog.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := openCatalog(directory)
	if err != nil {
		t.Fatal(err)
	}
	item := c.list(Filters{}).Items[0]
	if item.Price != "" || item.Description != "Saved note" {
		t.Fatal("older catalog did not load with optional price")
	}
}
func TestIntegrationPublicPriceAndAdministratorEdits(t *testing.T) {
	directory := t.TempDir()
	server, admin := startIntegrationServer(t, directory)
	admin.login(t)
	review := ImportReview{Options: ImportOptions{Name: "Box", Mode: "box"}, Files: []string{"photo.png"}}
	admin.plan(t, &review)
	admin.upload(t, review, integrationPNG(t, 100), 200)
	edit := map[string]any{"path": "Box/photo.png", "description": "Blue stamp", "price": " 25.00 SEK "}
	admin.json(t, "/api/description", edit, 200)
	viewer := &integrationClient{client: server.Client(), address: server.URL}
	if item := viewer.listing(t, "").Items[0]; item.Price != "25.00 SEK" {
		t.Fatal("public user cannot see price")
	}
	edit["price"] = "1000 EUR"
	viewer.json(t, "/api/description", edit, 401)
	if viewer.listing(t, "").Items[0].Price != "25.00 SEK" {
		t.Fatal("visitor changed price")
	}
	// Older description-only requests must keep the saved price.
	admin.json(t, "/api/description", map[string]string{"path": "Box/photo.png", "description": "Updated note"}, 200)
	for _, invalid := range []any{123, strings.Repeat("x", 81)} {
		admin.json(t, "/api/description", map[string]any{"path": "Box/photo.png", "description": "Invalid update", "price": invalid}, 400)
	}
	admin.json(t, "/api/scan", map[string]any{}, 200)
	server.Close()
	restarted, nextAdmin := startIntegrationServer(t, directory)
	viewer.address = restarted.URL
	item := viewer.listing(t, "").Items[0]
	if item.Price != "25.00 SEK" || item.Description != "Updated note" {
		t.Fatal("price or description lost after reload/restart")
	}
	nextAdmin.login(t)
	nextAdmin.json(t, "/api/description", map[string]string{"path": "Box/photo.png", "description": "Updated note", "price": ""}, 200)
	if viewer.listing(t, "").Items[0].Price != "" {
		t.Fatal("administrator could not remove price")
	}
}
