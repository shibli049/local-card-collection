package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
)

// Transfer keeps filenames and metadata, assigning free positions in paged albums.
func (c *Catalog) transfer(paths []string, destination string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(paths) == 0 || len(paths) > 1000 {
		return errors.New("select between 1 and 1000 photos")
	}
	found := false
	for _, item := range c.data.Images {
		if item.Present && item.Folder == destination {
			found = true
			break
		}
	}
	if _, ok := c.data.Albums[destination]; ok {
		found = true
	}
	if !found {
		return errors.New("destination album or box no longer exists")
	}
	album := inferredAlbum(c.data, destination)
	if album.Mode == "" {
		album.Mode = "box"
	}
	next := c.snapshot()
	occupied := map[Slot]bool{}
	for _, item := range next.Images {
		if item.Present && item.Folder == destination {
			occupied[imageSlot(item)] = true
		}
	}
	type move struct{ from, to string }
	moves := []move{}
	seen := map[string]bool{}
	position := 0
	for _, key := range paths {
		item, ok := next.Images[key]
		if seen[key] || !ok || !item.Present {
			return errors.New("selection changed; reload and select the photos again")
		}
		seen[key] = true
		if item.Folder == destination {
			return errors.New("choose a different destination from every selected photo")
		}
		if _, exists := next.Albums[item.Folder]; !exists {
			sourceAlbum := inferredAlbum(c.data, item.Folder)
			if sourceAlbum.Mode == "" {
				sourceAlbum.Mode = "box"
			}
			next.Albums[item.Folder] = sourceAlbum
		}
		target := path.Join(destination, item.Filename)
		if existing, ok := next.Images[target]; ok && existing.Present {
			return fmt.Errorf("destination already contains %s; no photos moved", item.Filename)
		}
		slot := Slot{}
		if album.Mode == "paged" {
			capacity := album.Pages * album.Rows * album.Columns
			for position < capacity {
				slot = Slot{position/(album.Rows*album.Columns) + 1, position/album.Columns%album.Rows + 1, position%album.Columns + 1}
				position++
				if !occupied[slot] {
					break
				}
				slot = Slot{}
			}
			if slot.Page == 0 {
				return errors.New("not enough empty cells in the destination; increase its layout first")
			}
			occupied[slot] = true
		}
		item = setSlot(item, slot)
		item.Path, item.Folder = target, destination
		delete(next.Images, key)
		next.Images[target] = item
		moves = append(moves, move{key, target})
	}
	next.Albums[destination] = album
	root, err := os.OpenRoot(c.data.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	// Exclusive creation prevents overwriting files, including files not yet scanned.
	copyFile := func(from, to string) error {
		source, err := root.Open(from)
		if err != nil {
			return err
		}
		defer source.Close()
		info, err := source.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("selected photo is not a regular file")
		}
		output, err := root.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, source)
		syncErr := output.Sync()
		closeErr := output.Close()
		if err = errors.Join(copyErr, syncErr, closeErr); err != nil {
			return errors.Join(err, root.Remove(to))
		}
		return nil
	}
	created, removed := 0, 0
	rollback := func(cause error) error {
		for i := 0; i < removed; i++ {
			if err := copyFile(moves[i].to, moves[i].from); err != nil {
				// Keep the destination copy if restoring the source fails.
				cause = errors.Join(cause, fmt.Errorf("restore %s: %w; destination copy retained", moves[i].from, err))
				continue
			}
			cause = errors.Join(cause, root.Remove(moves[i].to))
		}
		for i := removed; i < created; i++ {
			cause = errors.Join(cause, root.Remove(moves[i].to))
		}
		return cause
	}
	for _, m := range moves {
		if err := copyFile(m.from, m.to); err != nil {
			return rollback(fmt.Errorf("cannot move %s: %w", m.from, err))
		}
		created++
	}
	for _, m := range moves {
		if err := root.Remove(m.from); err != nil {
			return rollback(err)
		}
		removed++
	}
	if err := c.commit(next); err != nil {
		return rollback(err)
	}
	return nil
}
