package main

import (
	"errors"
	"fmt"
	"strconv"
)

type Slot struct {
	Page int `json:"page"`
	Row  int `json:"row"`
	Col  int `json:"col"`
}

func imageSlot(i Image) Slot {
	if i.Page == nil || i.Row == nil || i.Col == nil {
		return Slot{}
	}
	return Slot{*i.Page, *i.Row, *i.Col}
}
func setSlot(i Image, s Slot) Image {
	i.PositionOverride = true
	i.Order = 0
	if s.Page == 0 {
		i.Page, i.Row, i.Col = nil, nil, nil
	} else {
		i.Page, i.Row, i.Col = &s.Page, &s.Row, &s.Col
	}
	return i
}
func (c *Catalog) arrange(path, target string, destination Slot, expected Slot) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	source, ok := c.data.Images[path]
	if !ok || !source.Present {
		return errors.New("photo no longer exists; reload the album")
	}
	if imageSlot(source) != expected {
		return errors.New("photo moved since this view; reload and try again")
	}
	next := c.snapshot()
	album := inferredAlbum(next, source.Folder)
	if album.Mode == "box" || album.Mode == "" {
		other, ok := next.Images[target]
		if !ok || !other.Present || other.Folder != source.Folder || target == path {
			return errors.New("drop onto another photo in the same box")
		}
		// Explicit ranks also allow unpositioned boxes to retain an arbitrary order.
		paths := []string{}
		for k, i := range next.Images {
			if i.Present && i.Folder == source.Folder {
				paths = append(paths, k)
			}
		}
		sortPathsByOrder(paths, next.Images)
		for n, k := range paths {
			i := next.Images[k]
			i.Order = n + 1
			next.Images[k] = i
		}
		source = next.Images[path]
		other = next.Images[target]
		source.Order, other.Order = other.Order, source.Order
		next.Images[path], next.Images[target] = source, other
	} else {
		if destination.Page < 1 || destination.Page > album.Pages || destination.Row < 1 || destination.Row > album.Rows || destination.Col < 1 || destination.Col > album.Columns {
			return errors.New("choose a position inside this album")
		}
		occupants := []string{}
		for k, i := range next.Images {
			if i.Present && i.Folder == source.Folder && imageSlot(i) == destination && k != path {
				occupants = append(occupants, k)
			}
		}
		if len(occupants) > 1 {
			return errors.New("multiple photos occupy this position; move them to empty cells first")
		}
		actual := ""
		if len(occupants) == 1 {
			actual = occupants[0]
		}
		if actual != target {
			return errors.New("destination changed since this view; reload and try again")
		}
		if actual != "" {
			old := imageSlot(source)
			if old.Page > 0 {
				for k, i := range next.Images {
					if k != path && i.Present && i.Folder == source.Folder && imageSlot(i) == old {
						return errors.New("source position contains multiple photos; move this photo to an empty cell first")
					}
				}
			}
			next.Images[actual] = setSlot(next.Images[actual], imageSlot(source))
		}
		next.Albums[source.Folder] = album
		next.Images[path] = setSlot(source, destination)
	}
	return c.commit(next)
}
func inferredAlbum(data CatalogData, folder string) Album {
	if a, ok := data.Albums[folder]; ok {
		return a
	}
	a := Album{Mode: "box"}
	for _, i := range data.Images {
		if i.Present && i.Folder == folder {
			s := imageSlot(i)
			if s.Page > 0 && s.Page <= 1000 && s.Row > 0 && s.Row <= 100 && s.Col > 0 && s.Col <= 100 {
				a.Mode = "paged"
				a.Pages = max(a.Pages, s.Page)
				a.Rows = max(a.Rows, s.Row)
				a.Columns = max(a.Columns, s.Col)
			}
		}
	}
	return a
}
func (c *Catalog) setLayout(folder string, a Album) error {
	if a.Mode != "paged" || a.Pages < 1 || a.Pages > 1000 || a.Rows < 1 || a.Rows > 100 || a.Columns < 1 || a.Columns > 100 {
		return errors.New("use 1–1000 pages and 1–100 rows and columns")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if inferredAlbum(c.data, folder).Mode != "paged" {
		return errors.New("keep the existing album type; boxes do not have a page layout")
	}
	found := false
	for _, i := range c.data.Images {
		if i.Present && i.Folder == folder {
			found = true
			s := imageSlot(i)
			if s.Page > a.Pages || s.Row > a.Rows || s.Col > a.Columns {
				return errors.New("layout must include all existing photo positions")
			}
		}
	}
	if !found {
		return errors.New("album not found")
	}
	next := c.snapshot()
	next.Albums[folder] = a
	for k, i := range next.Images {
		if i.Folder == folder {
			next.Images[k] = setSlot(i, imageSlot(i))
		}
	}
	return c.commit(next)
}

// Physical slots are paginated independently of occupied cards, keeping even a
// completely empty page and very large sparse albums cheap to render.
func (c *Catalog) slotListing(filters Filters) (Listing, error) {
	index := c.catalogIndex()
	var album *Album
	for _, f := range index.folders {
		if f.Path == filters.Folder {
			album = f.Album
			break
		}
	}
	if album == nil || album.Mode != "paged" || album.Pages < 1 || album.Rows < 1 || album.Columns < 1 {
		return Listing{}, errors.New("choose a paged album")
	}
	first, last := 0, album.Pages*album.Rows*album.Columns
	if filters.Sheet != "" {
		p, err := strconv.Atoi(filters.Sheet)
		if err != nil || p < 1 || p > album.Pages {
			return Listing{}, fmt.Errorf("invalid album page")
		}
		first = (p - 1) * album.Rows * album.Columns
		last = first + album.Rows*album.Columns
	}
	result := Listing{Mode: "slots", AlbumCount: len(index.folders), Folders: index.folders, Items: []Image{}, Pages: []int{}, Slots: []Slot{}, Root: index.root, Count: len(index.position), Total: len(index.byFolder[filters.Folder])}
	for p := 1; p <= album.Pages; p++ {
		result.Pages = append(result.Pages, p)
	}
	result.PageCount = max(1, (last-first+59)/60)
	result.Page = max(1, min(filters.Page, result.PageCount))
	start := first + (result.Page-1)*60
	end := min(start+60, last)
	for n := start; n < end; n++ {
		result.Slots = append(result.Slots, Slot{n/(album.Rows*album.Columns) + 1, n/album.Columns%album.Rows + 1, n%album.Columns + 1})
	}
	for _, entry := range index.byFolder[filters.Folder] {
		i := entry.image
		s := imageSlot(i)
		if s.Page == 0 {
			result.Unpositioned++
			continue
		}
		n := ((s.Page-1)*album.Rows+s.Row-1)*album.Columns + s.Col - 1
		if n >= start && n < end {
			result.Items = append(result.Items, i)
		}
	}
	return result, nil
}
