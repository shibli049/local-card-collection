package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// deleteSelection validates the entire batch before archiving any files.
func (c *Catalog) deleteSelection(paths, folders []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(paths)+len(folders) == 0 || len(paths)+len(folders) > 1000 {
		return errors.New("select between 1 and 1000 photos or albums")
	}
	if len(paths) > 0 && len(folders) > 0 {
		return errors.New("select photos or albums separately")
	}
	valid := func(key string) bool {
		return key != "." && filepath.IsLocal(key) && filepath.ToSlash(filepath.Clean(key)) == key && !strings.HasPrefix(key, ".card-atlas-trash/") && key != ".card-atlas-trash"
	}
	seen := map[string]bool{}
	for _, key := range append(append([]string{}, paths...), folders...) {
		if !valid(key) || seen[key] {
			return errors.New("selection changed; reload and select again")
		}
		seen[key] = true
	}
	next := c.snapshot()
	sources := []string{}
	for _, key := range folders {
		found := false
		if _, ok := next.Albums[key]; ok {
			found = true
		}
		for _, item := range next.Images {
			if (item.Folder == key || strings.HasPrefix(item.Folder, key+"/")) && item.Present {
				found = true
			}
		}
		if !found {
			return errors.New("album or box no longer exists; reload the collection")
		}
		nested := false
		for _, parent := range folders {
			if parent != key && strings.HasPrefix(key, parent+"/") {
				nested = true
			}
		}
		if !nested {
			sources = append(sources, key)
		}
	}
	for _, key := range paths {
		item, ok := next.Images[key]
		if !ok || !item.Present {
			return errors.New("photo no longer exists; reload the collection")
		}
		if _, ok := next.Albums[item.Folder]; !ok {
			next.Albums[item.Folder] = inferredAlbum(c.data, item.Folder)
		}
		sources = append(sources, key)
		delete(next.Images, key)
	}
	for _, key := range folders {
		contains := func(s string) bool { return s == key || strings.HasPrefix(s, key+"/") }
		for k := range next.Albums {
			if contains(k) {
				delete(next.Albums, k)
			}
		}
		for k, item := range next.Images {
			if contains(item.Folder) {
				delete(next.Images, k)
			}
		}
	}
	for _, key := range sources {
		source := filepath.Join(c.data.Root, filepath.FromSlash(key))
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return err
		}
		if resolved != source || !within(c.data.Root, resolved) {
			return errors.New("cannot delete through symbolic links")
		}
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if (len(folders) > 0) != info.IsDir() || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("selection changed; reload and select again")
		}
	}
	trash := filepath.Join(c.data.Root, ".card-atlas-trash")
	if err := os.Mkdir(trash, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	resolved, err := filepath.EvalSymlinks(trash)
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
	type movedFile struct{ source, target string }
	moved := []movedFile{}
	rollback := func(cause error) error {
		failed := false
		for i := len(moved) - 1; i >= 0; i-- {
			m := moved[i]
			if err := os.Rename(m.target, m.source); err != nil {
				failed = true
				cause = errors.Join(cause, fmt.Errorf("recovery failed; files remain at %s: %w", m.target, err))
			}
		}
		if !failed {
			cause = errors.Join(cause, os.RemoveAll(container))
		}
		return cause
	}
	for i, key := range sources {
		source := filepath.Join(c.data.Root, filepath.FromSlash(key))
		target := filepath.Join(container, fmt.Sprintf("%d-%s", i, filepath.Base(source)))
		if err := os.Rename(source, target); err != nil {
			return rollback(err)
		}
		moved = append(moved, movedFile{source, target})
	}
	if err := c.commit(next); err != nil {
		return rollback(err)
	}
	return nil
}
