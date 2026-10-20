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

func TestDeleteAlbumRejectsSymlinkRecoveryFolder(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	outside := t.TempDir()
	writeImage(t, root, "Box/a.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".card-atlas-trash")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := c.deleteAlbum("Box"); err == nil {
		t.Fatal("symlink recovery destination accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "Box/a.jpg")); err != nil {
		t.Fatal("source file lost", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("deletion wrote outside the collection", err)
	}
	if c.list(Filters{Folder: "Box"}).Total != 1 {
		t.Fatal("rejected deletion changed catalog")
	}
}

func TestDeleteAlbumHTTPRequiresValidInputAndPreservesOtherAlbums(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Box/a.jpg")
	writeImage(t, root, "BoxSibling/b.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	login := publicCall(c, "POST", "/api/login", `{"username":"admin","password":"Inventory@2026"}`, nil)
	cookie := login.Result().Cookies()[0]
	for _, body := range []string{`{`, `{"folder":"."}`, `{"folder":"../Box"}`, `{"folder":"Box"} {}`} {
		if response := publicCall(c, "POST", "/api/delete-album", body, cookie); response.Code != 400 {
			t.Fatal("invalid delete accepted", body, response.Code)
		}
	}
	response := publicCall(c, "POST", "/api/delete-album", `{"folder":"Box"}`, cookie)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if c.list(Filters{}).Total != 1 || c.list(Filters{Folder: "BoxSibling"}).Total != 1 {
		t.Fatal("sibling album affected")
	}
	if response := publicCall(c, "POST", "/api/delete-album", `{"folder":"Box"}`, cookie); response.Code != 400 {
		t.Fatal("repeated deletion accepted")
	}
}
