package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func publicCall(c *Catalog, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	c.handler().ServeHTTP(w, r)
	return w
}
func TestPublicReadOnlyAndProtectedActions(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "plain.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if err := c.describe("plain.jpg", "Searchable note"); err != nil {
		t.Fatal(err)
	}
	listing := publicCall(c, "GET", "/api/images?q=searchable", "", nil)
	if listing.Code != 200 {
		t.Fatal("public search blocked")
	}
	var data Listing
	if err := json.Unmarshal(listing.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Total != 1 || data.Root != "" {
		t.Fatal("public listing wrong or exposed root path")
	}
	if r := publicCall(c, "GET", "/image?path=plain.jpg", "", nil); r.Code != 200 {
		t.Fatal("public image viewing blocked")
	}
	for _, path := range []string{"/api/scan", "/api/description", "/api/import-plan", "/api/import"} {
		r := publicCall(c, "POST", path, `{"path":"plain.jpg","description":"unauthorized"}`, nil)
		if r.Code != 401 {
			t.Fatalf("public mutation %s returned %d", path, r.Code)
		}
	}
	if c.data.Images["plain.jpg"].Description != "Searchable note" {
		t.Fatal("public user changed a description")
	}
	forged := &http.Cookie{Name: sessionCookie, Value: "invented"}
	if publicCall(c, "POST", "/api/scan", `{}`, forged).Code != 401 {
		t.Fatal("forged session accepted")
	}
}
func TestLoginLogoutAndSessionExpiry(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "plain.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if response := publicCall(c, "POST", "/api/login", `{"username":"admin","password":"wrong"}`, nil); response.Code != 401 || len(response.Result().Cookies()) != 0 {
		t.Fatal("wrong password accepted")
	}
	login := publicCall(c, "POST", "/api/login", `{"username":"admin","password":"Inventory@2026"}`, nil)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session cookie")
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge <= 0 {
		t.Fatal("missing cookie protections")
	}
	session := publicCall(c, "GET", "/api/session", "", cookie)
	if !strings.Contains(session.Body.String(), `"authenticated":true`) {
		t.Fatal("session not recognized")
	}
	if publicCall(c, "POST", "/api/description", `{"path":"plain.jpg","description":"admin note"}`, cookie).Code != 200 {
		t.Fatal("admin edit blocked")
	}
	if publicCall(c, "POST", "/api/logout", `{}`, cookie).Code != 200 {
		t.Fatal("logout failed")
	}
	if publicCall(c, "POST", "/api/description", `{"path":"plain.jpg","description":"after logout"}`, cookie).Code != 401 {
		t.Fatal("session valid after logout")
	}
	if c.data.Images["plain.jpg"].Description != "admin note" {
		t.Fatal("logged out user changed note")
	}
	login = publicCall(c, "POST", "/api/login", `{"username":"admin","password":"Inventory@2026"}`, nil)
	cookie = login.Result().Cookies()[0]
	c.auth.mu.Lock()
	c.auth.sessions[cookie.Value] = time.Now().Add(-time.Second)
	c.auth.mu.Unlock()
	if publicCall(c, "POST", "/api/scan", `{}`, cookie).Code != 401 {
		t.Fatal("expired session accepted")
	}
	reopened, err := openCatalog(filepath.Dir(c.file))
	if err != nil {
		t.Fatal(err)
	}
	if publicCall(reopened, "POST", "/api/scan", `{}`, cookie).Code != 401 {
		t.Fatal("session survived server restart")
	}
}
