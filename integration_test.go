package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These tests exercise real HTTP connections, browser-style cookie handling,
// multipart uploads, and the on-disk catalog. Every fixture uses t.TempDir.
type integrationClient struct {
	client  *http.Client
	address string
}

func startIntegrationServer(t *testing.T, directory string) (*httptest.Server, *integrationClient) {
	t.Helper()
	catalog, err := openCatalog(directory)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(catalog.handler())
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return server, &integrationClient{client: &http.Client{Jar: jar, Timeout: 10 * time.Second}, address: server.URL}
}
func (c *integrationClient) call(t *testing.T, method, path string, body io.Reader, contentType string, status int) ([]byte, http.Header) {
	t.Helper()
	request, err := http.NewRequest(method, c.address+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := c.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.StatusCode, status, raw)
	}
	return raw, response.Header
}
func (c *integrationClient) json(t *testing.T, path string, value any, status int) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	response, _ := c.call(t, "POST", path, bytes.NewReader(raw), "application/json", status)
	return response
}
func (c *integrationClient) listing(t *testing.T, query string) Listing {
	t.Helper()
	raw, _ := c.call(t, "GET", "/api/images?"+query, nil, "", 200)
	var listing Listing
	if err := json.Unmarshal(raw, &listing); err != nil {
		t.Fatal(err)
	}
	return listing
}
func (c *integrationClient) login(t *testing.T) {
	t.Helper()
	c.json(t, "/api/login", map[string]string{"username": "admin", "password": "Inventory@2026"}, 200)
}
func (c *integrationClient) plan(t *testing.T, review *ImportReview) ImportPlan {
	t.Helper()
	raw := c.json(t, "/api/import-plan", review, 200)
	var plan ImportPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	review.Fingerprint = plan.Fingerprint
	return plan
}
func multipartImport(t *testing.T, review ImportReview, photo []byte) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	field, err := writer.CreateFormField("review")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(field).Encode(review); err != nil {
		t.Fatal(err)
	}
	for _, name := range review.Files {
		part, err := writer.CreateFormFile("images", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(photo); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), writer.FormDataContentType()
}
func importQuery(options ImportOptions) string {
	return url.Values{"album": {options.Name}, "mode": {options.Mode}, "pages": {strconv.Itoa(options.Pages)}, "rows": {strconv.Itoa(options.Rows)}, "columns": {strconv.Itoa(options.Columns)}, "startPage": {strconv.Itoa(options.StartPage)}, "startRow": {strconv.Itoa(options.StartRow)}, "startColumn": {strconv.Itoa(options.StartColumn)}}.Encode()
}
func (c *integrationClient) upload(t *testing.T, review ImportReview, photo []byte, status int) {
	t.Helper()
	body, contentType := multipartImport(t, review, photo)
	c.call(t, "POST", "/api/import?"+importQuery(review.Options), bytes.NewReader(body), contentType, status)
}
func integrationPNG(t *testing.T, shade uint8) []byte {
	t.Helper()
	var data bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: shade, A: 255})
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestIntegrationAlbumLifecycle(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	server, admin := startIntegrationServer(t, directory)
	viewer := &integrationClient{client: &http.Client{Timeout: 10 * time.Second}, address: server.URL}
	if listing := viewer.listing(t, ""); listing.Count != 0 || listing.Root != "" {
		t.Fatal("new public catalog is not empty")
	}
	// The anonymous client remains independent of the administrator cookie jar.
	admin.login(t)
	options := ImportOptions{Name: "Album", Mode: "paged", Pages: 2, Rows: 1, Columns: 2}
	review := ImportReview{Options: options, Files: []string{"photo1.png", "photo2.png"}}
	if plan := admin.plan(t, &review); plan.Unresolved != 0 {
		t.Fatal("new import has conflicts")
	}
	original := integrationPNG(t, 100)
	admin.upload(t, review, original, 200)
	listing := admin.listing(t, "folder=Album")
	if listing.Total != 2 || listing.Root == "" {
		t.Fatal("import or administrator root path missing")
	}
	root := listing.Root
	note := map[string]string{"path": "Album/photo1-1-1-1.png", "description": "Rare blue stamp"}
	admin.json(t, "/api/description", note, 200)
	found := viewer.listing(t, "q=rare+blue&folder=Album&sheet=1")
	if found.Total != 1 || found.Root != "" || found.Items[0].Description != note["description"] {
		t.Fatal("public search did not find saved description")
	}
	viewer.json(t, "/api/description", note, 401)
	photoPath := "/image?path=" + url.QueryEscape(note["path"])
	served, headers := viewer.call(t, "GET", photoPath, nil, "", 200)
	if !bytes.Equal(served, original) || !strings.HasPrefix(headers.Get("Content-Type"), "image/png") {
		t.Fatal("image content not served correctly")
	}
	cachedRequest, err := http.NewRequest("GET", viewer.address+photoPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	cachedRequest.Header.Set("If-None-Match", headers.Get("ETag"))
	cached, err := viewer.client.Do(cachedRequest)
	if err != nil {
		t.Fatal(err)
	}
	cached.Body.Close()
	if headers.Get("ETag") == "" || cached.StatusCode != 304 {
		t.Fatal("conditional image request failed")
	}
	// A file copied outside the app is picked up by the authenticated reload.
	external := filepath.Join(root, "Album", "external-2-1-1.png")
	if err = os.WriteFile(external, original, 0600); err != nil {
		t.Fatal(err)
	}
	viewer.json(t, "/api/scan", map[string]any{}, 401)
	admin.json(t, "/api/scan", map[string]any{}, 200)
	if admin.listing(t, "folder=Album").Total != 3 {
		t.Fatal("reload missed external photo")
	}
	// Choose Replace for one conflict and Rename for the other in one request.
	review.Actions = map[int]string{0: "replace", 1: "rename"}
	plan := admin.plan(t, &review)
	if plan.Unresolved != 0 || plan.Entries[1].Filename != "photo2-2-1-2.png" {
		t.Fatalf("wrong mixed plan: %+v", plan)
	}
	replacement := integrationPNG(t, 220)
	admin.upload(t, review, replacement, 200)
	served, newHeaders := viewer.call(t, "GET", photoPath, nil, "", 200)
	if !bytes.Equal(served, replacement) || newHeaders.Get("ETag") == headers.Get("ETag") {
		t.Fatal("replaced image or cache revision did not change")
	}
	if result := viewer.listing(t, "folder=Album"); result.Total != 4 {
		t.Fatal("rename did not keep both photos")
	}
	if viewer.listing(t, "q=rare+blue").Total != 1 {
		t.Fatal("replacement lost description")
	}
	admin.json(t, "/api/logout", map[string]any{}, 200)
	admin.json(t, "/api/description", note, 401)
	// A new handler loads the persisted files and notes, but no old session.
	admin.login(t)
	server.Close()
	restarted, _ := startIntegrationServer(t, directory)
	admin.address = restarted.URL
	admin.json(t, "/api/scan", map[string]any{}, 401)
	persisted := admin.listing(t, "q=rare+blue")
	if persisted.Total != 1 || persisted.Items[0].Description != note["description"] {
		t.Fatal("restart lost saved description")
	}
	admin.login(t)
	if result := admin.listing(t, "folder=Album"); result.Total != 4 || result.Folders[0].Album == nil || result.Folders[0].Album.Pages != 2 {
		t.Fatal("restart lost photos or album dimensions")
	}
}
func TestIntegrationInterruptedUploadCanBeRetried(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	_, admin := startIntegrationServer(t, directory)
	admin.login(t)
	review := ImportReview{Options: ImportOptions{Name: "Box", Mode: "box"}, Files: []string{"photo.png"}}
	admin.plan(t, &review)
	photo := integrationPNG(t, 80)
	// Omit the final multipart boundary, as with a truncated upload.
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	field, err := writer.CreateFormField("review")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(field).Encode(review); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("images", "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(photo); err != nil {
		t.Fatal(err)
	}
	admin.call(t, "POST", "/api/import?album=Box&mode=box", &buffer, writer.FormDataContentType(), 400)
	if admin.listing(t, "").Total != 0 {
		t.Fatal("interrupted upload partially committed")
	}
	entries, err := os.ReadDir(filepath.Join(directory, "images"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("interrupted upload left an album or staging files")
	}
	admin.upload(t, review, photo, 200)
	if result := admin.listing(t, "folder=Box"); result.Total != 1 || result.Items[0].Page != nil {
		t.Fatal("retry failed or added positions to box")
	}
	// A destination change after review rejects replacements without touching files.
	review.Actions = map[int]string{0: "replace"}
	admin.plan(t, &review)
	root := admin.listing(t, "").Root
	if err = os.WriteFile(filepath.Join(root, "Box", "external.png"), photo, 0600); err != nil {
		t.Fatal(err)
	}
	admin.upload(t, review, integrationPNG(t, 200), 400)
	saved, err := os.ReadFile(filepath.Join(root, "Box", "photo.png"))
	if err != nil || !bytes.Equal(saved, photo) {
		t.Fatal("stale review overwrote original photo")
	}
}
