package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := openCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func writeImage(t *testing.T, root, path string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestScanSearchAndPersistence(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	for _, path := range []string{"album/part-name-12-3-2.PNG", "album/part-2-1-1.jpg", "box/plain.jpg", "box/Ångström.jpg"} {
		writeImage(t, root, path)
	}
	if count, err := c.scan(root); err != nil || count != 4 {
		t.Fatalf("scan: %d %v", count, err)
	}
	if err := c.describe("album/part-name-12-3-2.PNG", "Brass fitting, blå"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"brass album", "12 3 2", "BLÅ", "ÅNGSTRÖM"} {
		if result := c.list(Filters{Query: query}); result.Total != 1 {
			t.Fatalf("search %s: %+v", query, result)
		}
	}
	result := c.list(Filters{Folder: "album"})
	if *result.Items[0].Page != 2 || *result.Items[1].Page != 12 {
		t.Fatal("pages not sorted numerically")
	}
	if result := c.list(Filters{Sheet: "12"}); result.Total != 1 {
		t.Fatal("page filter failed")
	}
	path := filepath.Join(root, "album", "part-name-12-3-2.PNG")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := c.scan(""); err != nil {
		t.Fatal(err)
	}
	if c.list(Filters{Query: "brass"}).Total != 0 {
		t.Fatal("missing image still present")
	}
	writeImage(t, root, "album/part-name-12-3-2.PNG")
	if _, err := c.scan(""); err != nil {
		t.Fatal(err)
	}
	reopened, err := openCatalog(filepath.Dir(c.file))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.list(Filters{Query: "brass"}).Total != 1 {
		t.Fatal("notes lost after rescan/restart")
	}
	if _, err := c.scan(t.TempDir()); err == nil {
		t.Fatal("root replacement allowed")
	}
}

func TestConnectedFolderTypesAreInferredAndPreserved(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	for _, key := range []string{"Box/plain.jpg", "Album/card-2-3-4.jpg", "Mixed/plain.jpg", "Mixed/card-1-1-1.jpg"} {
		writeImage(t, root, key)
	}
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	for _, folder := range c.listAlbums("", 1, true).Folders {
		want := "paged"
		if folder.Path == "Box" {
			want = "box"
		}
		if folder.Album == nil || folder.Album.Mode != want {
			t.Fatalf("%s: expected %s, got %+v", folder.Path, want, folder.Album)
		}
	}
	if a := c.data.Albums["Album"]; a != (Album{"paged", 2, 3, 4}) {
		t.Fatalf("wrong dimensions: %+v", a)
	}
	writeImage(t, root, "Box/new-1-1-1.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if c.data.Albums["Box"].Mode != "box" || imageSlot(c.data.Images["Box/new-1-1-1.jpg"]) != (Slot{}) {
		t.Fatal("rescan converted a box into an album")
	}
	if err := c.setLayout("Box", Album{"paged", 1, 1, 1}); err == nil {
		t.Fatal("inferred box can be converted")
	}
	if _, err := c.planImport(ImportReview{Options: ImportOptions{Name: "Box", Mode: "paged", Pages: 1, Rows: 1, Columns: 1}, Files: []string{"new.jpg"}}); err == nil {
		t.Fatal("import can convert an inferred box")
	}
	reopened, err := openCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.data.Albums["Box"].Mode != "box" {
		t.Fatal("box type was not persisted")
	}
}
func TestPagination(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	for i := 1; i <= 65; i++ {
		writeImage(t, root, fmt.Sprintf("album/img-%d-1-1.jpg", i))
	}
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	result := c.list(Filters{Page: 2})
	if len(result.Items) != 5 || result.PageCount != 2 || *result.Items[0].Page != 61 {
		t.Fatalf("bad pagination: %+v", result)
	}
	if c.list(Filters{Page: 999}).Page != 2 {
		t.Fatal("page not clamped")
	}
}
func TestFailedWritesAndCorruptCatalog(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "plain.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	previous := c.file
	c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
	if err := c.describe("plain.jpg", "lost"); err == nil {
		t.Fatal("write should fail")
	}
	if c.data.Images["plain.jpg"].Description != "" {
		t.Fatal("failed write changed memory")
	}
	c.file = previous
	if err := os.WriteFile(c.file, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openCatalog(filepath.Dir(c.file)); err == nil {
		t.Fatal("corrupt data accepted")
	}
	raw, _ := os.ReadFile(c.file)
	if string(raw) != "broken" {
		t.Fatal("corrupt data overwritten")
	}
}
func authorizeTestRequest(c *Catalog, r *http.Request) {
	login := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/login", strings.NewReader(`{"username":"admin","password":"Inventory@2026"}`))
	login.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	c.handler().ServeHTTP(response, login)
	cookies := response.Result().Cookies()
	if len(cookies) == 0 {
		panic("test login failed")
	}
	r.AddCookie(cookies[0])
}
func request(c *Catalog, method, path, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if method == "POST" && path != "/api/login" && path != "/api/logout" {
		authorizeTestRequest(c, r)
	}
	w := httptest.NewRecorder()
	c.handler().ServeHTTP(w, r)
	return w
}
func TestHTTPAndImageBoundaries(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "plain.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/app.js", "/style.css", "/api/images", "/image?path=plain.jpg"} {
		if response := request(c, "GET", path, "", ""); response.Code != 200 {
			t.Fatalf("%s: %d", path, response.Code)
		}
	}
	for _, path := range []string{"/image?path=../outside.jpg", "/image?path=/etc/passwd", "/missing"} {
		if response := request(c, "GET", path, "", ""); response.Code != 404 {
			t.Fatalf("%s: %d", path, response.Code)
		}
	}
	response := request(c, "POST", "/api/description", `{"path":"plain.jpg","description":"saved note"}`, "application/json")
	if response.Code != 200 || c.list(Filters{Query: "saved note"}).Total != 1 {
		t.Fatal("description not saved")
	}
	for _, body := range []string{`{"path":"plain.jpg"}`, `{"path":"plain.jpg","description":42}`, `{} {}`, `not-json`} {
		if request(c, "POST", "/api/description", body, "application/json").Code != 400 {
			t.Fatal("invalid body accepted")
		}
	}
	for _, host := range []string{"evil.example:8765", "127.0.0.1:8765"} {
		r := httptest.NewRequest("POST", "http://"+host+"/api/scan", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://evil.example")
		w := httptest.NewRecorder()
		c.handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("host/origin restriction failed")
		}
	}
}
func TestSymlinkEscape(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.jpg")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.jpg")); err != nil {
		t.Skip("symlink permissions unavailable")
	}
	writeImage(t, root, "safe.jpg")
	if count, err := c.scan(root); err != nil || count != 1 {
		t.Fatalf("scan escape: %d %v", count, err)
	}
	if err := os.Remove(filepath.Join(root, "safe.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "safe.jpg")); err != nil {
		t.Fatal(err)
	}
	if response := request(c, "GET", "/image?path=safe.jpg", "", ""); response.Code != 404 {
		t.Fatal("post-scan symlink escape allowed")
	}
}
func bulkRequest(t *testing.T, c *Catalog, query string, names []string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, name := range names {
		part, err := writer.CreateFormFile("images", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(part, "photo bytes"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/import?"+query, &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	authorizeTestRequest(c, r)
	w := httptest.NewRecorder()
	c.handler().ServeHTTP(w, r)
	return w
}
func TestPagedImportAndBox(t *testing.T) {
	c := newTestCatalog(t)
	// Order received from the preview must determine positions, even when names differ.
	names := []string{"z.jpg", "a.jpg", "b.jpg", "c.jpg", "d.jpg"}
	response := bulkRequest(t, c, "album=Stamps&mode=paged&pages=2&rows=2&columns=2", names)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	for _, path := range []string{"Stamps/z-1-1-1.jpg", "Stamps/a-1-1-2.jpg", "Stamps/b-1-2-1.jpg", "Stamps/c-1-2-2.jpg", "Stamps/d-2-1-1.jpg"} {
		if _, err := os.Stat(filepath.Join(c.data.Root, filepath.FromSlash(path))); err != nil {
			t.Fatal(path, err)
		}
	}
	if c.data.Albums["Stamps"].Pages != 2 {
		t.Fatal("dimensions not saved")
	}
	if err := c.describe("Stamps/d-2-1-1.jpg", "A rare stamp"); err != nil {
		t.Fatal(err)
	}
	response = bulkRequest(t, c, "album=Box+04&mode=box", []string{"plain.jpg", "original-12-3-2.png"})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if _, err := c.scan(""); err != nil {
		t.Fatal(err)
	}
	reopened, err := openCatalog(filepath.Dir(c.file))
	if err != nil {
		t.Fatal(err)
	}
	box := reopened.list(Filters{Folder: "Box 04"})
	if box.Total != 2 || box.Items[0].Page != nil || box.Items[1].Page != nil {
		t.Fatalf("box positions inferred: %+v", box)
	}
	if reopened.list(Filters{Query: "rare stamp"}).Total != 1 {
		t.Fatal("description lost")
	}
	var listing Listing
	response = request(c, "GET", "/api/images?folder="+url.QueryEscape("Box 04"), "", "")
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Total != 2 {
		t.Fatal("folder filter did not return box")
	}
}
func TestImportValidationAndRollback(t *testing.T) {
	c := newTestCatalog(t)
	cases := []struct {
		query string
		files []string
	}{
		{"album=TooMany&mode=paged&pages=1&rows=1&columns=1", []string{"a.jpg", "b.jpg"}},
		{"album=Duplicates&mode=box", []string{"a.jpg", "A.jpg"}},
		{"album=Unsupported&mode=box", []string{"a.jpg", "script.html"}},
		{"album=Empty&mode=box", nil},
		{"album=../Escape&mode=box", []string{"a.jpg"}},
		{"album=CON&mode=box", []string{"a.jpg"}},
		{"album=BadDimensions&mode=paged&pages=1&rows=0&columns=1", []string{"a.jpg"}},
	}
	for _, test := range cases {
		response := bulkRequest(t, c, test.query, test.files)
		if response.Code != 400 {
			t.Fatalf("accepted %s: %s", test.query, response.Body.String())
		}
		if c.list(Filters{}).Count != 0 {
			t.Fatal("partial import committed")
		}
	}
	managed := filepath.Join(filepath.Dir(c.file), "images")
	entries, _ := os.ReadDir(managed)
	if len(entries) != 0 {
		t.Fatalf("failed imports left files: %v", entries)
	}
	response := bulkRequest(t, c, "album=Safe&mode=box", []string{"a.jpg"})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	response = bulkRequest(t, c, "album=Safe&mode=box", []string{"a.jpg"})
	if response.Code != 400 {
		t.Fatal("existing album overwritten")
	}
	raw, _ := os.ReadFile(filepath.Join(c.data.Root, "Safe", "a.jpg"))
	if string(raw) != "photo bytes" {
		t.Fatal("existing photo changed")
	}
	// Simulate a save failure after uploaded files have reached the album folder.
	c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
	response = bulkRequest(t, c, "album=Rollback&mode=box", []string{"new.jpg"})
	if response.Code != 400 {
		t.Fatal("save failure not reported")
	}
	if _, err := os.Stat(filepath.Join(c.data.Root, "Rollback")); !os.IsNotExist(err) {
		t.Fatal("failed save left partial album")
	}
	if c.list(Filters{}).Count != 1 {
		t.Fatal("failed save changed catalog")
	}
}

func reviewedUpload(t *testing.T, c *Catalog, review ImportReview, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormField("review")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(part).Encode(review); err != nil {
		t.Fatal(err)
	}
	for _, name := range review.Files {
		part, err := writer.CreateFormFile("images", name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(part, content)
	}
	writer.Close()
	o := review.Options
	query := url.Values{"album": {o.Name}, "mode": {o.Mode}, "pages": {strconv.Itoa(o.Pages)}, "rows": {strconv.Itoa(o.Rows)}, "columns": {strconv.Itoa(o.Columns)}, "startPage": {strconv.Itoa(o.StartPage)}, "startRow": {strconv.Itoa(o.StartRow)}, "startColumn": {strconv.Itoa(o.StartColumn)}}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/import?"+query.Encode(), &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	authorizeTestRequest(c, r)
	w := httptest.NewRecorder()
	c.handler().ServeHTTP(w, r)
	return w
}
func reviewPlan(t *testing.T, c *Catalog, review *ImportReview) ImportPlan {
	t.Helper()
	raw, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	response := request(c, "POST", "/api/import-plan", string(raw), "application/json")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var plan ImportPlan
	if err = json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	review.Fingerprint = plan.Fingerprint
	return plan
}
func TestExistingAlbumReplaceRenameAndNotes(t *testing.T) {
	c := newTestCatalog(t)
	if r := bulkRequest(t, c, "album=Album&mode=paged&pages=2&rows=2&columns=2", []string{"a.jpg", "b.jpg"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	price := "25.00 SEK"
	if err := c.editDetails("Album/a-1-1-1.jpg", "Keep this note", &price); err != nil {
		t.Fatal(err)
	}
	review := ImportReview{Options: ImportOptions{Name: "Album", Mode: "paged", Pages: 2, Rows: 2, Columns: 2}, Files: []string{"a.jpg", "b.jpg"}, Actions: map[int]string{}}
	pending := reviewPlan(t, c, &review)
	if pending.Unresolved != 2 {
		t.Fatalf("wanted two conflicts: %+v", pending)
	}
	if r := reviewedUpload(t, c, review, "changed"); r.Code != 400 {
		t.Fatal("unresolved conflicts allowed")
	}
	review.Actions = map[int]string{0: "replace", 1: "rename"}
	resolved := reviewPlan(t, c, &review)
	if resolved.Unresolved != 0 || resolved.Entries[1].Filename != "b-1-2-1.jpg" {
		t.Fatalf("bad mixed resolution: %+v", resolved)
	}
	if r := reviewedUpload(t, c, review, "new bytes"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(c.data.Root, "Album", "a-1-1-1.jpg"))
	if err != nil || string(raw) != "new bytes" {
		t.Fatal("replacement bytes missing")
	}
	if c.data.Images["Album/a-1-1-1.jpg"].Description != "Keep this note" {
		t.Fatal("replacement lost description")
	}
	if c.list(Filters{Folder: "Album"}).Total != 3 {
		t.Fatal("rename did not retain both photos")
	}
	// An occupied slot also conflicts when the incoming basename is different.
	review.Files = []string{"different.png"}
	review.Actions = map[int]string{0: "replace"}
	reviewPlan(t, c, &review)
	if r := reviewedUpload(t, c, review, "png bytes"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if _, err = os.Stat(filepath.Join(c.data.Root, "Album", "a-1-1-1.jpg")); !os.IsNotExist(err) {
		t.Fatal("old slot image was not replaced")
	}
	if c.data.Images["Album/different-1-1-1.png"].Price != price {
		t.Fatal("replacement lost price")
	}
	if c.data.Images["Album/b-1-2-1.jpg"].Price != "" {
		t.Fatal("renamed copy inherited a price")
	}
	if c.data.Images["Album/different-1-1-1.png"].Description != "Keep this note" {
		t.Fatal("description not transferred to replacement")
	}
}
func TestBoxConflictsAndDuplicatesWithinSelection(t *testing.T) {
	c := newTestCatalog(t)
	if r := bulkRequest(t, c, "album=Box&mode=box", []string{"a.jpg", "b.jpg"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	review := ImportReview{Options: ImportOptions{Name: "Box", Mode: "box"}, Files: []string{"a.jpg", "b.jpg"}, Actions: map[int]string{0: "replace", 1: "rename"}}
	plan := reviewPlan(t, c, &review)
	if plan.Unresolved != 0 || plan.Entries[1].Filename != "b (2).jpg" {
		t.Fatalf("box resolution: %+v", plan)
	}
	if r := reviewedUpload(t, c, review, "new bytes"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if c.list(Filters{Folder: "Box"}).Total != 3 {
		t.Fatal("box rename failed")
	}
	review = ImportReview{Options: ImportOptions{Name: "New box", Mode: "box"}, Files: []string{"same.jpg", "SAME.jpg"}, Actions: map[int]string{1: "rename"}}
	plan = reviewPlan(t, c, &review)
	if plan.Entries[1].CanReplace || plan.Unresolved != 0 {
		t.Fatal("duplicate selection must be renamed")
	}
	if r := reviewedUpload(t, c, review, "photo"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
}
func TestStaleImportReviewAndReplaceRollback(t *testing.T) {
	c := newTestCatalog(t)
	if r := bulkRequest(t, c, "album=Box&mode=box", []string{"a.jpg"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	review := ImportReview{Options: ImportOptions{Name: "Box", Mode: "box"}, Files: []string{"a.jpg"}, Actions: map[int]string{0: "replace"}}
	reviewPlan(t, c, &review)
	writeImage(t, c.data.Root, "Box/external.jpg")
	if r := reviewedUpload(t, c, review, "new bytes"); r.Code != 400 {
		t.Fatal("stale folder review accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(c.data.Root, "Box", "a.jpg"))
	if string(raw) != "photo bytes" {
		t.Fatal("stale review overwrote existing photo")
	}
	reviewPlan(t, c, &review)
	c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
	if r := reviewedUpload(t, c, review, "replacement"); r.Code != 400 {
		t.Fatal("failed save accepted")
	}
	raw, _ = os.ReadFile(filepath.Join(c.data.Root, "Box", "a.jpg"))
	if string(raw) != "photo bytes" {
		t.Fatal("failed replacement did not restore original")
	}
	// Reload includes additions made by other programs, while keeping old notes.
	c.file = filepath.Join(c.data.Root, "catalog.json")
	if err := c.describe("Box/a.jpg", "Original note"); err != nil {
		t.Fatal(err)
	}
	if r := request(c, "POST", "/api/scan", `{}`, "application/json"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if c.list(Filters{Folder: "Box"}).Total != 2 || c.data.Images["Box/a.jpg"].Description != "Original note" {
		t.Fatal("reload lost notes or external image")
	}
}
func TestRenameCapacityAndStartPosition(t *testing.T) {
	c := newTestCatalog(t)
	if r := bulkRequest(t, c, "album=Album&mode=paged&pages=1&rows=1&columns=1", []string{"a.jpg"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	review := ImportReview{Options: ImportOptions{Name: "Album", Mode: "paged", Pages: 1, Rows: 1, Columns: 1}, Files: []string{"a.jpg"}, Actions: map[int]string{0: "rename"}}
	if _, err := c.planImport(review); err == nil {
		t.Fatal("rename allowed without a free slot")
	}
	review.Options.Pages = 2
	reviewPlan(t, c, &review)
	if r := reviewedUpload(t, c, review, "new"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if c.data.Images["Album/a-2-1-1.jpg"].Page == nil {
		t.Fatal("rename did not use new page")
	}
	review.Options.Pages = 3
	review.Options.StartPage = 3
	review.Files = []string{"next.jpg"}
	review.Actions = nil
	plan := reviewPlan(t, c, &review)
	if plan.Unresolved != 0 || *plan.Entries[0].Page != 3 {
		t.Fatal("starting position ignored")
	}
	if r := reviewedUpload(t, c, review, "new"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
}
