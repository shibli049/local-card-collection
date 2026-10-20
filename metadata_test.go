package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUnitExtractedCardNames(t *testing.T) {
	for _, tc := range []struct {
		file       string
		positioned bool
		want       string
	}{
		{"Blue_Eyes-White-Dragon-12-3-2.PNG", true, "Blue Eyes White Dragon"},
		{"Pikachu.jpg", false, "Pikachu"},
		{"Box_card-12-3-2.png", false, "Box card 12 3 2"},
	} {
		if got := extractedName(tc.file, tc.positioned); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.file, got, tc.want)
		}
	}
}
func TestIntegrationNamesTagsImportSearchAndRestart(t *testing.T) {
	directory := t.TempDir()
	server, admin := startIntegrationServer(t, directory)
	admin.login(t)
	review := ImportReview{Options: ImportOptions{Name: "Cards", Mode: "paged", Pages: 2, Rows: 1, Columns: 1}, Files: []string{"Blue_Eyes-White-Dragon.png"}}
	admin.plan(t, &review)
	admin.upload(t, review, integrationPNG(t, 100), 200)
	item := admin.listing(t, "").Items[0]
	if item.Name != "Blue Eyes White Dragon" {
		t.Fatalf("name not extracted: %q", item.Name)
	}
	edit := map[string]any{"path": item.Path, "description": "First edition", "name": "Blue Eyes Dragon", "tags": []string{" Rare ", "Japanese", "rare", ""}, "price": "25 SEK"}
	admin.json(t, "/api/description", edit, 200)
	// Logging out makes subsequent reads public and edits unauthorized.
	admin.json(t, "/api/logout", map[string]any{}, 200)
	result := admin.listing(t, "q=dragon+japanese")
	if result.Total != 1 || !reflect.DeepEqual(result.Items[0].Tags, []string{"Rare", "Japanese"}) || result.Items[0].Name != "Blue Eyes Dragon" {
		t.Fatal("public name/tag search or normalization failed")
	}
	admin.json(t, "/api/description", edit, 401)
	admin.login(t)
	admin.json(t, "/api/scan", map[string]any{}, 200)
	// A different incoming filename replaces the same physical position.
	review.Files = []string{"new-photo.png"}
	review.Actions = map[int]string{0: "replace"}
	admin.plan(t, &review)
	admin.upload(t, review, integrationPNG(t, 200), 200)
	server.Close()
	_, next := startIntegrationServer(t, directory)
	restored := next.listing(t, "q=japanese").Items[0]
	if restored.Name != "Blue Eyes Dragon" || !reflect.DeepEqual(restored.Tags, []string{"Rare", "Japanese"}) || restored.Price != "25 SEK" {
		t.Fatal("replacement or restart lost metadata")
	}
	next.login(t)
	next.json(t, "/api/description", map[string]any{"path": restored.Path, "description": restored.Description, "name": restored.Name, "tags": []string{}}, 200)
	if len(next.listing(t, "").Items[0].Tags) != 0 {
		t.Fatal("could not clear tags")
	}
}
func TestUnitInvalidMetadataDoesNotPartiallySave(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Card-1-1-1.png")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	name := "Edited card"
	tags := []string{"Rare"}
	if err := c.editCard("Card-1-1-1.png", "Saved note", nil, &name, &tags); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", strings.Repeat("x", 201)} {
		if err := c.editCard("Card-1-1-1.png", "Changed", nil, &bad, nil); err == nil {
			t.Fatal("invalid name accepted")
		}
	}
	for _, bad := range [][]string{{strings.Repeat("x", 61)}, {"bad,tag"}, make([]string, 51)} {
		if err := c.editCard("Card-1-1-1.png", "Changed", nil, nil, &bad); err == nil {
			t.Fatal("invalid tags accepted")
		}
	}
	reopened, err := openCatalog(filepath.Dir(c.file))
	if err != nil {
		t.Fatal(err)
	}
	item := reopened.data.Images["Card-1-1-1.png"]
	if item.Name != name || item.Description != "Saved note" || !reflect.DeepEqual(item.Tags, tags) {
		t.Fatal("invalid changes partially saved")
	}
}
