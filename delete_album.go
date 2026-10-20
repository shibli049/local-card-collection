package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Deleted folders are retained outside the scanned collection for recovery.
func (c *Catalog) deleteAlbum(folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if folder == "" || folder == "." || !filepath.IsLocal(folder) || filepath.ToSlash(filepath.Clean(folder)) != folder {
		return errors.New("choose an album or box inside the collection; the collection root cannot be deleted")
	}
	contains := func(candidate string) bool { return candidate == folder || strings.HasPrefix(candidate, folder+"/") }
	next := c.snapshot()
	found := false
	for key := range next.Albums {
		if contains(key) {
			found = true
			delete(next.Albums, key)
		}
	}
	for key, item := range next.Images {
		if contains(item.Folder) {
			if item.Present {
				found = true
			}
			delete(next.Images, key)
		}
	}
	if !found {
		return errors.New("album or box no longer exists; reload the collection")
	}
	source := filepath.Join(c.data.Root, filepath.FromSlash(folder))
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	if resolved != source || !within(c.data.Root, resolved) {
		return errors.New("cannot delete albums through symbolic links")
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("album folder is not a directory")
	}
	trash := filepath.Join(c.data.Root, ".card-atlas-trash")
	if err := os.Mkdir(trash, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	resolved, err = filepath.EvalSymlinks(trash)
	if err != nil {
		return err
	}
	if resolved != trash {
		return errors.New("recovery folder cannot be a symbolic link")
	}
	container, err := os.MkdirTemp(trash, "deleted-")
	if err != nil {
		return err
	}
	target := filepath.Join(container, filepath.Base(source))
	if err := os.Rename(source, target); err != nil {
		return errors.Join(err, os.Remove(container))
	}
	if err := c.commit(next); err != nil {
		if restoreErr := os.Rename(target, source); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("album recovery failed; files remain at %s: %w", target, restoreErr))
		}
		return errors.Join(err, os.Remove(container))
	}
	return nil
}
