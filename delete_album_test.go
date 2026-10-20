package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteAlbumArchivesNestedFoldersAndSurvivesReload(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	for _, key := range []string{"Album/a-1-1-1.jpg", "Album/Nested/b.jpg", "AlbumOther/c.jpg"} {
		writeImage(t, root, key)
	}
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if err := c.deleteAlbum("Album"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Album")); !os.IsNotExist(err) {
		t.Fatal("deleted folder remains", err)
	}
	archived, err := filepath.Glob(filepath.Join(root, ".card-atlas-trash", "deleted-*", "Album", "a-1-1-1.jpg"))
	if err != nil || len(archived) != 1 {
		t.Fatal("missing recovery copy", archived, err)
	}
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if result := c.listAlbums("", 1, true); result.Total != 1 || result.Folders[0].Path != "AlbumOther" {
		t.Fatal("deleted albums returned", result)
	}
	reopened, err := openCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.list(Filters{}).Total != 1 {
		t.Fatal("deletion not persisted")
	}
}

func TestDeleteAlbumFailureAndAccess(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Box/a.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if r := publicCall(c, "POST", "/api/delete-album", `{"folder":"Box"}`, nil); r.Code != 401 {
		t.Fatal("public delete allowed", r.Code)
	}
	for _, folder := range []string{".", "", "../outside", "missing"} {
		if err := c.deleteAlbum(folder); err == nil {
			t.Fatal("invalid deletion accepted", folder)
		}
	}
	c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
	if err := c.deleteAlbum("Box"); err == nil {
		t.Fatal("save failure hidden")
	}
	if _, err := os.Stat(filepath.Join(root, "Box", "a.jpg")); err != nil {
		t.Fatal("failed deletion lost files", err)
	}
	if c.list(Filters{Folder: "Box"}).Total != 1 {
		t.Fatal("failed deletion changed catalog")
	}
}
