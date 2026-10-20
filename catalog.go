package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

type Image struct {
	Path        string `json:"path"`
	Folder      string `json:"folder"`
	Filename    string `json:"filename"`
	Page        *int   `json:"page"`
	Row         *int   `json:"row"`
	Col         *int   `json:"col"`
	Description string `json:"description"`
	Present     bool   `json:"present"`
	Revision    string `json:"revision"`
}
type Album struct {
	Mode    string `json:"mode"`
	Pages   int    `json:"pages"`
	Rows    int    `json:"rows"`
	Columns int    `json:"columns"`
}
type CatalogData struct {
	Version int              `json:"version"`
	Albums  map[string]Album `json:"albums,omitempty"`
	Root    string           `json:"root"`
	Images  map[string]Image `json:"images"`
}
type Catalog struct {
	auth authState
	mu   sync.RWMutex
	file string
	data CatalogData
}

var positionPattern = regexp.MustCompile(`-(\d+)-(\d+)-(\d+)$`)
var imageExtensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".avif": true, ".ico": true}

func openCatalog(directory string) (*Catalog, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	c := &Catalog{file: filepath.Join(directory, "catalog.json"), data: CatalogData{Version: 1, Images: map[string]Image{}, Albums: map[string]Album{}}}
	raw, err := os.ReadFile(c.file)
	if errors.Is(err, os.ErrNotExist) {
		if _, oldErr := os.Stat(filepath.Join(directory, "catalog.sqlite3")); oldErr == nil {
			return nil, errors.New("an older SQLite catalog exists: run python3 migrate_catalog.py first, or choose another --data directory")
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &c.data); err != nil {
		return nil, fmt.Errorf("cannot read catalog; file left untouched: %w", err)
	}
	if c.data.Version != 1 || c.data.Images == nil {
		return nil, errors.New("unsupported catalog format; file left untouched")
	}
	if c.data.Albums == nil {
		c.data.Albums = map[string]Album{}
	}
	return c, nil
}

// Persist the whole snapshot before replacing the in-memory state. Failed writes
// never report success or discard the previous notes.
func (c *Catalog) commit(data CatalogData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(c.file), ".catalog-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, c.file); err != nil {
		return err
	}
	c.data = data
	return nil
}
func (c *Catalog) snapshot() CatalogData {
	next := c.data
	next.Albums = make(map[string]Album, len(c.data.Albums))
	for k, v := range c.data.Albums {
		next.Albums[k] = v
	}
	next.Images = make(map[string]Image, len(c.data.Images))
	for k, v := range c.data.Images {
		next.Images[k] = v
	}
	return next
}
func parsePosition(filename string) (*int, *int, *int) {
	match := positionPattern.FindStringSubmatch(strings.TrimSuffix(filename, filepath.Ext(filename)))
	if match == nil {
		return nil, nil, nil
	}
	values := make([]int, 3)
	for i := range 3 {
		v, err := strconv.Atoi(match[i+1])
		if err != nil {
			return nil, nil, nil
		}
		values[i] = v
	}
	return &values[0], &values[1], &values[2]
}
func expandPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
	}
	return filepath.Abs(path)
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func (c *Catalog) scan(directory string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scanLocked(directory, nil, nil)
}
func (c *Catalog) scanLocked(directory string, albumUpdate map[string]Album, descriptions map[string]string) (int, error) {
	if directory == "" {
		directory = c.data.Root
	}
	if directory == "" {
		return 0, errors.New("enter the full path to an image folder")
	}
	root, err := expandPath(directory)
	if err != nil {
		return 0, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return 0, fmt.Errorf("cannot open folder: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, errors.New("choose a folder, not a file")
	}
	if c.data.Root != "" && c.data.Root != root {
		return 0, errors.New("this catalog uses another folder; use --data with another directory for a separate collection")
	}
	next := c.snapshot()
	next.Root = root
	for k, v := range albumUpdate {
		next.Albums[k] = v
	}
	for k, v := range next.Images {
		v.Present = false
		next.Images[k] = v
	}
	count := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".soft-inventory-import-") {
				return filepath.SkipDir
			}
			return nil
		}
		if !imageExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		if !within(root, resolved) {
			return nil
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		page, row, col := parsePosition(entry.Name())
		if album, ok := next.Albums[filepath.ToSlash(filepath.Dir(relative))]; ok && album.Mode == "box" {
			page, row, col = nil, nil, nil
		}
		next.Images[key] = Image{Path: key, Folder: filepath.ToSlash(filepath.Dir(relative)), Filename: entry.Name(), Page: page, Row: row, Col: col, Description: next.Images[key].Description, Present: true, Revision: fmt.Sprintf("%x-%x", info.ModTime().UnixNano(), info.Size())}
		count++
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("scan stopped; previous catalog kept: %w", err)
	}
	for path, note := range descriptions {
		item := next.Images[path]
		item.Description = note
		next.Images[path] = item
	}
	return count, c.commit(next)
}
func (c *Catalog) describe(path, description string) error {
	if len([]rune(description)) > 20000 {
		return errors.New("description must be under 20,000 characters")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.data.Images[path]
	if !ok || !item.Present {
		return errors.New("image is not in the current catalog; rescan the folder")
	}
	next := c.snapshot()
	item.Description = description
	next.Images[path] = item
	return c.commit(next)
}

type FolderSummary struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
	Album *Album `json:"album,omitempty"`
}
type Listing struct {
	Items        []Image         `json:"items"`
	Total        int             `json:"total"`
	Count        int             `json:"count"`
	Folders      []FolderSummary `json:"folders"`
	Pages        []int           `json:"pages"`
	Unpositioned int             `json:"unpositioned"`
	Root         string          `json:"root"`
	Page         int             `json:"page"`
	PageCount    int             `json:"pageCount"`
}
type Filters struct {
	Query, Folder, Sheet, Sort string
	Page                       int
}

func compareNumber(a, b *int) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	if *a < *b {
		return -1
	}
	if *a > *b {
		return 1
	}
	return 0
}
func albumPointer(albums map[string]Album, key string) *Album {
	if a, ok := albums[key]; ok {
		return &a
	}
	return nil
}
func (c *Catalog) list(filters Filters) Listing {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := Listing{Items: []Image{}, Folders: []FolderSummary{}, Pages: []int{}, Root: c.data.Root}
	folders := map[string]int{}
	pages := map[int]bool{}
	matches := []Image{}
	terms := strings.Fields(strings.ToLower(filters.Query))
	for _, item := range c.data.Images {
		if !item.Present {
			continue
		}
		result.Count++
		folders[item.Folder]++
		if filters.Folder != "" && filters.Folder != item.Folder {
			continue
		}
		haystack := strings.ToLower(item.Path + " " + item.Description)
		for _, v := range []*int{item.Page, item.Row, item.Col} {
			if v != nil {
				haystack += " " + strconv.Itoa(*v)
			}
		}
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if item.Page != nil {
			pages[*item.Page] = true
		} else {
			result.Unpositioned++
		}
		if filters.Sheet == "none" && item.Page != nil {
			continue
		}
		if filters.Sheet != "" && filters.Sheet != "none" {
			if item.Page == nil || strconv.Itoa(*item.Page) != filters.Sheet {
				continue
			}
		}
		matches = append(matches, item)
	}
	for folder, count := range folders {
		result.Folders = append(result.Folders, FolderSummary{Path: folder, Count: count, Album: albumPointer(c.data.Albums, folder)})
	}
	slices.SortFunc(result.Folders, func(a, b FolderSummary) int { return strings.Compare(a.Path, b.Path) })
	for number := range pages {
		result.Pages = append(result.Pages, number)
	}
	slices.Sort(result.Pages)
	slices.SortFunc(matches, func(a, b Image) int {
		if filters.Sort == "name" {
			if cmp := strings.Compare(strings.ToLower(a.Filename), strings.ToLower(b.Filename)); cmp != 0 {
				return cmp
			}
		}
		if cmp := strings.Compare(a.Folder, b.Folder); cmp != 0 {
			return cmp
		}
		for _, pair := range [][2]*int{{a.Page, b.Page}, {a.Row, b.Row}, {a.Col, b.Col}} {
			if cmp := compareNumber(pair[0], pair[1]); cmp != 0 {
				return cmp
			}
		}
		return strings.Compare(a.Path, b.Path)
	})
	result.Total = len(matches)
	result.PageCount = max(1, (result.Total+59)/60)
	result.Page = max(1, min(filters.Page, result.PageCount))
	start := (result.Page - 1) * 60
	end := min(start+60, result.Total)
	result.Items = matches[start:end]
	return result
}
