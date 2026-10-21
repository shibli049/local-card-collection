package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed static/*
var assets embed.FS

func writeJSON(w http.ResponseWriter, status int, value any) {
	if status >= 400 {
		if body, ok := value.(map[string]string); ok {
			if message, ok := body["error"]; ok {
				recordError(w, errors.New(message))
				body["error"] = userError(errors.New(message))
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func badRequest(w http.ResponseWriter, err error) {
	recordError(w, err)
	writeJSON(w, 400, map[string]string{"error": userError(err)})
}
func decode(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1000000)
	d := json.NewDecoder(r.Body)
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}
	return nil
}
func (c *Catalog) handler() http.Handler {
	return c.handlerForHost("127.0.0.1")
}
func (c *Catalog) handlerForHost(listenHost string) http.Handler {
	files, _ := fs.Sub(assets, "static")
	static := http.FileServer(http.FS(files))
	return c.logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		if !allowedRequestHost(r.Host, listenHost == "0.0.0.0") {
			http.Error(w, "Local requests only", 403)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Cross-site request rejected", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "Cross-origin request rejected", 403)
			return
		}
		if r.Method == http.MethodPost {
			if r.URL.Path == "/api/login" || r.URL.Path == "/api/logout" {
				if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					badRequest(w, fmt.Errorf("expected application/json"))
					return
				}
				if r.URL.Path == "/api/login" {
					c.loginHTTP(w, r)
				} else {
					c.logoutHTTP(w, r)
				}
				return
			}
			if !c.authenticated(r) {
				writeJSON(w, 401, map[string]string{"error": "Administrator sign-in is required."})
				return
			}
			if r.URL.Path == "/api/import" {
				c.importHTTP(w, r)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				badRequest(w, fmt.Errorf("expected application/json"))
				return
			}
			switch r.URL.Path {
			case "/api/delete-selection":
				var input struct {
					Paths   []string `json:"paths"`
					Folders []string `json:"folders"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				if err := c.deleteSelection(input.Paths, input.Folders); err != nil {
					badRequest(w, err)
					return
				}
				writeJSON(w, 200, map[string]bool{"deleted": true})
			case "/api/delete-album":
				var input struct {
					Folder string `json:"folder"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				if err := c.deleteAlbum(input.Folder); err != nil {
					badRequest(w, err)
					return
				}
				writeJSON(w, 200, map[string]bool{"deleted": true})
			case "/api/transfer":
				var input struct {
					Paths       []string `json:"paths"`
					Destination string   `json:"destination"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				if err := c.transfer(input.Paths, input.Destination); err != nil {
					badRequest(w, err)
					return
				}
				writeJSON(w, 200, map[string]int{"moved": len(input.Paths)})
			case "/api/arrange":
				var input struct {
					Path        string `json:"path"`
					Target      string `json:"target"`
					Destination Slot   `json:"destination"`
					Expected    Slot   `json:"expected"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				if err := c.arrange(input.Path, input.Target, input.Destination, input.Expected); err != nil {
					badRequest(w, err)
					return
				}
				writeJSON(w, 200, map[string]bool{"saved": true})
			case "/api/layout":
				var input struct {
					Folder string `json:"folder"`
					Album  Album  `json:"album"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				if err := c.setLayout(input.Folder, input.Album); err != nil {
					badRequest(w, err)
					return
				}
				writeJSON(w, 200, map[string]bool{"saved": true})
			case "/api/export":
				var input struct {
					Albums []string `json:"albums"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				c.exportSelectionCSV(w, r, input.Albums)
			case "/api/import-plan":
				c.importPlanHTTP(w, r)
			case "/api/scan":
				var input struct {
					Root string `json:"root"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				count, err := c.scan(strings.TrimSpace(input.Root))
				if err != nil {
					badRequest(w, err)
					return
				}
				writeJSON(w, 200, map[string]int{"count": count})
			case "/api/description":
				var input struct {
					Path        string    `json:"path"`
					Description *string   `json:"description"`
					Price       *string   `json:"price"`
					Name        *string   `json:"name"`
					Tags        *[]string `json:"tags"`
				}
				if err := decode(w, r, &input); err != nil {
					badRequest(w, err)
					return
				}
				if input.Description == nil {
					badRequest(w, fmt.Errorf("description must be text"))
					return
				}
				if err := c.editCard(input.Path, *input.Description, input.Price, input.Name, input.Tags); err != nil {
					badRequest(w, err)
					return
				}
				writeJSON(w, 200, map[string]bool{"saved": true})
			default:
				http.NotFound(w, r)
			}
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD, POST")
			http.Error(w, "Method not allowed", 405)
			return
		}
		switch r.URL.Path {
		case "/api/export":
			if !c.authenticated(r) {
				writeJSON(w, 401, map[string]string{"error": "Administrator sign-in is required."})
				return
			}
			c.exportCSV(w, r)
		case "/api/session":
			authenticated := c.authenticated(r)
			username := ""
			if authenticated {
				username = adminUsername
			}
			writeJSON(w, 200, map[string]any{"authenticated": authenticated, "username": username})
		case "/api/images":
			q := r.URL.Query()
			page, _ := strconv.Atoi(q.Get("page"))
			var result Listing
			if q.Get("browse") == "albums" {
				result = c.listAlbums(q.Get("q"), page, q.Get("all") == "1")
			} else {
				result = c.list(Filters{Query: q.Get("q"), Folder: q.Get("folder"), Sheet: q.Get("sheet"), Sort: q.Get("sort"), Page: page})
				if q.Get("layout") != "" {
					slots, slotErr := c.slotListing(Filters{Folder: q.Get("folder"), Sheet: q.Get("sheet"), Page: page})
					if slotErr == nil {
						result = slots
					} else if q.Get("layout") == "1" {
						badRequest(w, slotErr)
						return
					}
				}

				if q.Get("summary") == "0" {
					summaries := result.Folders
					required := map[string]bool{q.Get("folder"): true}
					for _, item := range result.Items {
						required[item.Folder] = true
					}
					result.Folders = []FolderSummary{}
					for i, album := range summaries {
						if i < 40 || required[album.Path] {
							result.Folders = append(result.Folders, album)
						}
					}
				}
			}
			if !c.authenticated(r) {
				result.Root = ""
			}
			writeJSON(w, 200, result)
		case "/thumbnail":
			c.serveThumbnail(w, r)
		case "/image":
			c.serveImage(w, r)
		case "/admin-guide.html":
			w.Header().Set("Cache-Control", "no-store")
			if !c.authenticated(r) {
				http.Error(w, "Please sign in as an administrator in Card Atlas, then open the Admin guide again.", http.StatusUnauthorized)
				return
			}
			source, err := assets.ReadFile("static/admin-guide.md")
			var page []byte
			if err == nil {
				page, err = renderAdminGuide(source)
			}
			if err != nil {
				recordError(w, err)
				http.Error(w, "Could not open the Admin guide. Close this window and try again.", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(page)
		case "/", "/app.js", "/style.css", "/admin-tour.js":
			static.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}
func (c *Catalog) serveImage(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("path")
	c.mu.RLock()
	root := c.data.Root
	item, exists := c.data.Images[key]
	c.mu.RUnlock()
	if root == "" || !exists || !item.Present {
		imageUnavailable(w, nil)
		return
	}
	// OpenRoot prevents path traversal and symlink escapes even if a link is
	// changed between the scan and the request.
	directory, err := os.OpenRoot(root)
	if err != nil {
		imageUnavailable(w, err)
		return
	}
	defer directory.Close()
	file, err := directory.Open(filepath.FromSlash(key))
	if err != nil {
		imageUnavailable(w, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		imageUnavailable(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()))
	http.ServeContent(w, r, item.Filename, info.ModTime(), file)
}

func imageUnavailable(w http.ResponseWriter, err error) {
	if err != nil {
		recordError(w, err)
	}
	http.Error(w, "This image is no longer available. Reload the collection and try again.", http.StatusNotFound)
}
