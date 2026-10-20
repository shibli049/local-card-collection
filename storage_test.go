package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUnitCatalogStoredInSelectedImageRoot(t *testing.T) {
	settings := t.TempDir()
	root := t.TempDir()
	writeImage(t, root, "Card-1-1-1.png")
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := openCatalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.scan(root); err != nil {
		t.Fatal(err)
	}
	if c.file != filepath.Join(root, "catalog.json") {
		t.Fatalf("catalog stored at %s instead of image root", c.file)
	}
	name := "Rare card"
	price := "25 SEK"
	tags := []string{"Foil"}
	if err = c.editCard("Card-1-1-1.png", "Saved note", &price, &name, &tags); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(settings, "catalog.json")); !os.IsNotExist(err) {
		t.Fatal("new catalog metadata was stored in settings folder")
	}
	location, err := os.ReadFile(filepath.Join(settings, "library-location.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pointer map[string]string
	if err = json.Unmarshal(location, &pointer); err != nil {
		t.Fatal(err)
	}
	if len(pointer) != 1 || pointer["root"] != root {
		t.Fatal("settings file should contain only the image root location")
	}
	reopened, err := openCatalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	if item := reopened.list(Filters{}).Items[0]; item.Name != name || item.Price != price || len(item.Tags) != 1 {
		t.Fatal("restart lost image-root metadata")
	}
	// Another installation can reconnect the same folder and load its metadata.
	other, err := openCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.scan(root); err != nil {
		t.Fatal(err)
	}
	if other.list(Filters{Query: "Foil"}).Total != 1 {
		t.Fatal("reconnecting did not adopt existing catalog")
	}
	// The portable catalog does not keep pointing to the old root after a move.
	moved := t.TempDir()
	for _, name := range []string{"catalog.json", "Card-1-1-1.png"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(moved, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	portable, err := openCatalog(moved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = portable.scan(""); err != nil {
		t.Fatal(err)
	}
	actual, err := filepath.EvalSymlinks(moved)
	if err != nil {
		t.Fatal(err)
	}
	if portable.data.Root != actual || portable.list(Filters{Query: "Saved note"}).Total != 1 {
		t.Fatal("moved folder did not retain local metadata")
	}
}
func TestUnitLegacyCatalogRelocationPreservesMetadata(t *testing.T) {
	settings := t.TempDir()
	root := t.TempDir()
	writeImage(t, root, "photo.png")
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	legacy := CatalogData{Version: 1, Root: root, Images: map[string]Image{"photo.png": {Path: "photo.png", Folder: ".", Filename: "photo.png", Name: "Legacy name", Description: "Legacy note", Price: "10 EUR", Tags: []string{"Rare"}, Present: true}}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(settings, "catalog.json")
	if err = os.WriteFile(old, raw, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := openCatalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.scan(""); err != nil {
		t.Fatal(err)
	}
	if c.file != filepath.Join(root, "catalog.json") {
		t.Fatal("legacy catalog was not relocated")
	}
	backup, err := os.ReadFile(old)
	if err != nil || !bytes.Equal(backup, raw) {
		t.Fatal("legacy backup was changed")
	}
	if err = c.describe("photo.png", "Updated note"); err != nil {
		t.Fatal(err)
	}
	reopened, err := openCatalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	if item := reopened.list(Filters{}).Items[0]; item.Description != "Updated note" || item.Price != "10 EUR" || item.Name != "Legacy name" {
		t.Fatal("restart preferred stale legacy backup")
	}
}
func TestUnitLocationSaveFailureRollsBackCatalog(t *testing.T) {
	settings := t.TempDir()
	root := t.TempDir()
	writeImage(t, root, "photo.png")
	c, err := openCatalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	// A directory at the settings filename makes the atomic settings write fail.
	if err = os.Mkdir(filepath.Join(settings, "library-location.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = c.scan(root); err == nil {
		t.Fatal("failed location write was ignored")
	}
	if c.data.Root != "" || len(c.data.Images) != 0 {
		t.Fatal("failed connection changed active catalog")
	}
	if _, err = os.Stat(filepath.Join(root, "catalog.json")); !os.IsNotExist(err) {
		t.Fatal("failed connection left an orphaned catalog")
	}
	if _, err = os.Stat(filepath.Join(root, "photo.png")); err != nil {
		t.Fatal("failed connection removed original image")
	}
}

func TestUnitMovedLibraryCanBeReconnectedFromSettings(t *testing.T) {
	settings := t.TempDir()
	parent := t.TempDir()
	root := filepath.Join(parent, "original")
	writeImage(t, root, "photo.png")
	c, err := openCatalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.scan(root); err != nil {
		t.Fatal(err)
	}
	if err = c.describe("photo.png", "Portable note"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	if err = os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	restarted, err := openCatalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.data.Root != "" {
		t.Fatal("missing previous folder should allow reconnecting")
	}
	if _, err = restarted.scan(moved); err != nil {
		t.Fatal(err)
	}
	if restarted.list(Filters{Query: "Portable note"}).Total != 1 {
		t.Fatal("reconnecting moved library lost notes")
	}
}
