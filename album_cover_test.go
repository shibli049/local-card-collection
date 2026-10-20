package main

import "testing"

func TestAlbumCoverUsesFirstPresentPhotoInDisplayOrder(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Album/zebra-1-1-1.jpg")
	writeImage(t, root, "Album/apple-1-1-2.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	cover := c.listAlbums("", 1, false).Folders[0].Cover
	if cover == nil || cover.Path != "Album/zebra-1-1-1.jpg" || cover.Revision == "" {
		t.Fatalf("wrong cover: %+v", cover)
	}
	if err := c.arrange(cover.Path, "Album/apple-1-1-2.jpg", Slot{1, 1, 2}, Slot{1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if got := c.listAlbums("", 1, false).Folders[0].Cover.Path; got != "Album/apple-1-1-2.jpg" {
		t.Fatal("cover did not follow rearrangement", got)
	}
}
