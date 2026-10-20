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
	PositionOverride bool     `json:"position_override,omitempty"`
	Order            int      `json:"order,omitempty"`
	Path             string   `json:"path"`
	Folder           string   `json:"folder"`
	Filename         string   `json:"filename"`
	Name             string   `json:"name"`
	Tags             []string `json:"tags"`
	Page             *int     `json:"page"`
	Row              *int     `json:"row"`
	Col              *int     `json:"col"`
	Description      string   `json:"description"`
	Price            string   `json:"price"`
	Present          bool     `json:"present"`
	Revision         string   `json:"revision"`
}
type Album struct {
	Mode    string `json:"mode"`
	Pages   int    `json:"pages"`
	Rows    int    `json:"rows"`
	Columns int    `json:"columns"`
}
type CatalogData struct {
	Version  int              `json:"version"`
	Portable bool             `json:"in_image_root,omitempty"`
	Albums   map[string]Album `json:"albums,omitempty"`
	Root     string           `json:"root"`
	Images   map[string]Image `json:"images"`
}
type Catalog struct {
	index      *catalogIndex
	bootstrap  string
	remembered bool
	auth       authState
	mu         sync.RWMutex
	file       string
	data       CatalogData
}

var positionPattern = regexp.MustCompile(`-(\d+)-(\d+)-(\d+)$`)
var imageExtensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".avif": true, ".ico": true}

func openCatalog(directory string) (*Catalog, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	c := &Catalog{bootstrap: directory, file: filepath.Join(directory, "catalog.json"), data: CatalogData{Version: 1, Images: map[string]Image{}, Albums: map[string]Album{}}}
	if location, err := os.ReadFile(filepath.Join(directory, "library-location.json")); err == nil {
		var saved struct {
			Root string `json:"root"`
		}
		if err = json.Unmarshal(location, &saved); err != nil {
			return nil, fmt.Errorf("cannot read library location: %w", err)
		}
		if saved.Root == "" {
			return nil, errors.New("saved library location is empty")
		}
		if _, err = os.Stat(saved.Root); err == nil {
			c.file = filepath.Join(saved.Root, "catalog.json")
			c.remembered = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		} else {
			return c, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	raw, err := os.ReadFile(c.file)
	if errors.Is(err, os.ErrNotExist) {
		if c.remembered {
			c.data.Root = filepath.Dir(c.file)
			c.data.Portable = true
			return c, nil
		}
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
	if c.data.Portable {
		c.data.Root = filepath.Dir(c.file)
	}
	if c.data.Albums == nil {
		c.data.Albums = map[string]Album{}
	}
	for key, item := range c.data.Images {
		if item.Name == "" {
			item.Name = extractedName(item.Filename, item.Page != nil)
		}
		if item.Tags == nil {
			item.Tags = []string{}
		}
		c.data.Images[key] = item
	}
	return c, nil
}

// Persist the whole snapshot before replacing the in-memory state. Failed writes
// never report success or discard the previous notes.
func atomicCatalogWrite(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".catalog-*.tmp")
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
	return os.Rename(name, path)
}
func (c *Catalog) commit(data CatalogData) error {
	target := c.file
	if data.Root != "" && c.file == filepath.Join(c.bootstrap, "catalog.json") {
		target = filepath.Join(data.Root, "catalog.json")
	}
	data.Portable = data.Root != ""
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	previous, readErr := os.ReadFile(target)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err = atomicCatalogWrite(target, raw); err != nil {
		return err
	}
	if data.Root != "" && c.bootstrap != data.Root && !c.remembered {
		location, _ := json.Marshal(struct {
			Root string `json:"root"`
		}{data.Root})
		if err = atomicCatalogWrite(filepath.Join(c.bootstrap, "library-location.json"), location); err != nil {
			var restoreErr error
			if readErr == nil {
				restoreErr = atomicCatalogWrite(target, previous)
			} else {
				restoreErr = os.Remove(target)
			}
			if restoreErr != nil {
				return fmt.Errorf("cannot remember library: %v; catalog recovery failed: %w", err, restoreErr)
			}
			return fmt.Errorf("cannot remember library location: %w", err)
		}
		c.remembered = true
	}
	c.file = target
	c.data = data
	c.index = nil
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
func extractedName(filename string, positioned bool) string {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	if positioned {
		stem = positionPattern.ReplaceAllString(stem, "")
	}
	name := strings.Join(strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(stem)), " ")
	if name == "" {
		return filename
	}
	return name
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
func (c *Catalog) scanLocked(directory string, albumUpdate map[string]Album, details map[string]imageDetails) (int, error) {
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
	rootCatalog := filepath.Join(root, "catalog.json")
	if rootCatalog != c.file {
		if _, err := os.Stat(rootCatalog); err == nil {
			if c.data.Root != "" {
				return 0, errors.New("this image root already contains another catalog; open it using --data with the image root")
			}
			existing, err := openCatalog(root)
			if err != nil {
				return 0, err
			}
			next = existing.snapshot()
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
	}
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
			if path != root && (strings.HasPrefix(entry.Name(), ".soft-inventory-import-") || entry.Name() == ".card-atlas-cache" || entry.Name() == ".card-atlas-trash" || entry.Name() == ".log") {
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
		previous := next.Images[key]
		if previous.PositionOverride {
			page, row, col = previous.Page, previous.Row, previous.Col
		}
		if _, replacing := details[key]; replacing {
			page, row, col = parsePosition(entry.Name())
			previous.PositionOverride = false
			if album, ok := next.Albums[filepath.ToSlash(filepath.Dir(relative))]; ok && album.Mode == "box" {
				page, row, col = nil, nil, nil
			}
		}
		name := next.Images[key].Name
		if name == "" {
			name = extractedName(entry.Name(), page != nil)
		}
		tags := next.Images[key].Tags
		if tags == nil {
			tags = []string{}
		}
		next.Images[key] = Image{PositionOverride: previous.PositionOverride, Order: previous.Order, Name: name, Tags: tags, Path: key, Folder: filepath.ToSlash(filepath.Dir(relative)), Filename: entry.Name(), Page: page, Row: row, Col: col, Description: next.Images[key].Description, Price: next.Images[key].Price, Present: true, Revision: fmt.Sprintf("%x-%x", info.ModTime().UnixNano(), info.Size())}
		count++
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("scan stopped; previous catalog kept: %w", err)
	}
	// Folder connections infer a type once. Unnumbered folders are boxes;
	// numbered folders retain their physical page layout on future scans.
	detected := map[string]Album{}
	for _, item := range next.Images {
		if !item.Present {
			continue
		}
		if _, saved := next.Albums[item.Folder]; saved {
			continue
		}
		a, exists := detected[item.Folder]
		if !exists {
			a.Mode = "box"
		}
		s := imageSlot(item)
		if s.Page > 0 && s.Page <= 1000 && s.Row > 0 && s.Row <= 100 && s.Col > 0 && s.Col <= 100 {
			a.Mode = "paged"
			a.Pages, a.Rows, a.Columns = max(a.Pages, s.Page), max(a.Rows, s.Row), max(a.Columns, s.Col)
		}
		detected[item.Folder] = a
	}
	for folder, album := range detected {
		next.Albums[folder] = album
	}
	for path, details := range details {
		item := next.Images[path]
		item.Description = details.Description
		item.Price = details.Price
		if details.Name != "" {
			item.Name = details.Name
		}
		item.Tags = details.Tags
		next.Images[path] = item
	}
	return count, c.commit(next)
}

type imageDetails struct {
	Name        string
	Tags        []string
	Description string
	Price       string
}

func (c *Catalog) describe(path, description string) error {
	return c.editDetails(path, description, nil)
}
func (c *Catalog) editDetails(path, description string, price *string) error {
	return c.editCard(path, description, price, nil, nil)
}
func (c *Catalog) editCard(path, description string, price *string, name *string, tags *[]string) error {
	if len([]rune(description)) > 20000 {
		return errors.New("description must be under 20,000 characters")
	}
	if price != nil && len([]rune(*price)) > 80 {
		return errors.New("price must be under 80 characters")
	}
	if name != nil && (strings.TrimSpace(*name) == "" || len([]rune(*name)) > 200) {
		return errors.New("name must contain 1 to 200 characters")
	}
	var normalized []string
	if tags != nil {
		if len(*tags) > 50 {
			return errors.New("use up to 50 tags")
		}
		seen := map[string]bool{}
		normalized = []string{}
		for _, tag := range *tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if len([]rune(tag)) > 60 || strings.ContainsAny(tag, ",\r\n") {
				return errors.New("each tag must be under 60 characters without commas or line breaks")
			}
			key := strings.ToLower(tag)
			if !seen[key] {
				normalized = append(normalized, tag)
				seen[key] = true
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.data.Images[path]
	if !ok || !item.Present {
		return errors.New("image is not in the current catalog; rescan the folder")
	}
	next := c.snapshot()
	item.Description = description
	if name != nil {
		item.Name = strings.TrimSpace(*name)
	}
	if tags != nil {
		item.Tags = normalized
	}
	if price != nil {
		item.Price = strings.TrimSpace(*price)
	}
	next.Images[path] = item
	return c.commit(next)
}

type AlbumCover struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Revision string `json:"revision"`
}
type FolderSummary struct {
	Cover *AlbumCover `json:"cover,omitempty"`
	Path  string      `json:"path"`
	Count int         `json:"count"`
	Album *Album      `json:"album,omitempty"`
}
type Listing struct {
	Slots        []Slot          `json:"slots,omitempty"`
	Mode         string          `json:"mode"`
	AlbumCount   int             `json:"albumCount"`
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
func (c *Catalog) list(filters Filters) Listing { return c.listImages(filters, true) }
func (c *Catalog) listImages(filters Filters, paginate bool) Listing {
	index := c.catalogIndex()
	result := Listing{Mode: "images", AlbumCount: len(index.folders), Items: []Image{}, Folders: index.folders, Pages: []int{}, Root: index.root, Count: len(index.position)}
	source := index.position
	if filters.Sort == "name" {
		source = index.name
	}
	if filters.Folder != "" {
		source = index.byFolder[filters.Folder]
		if filters.Sort == "name" {
			source = index.byFolderName[filters.Folder]
		}
	}
	pages := map[int]bool{}
	matches := []*indexedImage{}
	terms := strings.Fields(strings.ToLower(filters.Query))
	for _, entry := range source {
		item := entry.image
		matched := true
		for _, term := range terms {
			if !strings.Contains(entry.search, term) {
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
		if filters.Sheet != "" && filters.Sheet != "none" && (item.Page == nil || strconv.Itoa(*item.Page) != filters.Sheet) {
			continue
		}
		matches = append(matches, entry)
	}
	for page := range pages {
		result.Pages = append(result.Pages, page)
	}
	slices.Sort(result.Pages)
	result.Total = len(matches)
	pageSize := 60
	if len(terms) > 0 {
		pageSize = 240
	}
	result.PageCount = max(1, (result.Total+pageSize-1)/pageSize)
	result.Page = max(1, min(filters.Page, result.PageCount))
	if !paginate {
		for _, entry := range matches {
			result.Items = append(result.Items, entry.image)
		}
		return result
	}
	start := (result.Page - 1) * pageSize
	for _, entry := range matches[start:min(start+pageSize, len(matches))] {
		result.Items = append(result.Items, entry.image)
	}
	return result
}
