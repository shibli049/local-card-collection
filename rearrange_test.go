package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSparseAlbumMoveSwapReloadAndImportConflicts(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Album/first-1-1-1.jpg")
	writeImage(t, root, "Album/last-3-2-3.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	a := c.catalogIndex().byFolder["Album"]
	if len(a) != 2 {
		t.Fatal("missing photos")
	}
	listing, err := c.slotListing(Filters{Folder: "Album", Sheet: "2", Page: 1})
	if err != nil || len(listing.Slots) != 6 || len(listing.Items) != 0 || len(listing.Pages) != 3 {
		t.Fatalf("empty page not represented: %+v %v", listing, err)
	}
	first := "Album/first-1-1-1.jpg"
	last := "Album/last-3-2-3.jpg"
	if err = c.arrange(first, "", Slot{2, 2, 2}, Slot{1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if err = c.arrange(first, last, Slot{3, 2, 3}, Slot{2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	if imageSlot(c.data.Images[last]) != (Slot{2, 2, 2}) {
		t.Fatal("swap did not retain old position")
	}
	if err = c.arrange(first, "", Slot{1, 1, 2}, Slot{1, 1, 1}); err == nil {
		t.Fatal("stale move accepted")
	}
	if err = c.arrange(first, "", Slot{2, 2, 2}, Slot{3, 2, 3}); err == nil {
		t.Fatal("occupied target silently overwritten")
	}
	if _, err = c.scan(root); err != nil {
		t.Fatal(err)
	}
	reopened, err := openCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if imageSlot(reopened.data.Images[first]) != (Slot{3, 2, 3}) || imageSlot(reopened.data.Images[last]) != (Slot{2, 2, 2}) {
		t.Fatal("moves lost on reload/restart")
	}
	if _, err = os.Stat(filepath.Join(root, first)); err != nil {
		t.Fatal("source filename changed", err)
	}
	review := ImportReview{Options: ImportOptions{Name: "Album", Mode: "paged", Pages: 3, Rows: 2, Columns: 3, StartPage: 2, StartRow: 2, StartColumn: 2}, Files: []string{"new.jpg"}}
	plan, err := c.planImport(review)
	if err != nil || plan.Unresolved != 1 {
		t.Fatalf("import failed to protect moved position: %+v %v", plan, err)
	}
}
func TestRearrangeHTTPAndLayout(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Box/a.jpg")
	writeImage(t, root, "Box/b.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/api/layout", "/api/arrange"} {
		if r := publicCall(c, "POST", p, `{}`, nil); r.Code != 401 {
			t.Fatal("public mutation allowed", p, r.Code)
		}
	}
	login := publicCall(c, "POST", "/api/login", `{"username":"admin","password":"Inventory@2026"}`, nil)
	cookie := login.Result().Cookies()[0]
	move := `{"path":"Box/a.jpg","target":"Box/b.jpg","destination":{},"expected":{}}`
	if r := publicCall(c, "POST", "/api/arrange", move, cookie); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if c.list(Filters{Folder: "Box"}).Items[0].Path != "Box/b.jpg" {
		t.Fatal("box reorder failed")
	}
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if c.list(Filters{Folder: "Box"}).Items[0].Path != "Box/b.jpg" {
		t.Fatal("box order lost on reload")
	}
	if r := publicCall(c, "POST", "/api/layout", `{"folder":"Box","album":{"mode":"paged","pages":2,"rows":2,"columns":2}}`, cookie); r.Code != 400 {
		t.Fatal("box type change allowed", r.Code, r.Body.String())
	}
	writeImage(t, root, "Album/a-1-1-1.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if err := c.setLayout("Album", Album{Mode: "box"}); err == nil {
		t.Fatal("paged album type change allowed")
	}
	if r := publicCall(c, "POST", "/api/layout", `{"folder":"Album","album":{"mode":"paged","pages":1000,"rows":100,"columns":100}}`, cookie); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	r := publicCall(c, "GET", "/api/images?folder=Album&layout=1&sheet=900&page=2", "", nil)
	var l Listing
	if err := json.Unmarshal(r.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	if r.Code != 200 || len(l.Slots) != 60 || len(l.Items) != 0 || l.Slots[0] != (Slot{900, 1, 61}) {
		t.Fatalf("sparse page not bounded: %+v", l)
	}
	if err := c.arrange("Album/a-1-1-1.jpg", "", Slot{900, 50, 50}, Slot{1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.setLayout("Album", Album{"paged", 2, 2, 2}); err == nil {
		t.Fatal("layout clipped an occupied position")
	}
	// Persistence failure leaves the in-memory arrangement untouched.
	c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
	if err := c.arrange("Album/a-1-1-1.jpg", "", Slot{900, 50, 51}, Slot{900, 50, 50}); err == nil {
		t.Fatal("save failure hidden")
	}
	if imageSlot(c.data.Images["Album/a-1-1-1.jpg"]) != (Slot{900, 50, 50}) {
		t.Fatal("failed move mutated metadata")
	}
}

func TestIntegrationRearrangeAndReplaceMovedPosition(t *testing.T) {
	_, admin := startIntegrationServer(t, t.TempDir())
	admin.login(t)
	options := ImportOptions{Name: "Album", Mode: "paged", Pages: 3, Rows: 2, Columns: 3}
	review := ImportReview{Options: options, Files: []string{"first.png"}}
	admin.plan(t, &review)
	admin.upload(t, review, integrationPNG(t, 1), 200)
	path := "Album/first-1-1-1.png"
	admin.json(t, "/api/arrange", map[string]any{"path": path, "target": "", "destination": Slot{2, 2, 2}, "expected": Slot{1, 1, 1}}, 200)
	admin.json(t, "/api/scan", map[string]string{}, 200)
	listing := admin.listing(t, "folder=Album&layout=1&sheet=2")
	if len(listing.Items) != 1 || imageSlot(listing.Items[0]) != (Slot{2, 2, 2}) {
		t.Fatal("HTTP move lost on reload")
	}
	options.StartPage, options.StartRow, options.StartColumn = 2, 2, 2
	replacement := ImportReview{Options: options, Files: []string{"replacement.png"}, Actions: map[int]string{0: "replace"}}
	plan := admin.plan(t, &replacement)
	if len(plan.Entries[0].Replaces) != 1 || plan.Entries[0].Replaces[0] != "first-1-1-1.png" {
		t.Fatal("replace did not find moved photo")
	}
	admin.upload(t, replacement, integrationPNG(t, 2), 200)
	listing = admin.listing(t, "folder=Album&layout=1&sheet=2")
	if len(listing.Items) != 1 || listing.Items[0].Path != "Album/replacement-2-2-2.png" || listing.Items[0].Name != "first" {
		t.Fatalf("replacement lost position or details: %+v", listing.Items)
	}
}
