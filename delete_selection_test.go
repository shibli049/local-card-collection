package main

import (
	"os"
	"path/filepath"
	"testing"
)

func deletionCatalog(t *testing.T) (*Catalog, string) {
	t.Helper()
	c := newTestCatalog(t)
	root := t.TempDir()
	for _, key := range []string{"Box/a.jpg", "Box/b.jpg", "Album/Nested/c.jpg", "Other/d.jpg"} {
		writeImage(t, root, key)
	}
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	return c, root
}
func TestDeleteSelectedPhotosPersistsAndRetainsAlbum(t *testing.T) {
	c, root := deletionCatalog(t)
	if err := c.deleteSelection([]string{"Box/a.jpg", "Box/b.jpg"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.data.Albums["Box"]; !ok {
		t.Fatal("empty box lost")
	}
	copies, _ := filepath.Glob(filepath.Join(root, ".card-atlas-trash", "deleted-*", "*.jpg"))
	if len(copies) != 2 {
		t.Fatal("recovery copies missing", copies)
	}
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if c.list(Filters{}).Total != 2 {
		t.Fatal("deleted photos returned on scan")
	}
	reopened, err := openCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.list(Filters{}).Total != 2 {
		t.Fatal("deletion not persistent")
	}
}
func TestDeleteSelectedAlbumsIncludesNestedAndHandlesOverlap(t *testing.T) {
	c, root := deletionCatalog(t)
	if err := c.deleteSelection(nil, []string{"Album", "Album/Nested", "Box"}); err != nil {
		t.Fatal(err)
	}
	if c.list(Filters{}).Total != 1 {
		t.Fatal("incorrect remaining photos")
	}
	for _, key := range []string{"Box", "Album"} {
		if _, err := os.Stat(filepath.Join(root, key)); !os.IsNotExist(err) {
			t.Fatal("folder remains", key)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "Other/d.jpg")); err != nil {
		t.Fatal(err)
	}
}
func TestDeleteSelectionRejectsInvalidBatchWithoutPartialChanges(t *testing.T) {
	c, root := deletionCatalog(t)
	for _, selection := range []struct{ paths, folders []string }{
		{nil, nil}, {[]string{"Box/a.jpg", "missing.jpg"}, nil}, {[]string{"Box/a.jpg", "Box/a.jpg"}, nil},
		{[]string{"../outside.jpg"}, nil}, {nil, []string{"."}}, {nil, []string{"Box", "missing"}},
		{[]string{"Box/a.jpg"}, []string{"Box"}},
	} {
		if err := c.deleteSelection(selection.paths, selection.folders); err == nil {
			t.Fatal("invalid selection accepted", selection)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "Box/a.jpg")); err != nil {
		t.Fatal(err)
	}
	if c.list(Filters{}).Total != 4 {
		t.Fatal("validation changed catalog")
	}
}
func TestDeleteSelectionRollsBackSaveFailure(t *testing.T) {
	for _, albums := range []bool{false, true} {
		t.Run(map[bool]string{false: "photos", true: "albums"}[albums], func(t *testing.T) {
			c, root := deletionCatalog(t)
			c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
			var err error
			if albums {
				err = c.deleteSelection(nil, []string{"Box", "Album"})
			} else {
				err = c.deleteSelection([]string{"Box/a.jpg", "Box/b.jpg"}, nil)
			}
			if err == nil {
				t.Fatal("save failure hidden")
			}
			for _, key := range []string{"Box/a.jpg", "Box/b.jpg", "Album/Nested/c.jpg"} {
				if _, err := os.Stat(filepath.Join(root, key)); err != nil {
					t.Fatal("file not restored", err)
				}
			}
			if c.list(Filters{}).Total != 4 {
				t.Fatal("failed deletion changed catalog")
			}
		})
	}
}
func TestDeleteSelectionRejectsSymlinks(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "source", true: "recovery"}[recovery], func(t *testing.T) {
			c, root := deletionCatalog(t)
			outside := t.TempDir()
			link := filepath.Join(root, ".card-atlas-trash")
			target := outside
			if !recovery {
				link = filepath.Join(root, "Box/a.jpg")
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				writeImage(t, outside, "a.jpg")
				target = filepath.Join(outside, "a.jpg")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skip(err)
			}
			if err := c.deleteSelection([]string{"Box/a.jpg", "Box/b.jpg"}, nil); err == nil {
				t.Fatal("symlink accepted")
			}
			if _, err := os.Stat(filepath.Join(root, "Box/b.jpg")); err != nil {
				t.Fatal("valid file moved before validation", err)
			}
		})
	}
}
func TestDeleteSelectionHTTPAuthorizationAndInput(t *testing.T) {
	c, _ := deletionCatalog(t)
	if r := publicCall(c, "POST", "/api/delete-selection", `{"paths":["Box/a.jpg"]}`, nil); r.Code != 401 {
		t.Fatal("public delete allowed", r.Code)
	}
	login := publicCall(c, "POST", "/api/login", `{"username":"`+adminUsername+`","password":"`+adminPassword+`"}`, nil)
	cookie := login.Result().Cookies()[0]
	for _, body := range []string{`{`, `{}`, `{"paths":["missing.jpg"]}`, `{"folders":["."]}`, `{"paths":["Box/a.jpg"]} {}`} {
		if r := publicCall(c, "POST", "/api/delete-selection", body, cookie); r.Code != 400 {
			t.Fatal("invalid request accepted", r.Code, body)
		}
	}
	if r := publicCall(c, "POST", "/api/delete-selection", `{"paths":["Box/a.jpg"]}`, cookie); r.Code != 200 {
		t.Fatal("admin deletion failed", r.Code, r.Body.String())
	}
	if c.list(Filters{}).Total != 3 {
		t.Fatal("wrong photo deleted")
	}
}
