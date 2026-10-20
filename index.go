package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

type indexedImage struct {
	image    Image
	search   string
	filename string
}
type catalogIndex struct {
	root         string
	position     []*indexedImage
	name         []*indexedImage
	byFolder     map[string][]*indexedImage
	byFolderName map[string][]*indexedImage
	folders      []FolderSummary
}

func compareImagePosition(a, b *indexedImage) int {
	if cmp := strings.Compare(a.image.Folder, b.image.Folder); cmp != 0 {
		return cmp
	}
	if a.image.Order != 0 || b.image.Order != 0 {
		if n := cmp.Compare(a.image.Order, b.image.Order); n != 0 {
			return n
		}
	}
	for _, pair := range [][2]*int{{a.image.Page, b.image.Page}, {a.image.Row, b.image.Row}, {a.image.Col, b.image.Col}} {
		if cmp := compareNumber(pair[0], pair[1]); cmp != 0 {
			return cmp
		}
	}
	return strings.Compare(a.image.Path, b.image.Path)
}

// Immutable indexes are rebuilt once after a catalog mutation. Queries retain a
// snapshot and do not hold the catalog lock while filtering or writing responses.
func (c *Catalog) catalogIndex() *catalogIndex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index != nil {
		return c.index
	}
	index := &catalogIndex{root: c.data.Root, byFolder: map[string][]*indexedImage{}, byFolderName: map[string][]*indexedImage{}, folders: []FolderSummary{}}
	for _, item := range c.data.Images {
		if !item.Present {
			continue
		}
		text := item.Path + " " + item.Name + " " + item.Description + " " + strings.Join(item.Tags, " ")
		for _, v := range []*int{item.Page, item.Row, item.Col} {
			if v != nil {
				text += " " + strconv.Itoa(*v)
			}
		}
		index.position = append(index.position, &indexedImage{image: item, search: strings.ToLower(text), filename: strings.ToLower(item.Filename)})
	}
	slices.SortFunc(index.position, compareImagePosition)
	index.name = slices.Clone(index.position)
	slices.SortFunc(index.name, func(a, b *indexedImage) int {
		if cmp := strings.Compare(a.filename, b.filename); cmp != 0 {
			return cmp
		}
		return compareImagePosition(a, b)
	})
	for _, item := range index.position {
		index.byFolder[item.image.Folder] = append(index.byFolder[item.image.Folder], item)
	}
	for _, item := range index.name {
		index.byFolderName[item.image.Folder] = append(index.byFolderName[item.image.Folder], item)
	}
	inferred := CatalogData{Albums: make(map[string]Album, len(c.data.Albums))}
	for k, a := range c.data.Albums {
		inferred.Albums[k] = a
	}
	for folder, items := range index.byFolder {
		if _, ok := inferred.Albums[folder]; !ok {
			a := Album{Mode: "box"}
			for _, entry := range items {
				s := imageSlot(entry.image)
				if s.Page > 0 && s.Page <= 1000 && s.Row > 0 && s.Row <= 100 && s.Col > 0 && s.Col <= 100 {
					a.Mode = "paged"
					a.Pages = max(a.Pages, s.Page)
					a.Rows = max(a.Rows, s.Row)
					a.Columns = max(a.Columns, s.Col)
				}
			}
			if a.Mode != "" {
				inferred.Albums[folder] = a
			}
		}
	}
	for folder, items := range index.byFolder {
		index.folders = append(index.folders, FolderSummary{Path: folder, Count: len(items), Album: albumPointer(inferred.Albums, folder), Cover: &AlbumCover{Path: items[0].image.Path, Filename: items[0].image.Filename, Revision: items[0].image.Revision}})
	}
	slices.SortFunc(index.folders, func(a, b FolderSummary) int { return strings.Compare(a.Path, b.Path) })
	c.index = index
	return index
}
func (c *Catalog) listAlbums(query string, page int, all bool) Listing {
	index := c.catalogIndex()
	folders := []FolderSummary{}
	terms := strings.Fields(strings.ToLower(query))
	for _, folder := range index.folders {
		matched := true
		for _, term := range terms {
			if !strings.Contains(strings.ToLower(folder.Path), term) {
				matched = false
				break
			}
		}
		if matched {
			folders = append(folders, folder)
		}
	}
	total := len(folders)
	pages := max(1, (total+49)/50)
	page = max(1, min(page, pages))
	if !all {
		start := (page - 1) * 50
		folders = folders[start:min(start+50, total)]
	}
	return Listing{Mode: "albums", AlbumCount: len(index.folders), Items: []Image{}, Folders: folders, Pages: []int{}, Total: total, Count: len(index.position), Page: page, PageCount: pages, Root: index.root}
}

func sortPathsByOrder(paths []string, images map[string]Image) {
	slices.SortFunc(paths, func(a, b string) int {
		return compareImagePosition(&indexedImage{image: images[a]}, &indexedImage{image: images[b]})
	})
}
