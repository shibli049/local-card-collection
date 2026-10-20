package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// These are the fixed administrator credentials requested for this local app.
const adminUsername = "admin"
const adminPassword = "Inventory@2026"
const sessionCookie = "soft_inventory_session"
const sessionLifetime = 8 * time.Hour

type authState struct {
	mu       sync.Mutex
	sessions map[string]time.Time
}

func (c *Catalog) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	c.auth.mu.Lock()
	defer c.auth.mu.Unlock()
	expires, ok := c.auth.sessions[cookie.Value]
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		delete(c.auth.sessions, cookie.Value)
		return false
	}
	return true
}
func (c *Catalog) loginHTTP(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		badRequest(w, err)
		return
	}
	expectedUser, user := sha256.Sum256([]byte(adminUsername)), sha256.Sum256([]byte(input.Username))
	expectedPassword, password := sha256.Sum256([]byte(adminPassword)), sha256.Sum256([]byte(input.Password))
	if subtle.ConstantTimeCompare(expectedUser[:], user[:])&subtle.ConstantTimeCompare(expectedPassword[:], password[:]) != 1 {
		writeJSON(w, 401, map[string]string{"error": "Incorrect username or password."})
		return
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Could not start a session."})
		return
	}
	value := hex.EncodeToString(token)
	expires := time.Now().Add(sessionLifetime)
	c.auth.mu.Lock()
	if c.auth.sessions == nil {
		c.auth.sessions = map[string]time.Time{}
	}
	for key, expiry := range c.auth.sessions {
		if time.Now().After(expiry) {
			delete(c.auth.sessions, key)
		}
	}
	if old, err := r.Cookie(sessionCookie); err == nil {
		delete(c.auth.sessions, old.Value)
	}
	c.auth.sessions[value] = expires
	c.auth.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionLifetime.Seconds()), Expires: expires})
	writeJSON(w, 200, map[string]any{"authenticated": true, "username": adminUsername})
}
func (c *Catalog) logoutHTTP(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		c.auth.mu.Lock()
		delete(c.auth.sessions, cookie.Value)
		c.auth.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"authenticated": false})
}
