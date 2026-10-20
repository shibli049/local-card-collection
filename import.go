package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const maxImportFiles = 10000
const maxImageBytes = 50 << 20
const maxImportBytes = 1 << 30

var invalidName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
var windowsDevice = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])(?:\.|$)`)

func validAlbumName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 120 && !invalidName.MatchString(name) && strings.Trim(name, " .") == name && !windowsDevice.MatchString(name) && !strings.HasPrefix(name, ".soft-inventory-")
}
func validAlbumPath(name string) bool {
	if name == "." {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if !validAlbumName(part) {
			return false
		}
	}
	return !strings.Contains(name, `\`)
}
func importedName(original string, index int, album Album) string {
	name := filepath.Base(strings.ReplaceAll(original, `\`, "/"))
	extension := strings.ToLower(filepath.Ext(name))
	stem := strings.Trim(invalidName.ReplaceAllString(strings.TrimSuffix(name, filepath.Ext(name)), "_"), " .")
	if len(stem) > 140 {
		stem = string([]rune(stem)[:min(70, len([]rune(stem)))])
	}
	if stem == "" || windowsDevice.MatchString(stem) {
		stem = "image-" + stem
	}
	if album.Mode == "box" {
		return stem + extension
	}
	stem = positionPattern.ReplaceAllString(stem, "")
	return fmt.Sprintf("%s-%d-%d-%d%s", stem, index/(album.Rows*album.Columns)+1, (index/album.Columns)%album.Rows+1, index%album.Columns+1, extension)
}

type ImportOptions struct {
	Name        string `json:"album"`
	Mode        string `json:"mode"`
	Pages       int    `json:"pages"`
	Rows        int    `json:"rows"`
	Columns     int    `json:"columns"`
	StartPage   int    `json:"startPage"`
	StartRow    int    `json:"startRow"`
	StartColumn int    `json:"startColumn"`
}

func (o ImportOptions) album() Album { return Album{o.Mode, o.Pages, o.Rows, o.Columns} }
func (o *ImportOptions) validate() error {
	o.Name = strings.TrimSpace(o.Name)
	if !validAlbumPath(o.Name) {
		return errors.New("use a valid album or folder name without reserved names or special filename characters")
	}
	if o.Mode == "box" {
		o.Pages, o.Rows, o.Columns = 0, 0, 0
		return nil
	}
	if o.Mode != "paged" {
		return errors.New("choose a paged album or a box")
	}
	if o.Pages < 1 || o.Pages > 1000 || o.Rows < 1 || o.Rows > 100 || o.Columns < 1 || o.Columns > 100 {
		return errors.New("enter 1–1000 pages and 1–100 rows and columns")
	}
	if o.StartPage == 0 {
		o.StartPage = 1
	}
	if o.StartRow == 0 {
		o.StartRow = 1
	}
	if o.StartColumn == 0 {
		o.StartColumn = 1
	}
	if o.StartPage < 1 || o.StartPage > o.Pages || o.StartRow < 1 || o.StartRow > o.Rows || o.StartColumn < 1 || o.StartColumn > o.Columns {
		return errors.New("the starting position must be inside the album")
	}
	return nil
}
func optionsFromRequest(r *http.Request) ImportOptions {
	q := r.URL.Query()
	number := func(key string) int { n, _ := strconv.Atoi(q.Get(key)); return n }
	return ImportOptions{Name: q.Get("album"), Mode: q.Get("mode"), Pages: number("pages"), Rows: number("rows"), Columns: number("columns"), StartPage: number("startPage"), StartRow: number("startRow"), StartColumn: number("startColumn")}
}

type ImportReview struct {
	Options     ImportOptions  `json:"options"`
	Files       []string       `json:"files"`
	Actions     map[int]string `json:"actions"`
	Fingerprint string         `json:"fingerprint"`
}
type ImportEntry struct {
	Index      int      `json:"index"`
	Original   string   `json:"original"`
	Filename   string   `json:"filename"`
	Page       *int     `json:"page"`
	Row        *int     `json:"row"`
	Col        *int     `json:"col"`
	Conflicts  []string `json:"conflicts"`
	CanReplace bool     `json:"canReplace"`
	Action     string   `json:"action"`
	Replaces   []string `json:"replaces"`
}
type ImportPlan struct {
	Entries     []ImportEntry `json:"entries"`
	Fingerprint string        `json:"fingerprint"`
	Unresolved  int           `json:"unresolved"`
}
type diskEntry struct {
	Name     string
	Size     int64
	Modified int64
	Regular  bool
	Slot     int
}

func (c *Catalog) canonicalAlbum(name string) string {
	for key := range c.data.Albums {
		if strings.EqualFold(key, name) {
			return key
		}
	}
	for _, image := range c.data.Images {
		if strings.EqualFold(image.Folder, name) {
			return image.Folder
		}
	}
	return name
}
func (c *Catalog) importLocation(o ImportOptions) (string, string, error) {
	root := c.data.Root
	if root == "" {
		root = filepath.Join(filepath.Dir(c.file), "images")
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	} else if parent, err := filepath.EvalSymlinks(filepath.Dir(root)); err == nil {
		root = filepath.Join(parent, filepath.Base(root))
	}
	destination := filepath.Join(root, filepath.FromSlash(o.Name))
	if !within(root, destination) {
		return "", "", errors.New("album must be inside the image folder")
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", errors.New("album destination must be a regular folder")
		}
		resolved, err := filepath.EvalSymlinks(destination)
		if err != nil || !within(root, resolved) || resolved != destination {
			return "", "", errors.New("album cannot use symbolic-link folders")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	} else if strings.Contains(o.Name, "/") || o.Name == "." {
		return "", "", errors.New("choose an existing folder or a new top-level album name")
	}
	if saved, ok := c.data.Albums[o.Name]; ok {
		if saved.Mode != o.Mode {
			return "", "", errors.New("keep the existing album type")
		}
		if saved.Mode == "paged" && (saved.Rows != o.Rows || saved.Columns != o.Columns || saved.Pages > o.Pages) {
			return "", "", errors.New("keep the existing row and column counts; pages may be increased")
		}
	}
	return root, destination, nil
}
func (c *Catalog) planImport(review ImportReview) (ImportPlan, error) {
	o := review.Options
	o.Name = c.canonicalAlbum(strings.TrimSpace(o.Name))
	if err := o.validate(); err != nil {
		return ImportPlan{}, err
	}
	_, destination, err := c.importLocation(o)
	if err != nil {
		return ImportPlan{}, err
	}
	if len(review.Files) == 0 || len(review.Files) > maxImportFiles {
		return ImportPlan{}, errors.New("select between 1 and 10,000 image files")
	}
	start := 0
	capacity := 0
	if o.Mode == "paged" {
		start = ((o.StartPage-1)*o.Rows+o.StartRow-1)*o.Columns + o.StartColumn - 1
		capacity = o.Pages * o.Rows * o.Columns
		if start+len(review.Files) > capacity {
			return ImportPlan{}, errors.New("more photos than positions from the starting point; increase the page count or select fewer photos")
		}
	}
	entries, err := os.ReadDir(destination)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ImportPlan{}, err
	}
	existing := map[string]diskEntry{}
	slots := map[int][]string{}
	disk := []diskEntry{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".soft-inventory-import-") || entry.Name() == ".card-atlas-cache" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return ImportPlan{}, err
		}
		d := diskEntry{Name: entry.Name(), Size: info.Size(), Modified: info.ModTime().UnixNano(), Regular: info.Mode().IsRegular(), Slot: -1}
		if o.Mode == "paged" && imageExtensions[strings.ToLower(filepath.Ext(d.Name))] {
			p, r, col := parsePosition(d.Name)
			if i, ok := c.data.Images[filepath.ToSlash(filepath.Join(o.Name, d.Name))]; ok && i.PositionOverride {
				p, r, col = i.Page, i.Row, i.Col
			}
			if p != nil && *p >= 1 && *r >= 1 && *r <= o.Rows && *col >= 1 && *col <= o.Columns {
				if *p > o.Pages {
					return ImportPlan{}, errors.New("the page count is smaller than an existing image page")
				}
				d.Slot = ((*p-1)*o.Rows+*r-1)*o.Columns + *col - 1
				slots[d.Slot] = append(slots[d.Slot], d.Name)
			}
		}
		existing[strings.ToLower(d.Name)] = d
		disk = append(disk, d)
	}
	raw, _ := json.Marshal(struct {
		Options ImportOptions
		Files   []string
		Disk    []diskEntry
	}{o, review.Files, disk})
	digest := sha256.Sum256(raw)
	plan := ImportPlan{Entries: []ImportEntry{}, Fingerprint: hex.EncodeToString(digest[:])}
	reservedNames := map[string]bool{}
	reservedSlots := map[int]bool{}
	for i, name := range review.Files {
		if !imageExtensions[strings.ToLower(filepath.Ext(name))] {
			return plan, fmt.Errorf("unsupported image extension: %s", name)
		}
		reservedNames[strings.ToLower(importedName(name, start+i, o.album()))] = true
		if o.Mode == "paged" {
			reservedSlots[start+i] = true
		}
	}
	usedNames := map[string]bool{}
	for i, original := range review.Files {
		slot := start + i
		name := importedName(original, slot, o.album())
		baseName := strings.ToLower(name)
		entry := ImportEntry{Index: i, Original: original, Filename: name, Conflicts: []string{}, Replaces: []string{}, CanReplace: true}
		conflicts := map[string]bool{}
		if file, ok := existing[baseName]; ok {
			conflicts[file.Name] = true
			if !file.Regular {
				entry.CanReplace = false
			}
		}
		if o.Mode == "paged" {
			for _, name := range slots[slot] {
				conflicts[name] = true
				if !existing[strings.ToLower(name)].Regular {
					entry.CanReplace = false
				}
			}
		}
		if usedNames[baseName] {
			entry.Conflicts = append(entry.Conflicts, "Another selected photo has this filename")
			entry.CanReplace = false
		}
		for name := range conflicts {
			entry.Conflicts = append(entry.Conflicts, name)
		}
		slices.Sort(entry.Conflicts)
		if len(entry.Conflicts) > 0 {
			entry.Action = review.Actions[i]
			switch entry.Action {
			case "":
				plan.Unresolved++
			case "replace":
				if !entry.CanReplace {
					return plan, errors.New("a duplicate within this selection or a non-file destination must be renamed")
				}
				for name := range conflicts {
					entry.Replaces = append(entry.Replaces, name)
				}
				slices.Sort(entry.Replaces)
				if file, ok := existing[baseName]; ok {
					entry.Filename = file.Name
				}
			case "rename":
				if o.Mode == "box" {
					stem := strings.TrimSuffix(name, filepath.Ext(name))
					extension := filepath.Ext(name)
					for suffix := 2; ; suffix++ {
						candidate := fmt.Sprintf("%s (%d)%s", stem, suffix, extension)
						key := strings.ToLower(candidate)
						if _, ok := existing[key]; !ok && !usedNames[key] && !reservedNames[key] {
							entry.Filename = candidate
							break
						}
					}
				} else {
					slot = -1
					for candidate := 0; candidate < capacity; candidate++ {
						if len(slots[candidate]) == 0 && !reservedSlots[candidate] {
							slot = candidate
							break
						}
					}
					if slot < 0 {
						return plan, errors.New("no free positions for Rename; increase the page count or choose Replace")
					}
					reservedSlots[slot] = true
					entry.Filename = importedName(original, slot, o.album())
				}
			default:
				return plan, errors.New("choose Replace or Rename for each conflict")
			}
		}
		usedNames[baseName] = true
		usedNames[strings.ToLower(entry.Filename)] = true
		if o.Mode == "paged" {
			entry.Page, entry.Row, entry.Col = parsePosition(entry.Filename)
		}
		plan.Entries = append(plan.Entries, entry)
	}
	return plan, nil
}
func (c *Catalog) importPlanHTTP(w http.ResponseWriter, r *http.Request) {
	var review ImportReview
	if err := decode(w, r, &review); err != nil {
		badRequest(w, err)
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	plan, err := c.planImport(review)
	if err != nil {
		badRequest(w, err)
		return
	}
	writeJSON(w, 200, plan)
}
func (c *Catalog) importHTTP(w http.ResponseWriter, r *http.Request) {
	o := optionsFromRequest(r)
	if err := o.validate(); err != nil {
		badRequest(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		badRequest(w, errors.New("select image files to import"))
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	o.Name = c.canonicalAlbum(o.Name)
	root, destination, err := c.importLocation(o)
	if err != nil {
		badRequest(w, err)
		return
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		badRequest(w, err)
		return
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		badRequest(w, err)
		return
	}
	destination = filepath.Join(root, filepath.FromSlash(o.Name))
	staging, err := os.MkdirTemp(root, ".soft-inventory-import-")
	if err != nil {
		badRequest(w, err)
		return
	}
	// Backup files stay here until the whole catalog has been saved.
	safeToClean := true
	defer func() {
		if safeToClean {
			os.RemoveAll(staging)
		}
	}()
	review := ImportReview{Options: o, Actions: map[int]string{}}
	names := []string{}
	for {
		part, readErr := reader.NextPart()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			badRequest(w, fmt.Errorf("upload interrupted; nothing imported: %w", readErr))
			return
		}
		if part.FormName() == "review" && len(names) == 0 {
			if review.Fingerprint != "" {
				part.Close()
				badRequest(w, errors.New("duplicate import review"))
				return
			}
			raw, err := io.ReadAll(io.LimitReader(part, 1000001))
			part.Close()
			if err != nil || len(raw) > 1000000 {
				badRequest(w, errors.New("import review too large"))
				return
			}
			var supplied struct {
				Fingerprint string         `json:"fingerprint"`
				Actions     map[int]string `json:"actions"`
			}
			if err = json.Unmarshal(raw, &supplied); err != nil {
				badRequest(w, err)
				return
			}
			review.Fingerprint = supplied.Fingerprint
			review.Actions = supplied.Actions
			continue
		}
		if part.FormName() != "images" || part.FileName() == "" || len(names) >= maxImportFiles {
			part.Close()
			badRequest(w, errors.New("expected up to 10,000 image files"))
			return
		}
		original := part.FileName()
		if !imageExtensions[strings.ToLower(filepath.Ext(original))] {
			part.Close()
			badRequest(w, fmt.Errorf("unsupported image extension: %s", original))
			return
		}
		file, err := os.OpenFile(filepath.Join(staging, fmt.Sprintf("upload-%d", len(names))), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			part.Close()
			badRequest(w, err)
			return
		}
		size, copyErr := io.Copy(file, io.LimitReader(part, maxImageBytes+1))
		closeErr := file.Close()
		part.Close()
		if copyErr != nil {
			badRequest(w, copyErr)
			return
		}
		if closeErr != nil {
			badRequest(w, closeErr)
			return
		}
		if size == 0 || size > maxImageBytes {
			badRequest(w, fmt.Errorf("%s is empty or exceeds 50 MiB; nothing imported", original))
			return
		}
		names = append(names, original)
	}
	review.Files = names
	plan, err := c.planImport(review)
	if err != nil {
		badRequest(w, err)
		return
	}
	if plan.Unresolved > 0 {
		badRequest(w, errors.New("review conflicts and choose Replace or Rename before importing"))
		return
	}
	if review.Fingerprint != "" && review.Fingerprint != plan.Fingerprint {
		badRequest(w, errors.New("the folder changed since your preview; review conflicts again"))
		return
	}
	for _, entry := range plan.Entries {
		if entry.Action != "" && review.Fingerprint == "" {
			badRequest(w, errors.New("review conflicts before importing"))
			return
		}
	}
	created := false
	if _, err = os.Stat(destination); errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(destination, 0700); err != nil {
			badRequest(w, err)
			return
		}
		created = true
	}
	installed := []string{}
	backups := map[string]string{}
	detailTransfers := map[string]imageDetails{}
	rollback := func(cause error) {
		failures := []string{}
		for _, path := range installed {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err.Error())
			}
		}
		for original, backup := range backups {
			if err := os.Rename(backup, original); err != nil {
				failures = append(failures, err.Error())
			}
		}
		if created {
			os.Remove(destination)
		}
		if len(failures) > 0 {
			safeToClean = false
			badRequest(w, fmt.Errorf("%v; recovery files kept in %s: %s", cause, staging, strings.Join(failures, "; ")))
		} else {
			badRequest(w, cause)
		}
	}
	// Recheck the fingerprint immediately before changing any destination files.
	current, err := c.planImport(review)
	if err != nil || current.Fingerprint != plan.Fingerprint {
		if created {
			os.Remove(destination)
		}
		badRequest(w, errors.New("folder changed; review the import again"))
		return
	}
	for _, entry := range plan.Entries {
		descriptions := []string{}
		price := ""
		cardName := ""
		tags := []string{}
		for _, name := range entry.Replaces {
			original := filepath.Join(destination, name)
			if _, ok := backups[original]; ok {
				continue
			}
			backup := filepath.Join(staging, fmt.Sprintf("backup-%d", len(backups)))
			if err = os.Rename(original, backup); err != nil {
				rollback(err)
				return
			}
			backups[original] = backup
			key := filepath.ToSlash(filepath.Join(o.Name, name))
			if cardName == "" {
				cardName = c.data.Images[key].Name
			}
			for _, tag := range c.data.Images[key].Tags {
				if !slices.Contains(tags, tag) {
					tags = append(tags, tag)
				}
			}
			if value := c.data.Images[key].Price; price == "" && value != "" {
				price = value
			}
			if note := c.data.Images[key].Description; note != "" && !slices.Contains(descriptions, note) {
				descriptions = append(descriptions, note)
			}
		}
		path := filepath.Join(destination, entry.Filename)
		// O_EXCL avoids overwriting a file added by another program after the review.
		target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			rollback(err)
			return
		}
		installed = append(installed, path)
		source, err := os.Open(filepath.Join(staging, fmt.Sprintf("upload-%d", entry.Index)))
		if err != nil {
			target.Close()
			rollback(err)
			return
		}
		_, copyErr := io.Copy(target, source)
		source.Close()
		closeErr := target.Close()
		if copyErr != nil {
			rollback(copyErr)
			return
		}
		if closeErr != nil {
			rollback(closeErr)
			return
		}
		if len(descriptions) > 0 || price != "" || cardName != "" || len(tags) > 0 {
			detailTransfers[filepath.ToSlash(filepath.Join(o.Name, entry.Filename))] = imageDetails{Name: cardName, Tags: tags, Description: strings.Join(descriptions, "\n\n"), Price: price}
		}
	}
	if _, err = c.scanLocked(root, map[string]Album{o.Name: o.album()}, detailTransfers); err != nil {
		rollback(err)
		return
	}
	writeJSON(w, 200, map[string]any{"count": len(names), "album": o.Name, "root": root})
}
