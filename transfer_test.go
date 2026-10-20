package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransferBetweenAlbumTypes(t *testing.T) {
	for _, sourceMode := range []string{"box", "paged"} {
		for _, destinationMode := range []string{"box", "paged"} {
			t.Run(sourceMode+"_to_"+destinationMode, func(t *testing.T) {
				c := newTestCatalog(t)
				root := t.TempDir()
				for _, key := range []string{"Source/a.jpg", "Source/b.jpg", "Destination/existing-1-1-1.jpg"} {
					writeImage(t, root, key)
				}
				if _, err := c.scan(root); err != nil {
					t.Fatal(err)
				}
				c.data.Albums["Source"] = Album{sourceMode, 1, 2, 2}
				c.data.Albums["Destination"] = Album{destinationMode, 1, 2, 2}
				item := c.data.Images["Source/a.jpg"]
				item.Name, item.Description, item.Price, item.Tags = "Saved name", "Saved description", "20 SEK", []string{"Rare"}
				c.data.Images[item.Path] = item
				if err := c.transfer([]string{"Source/a.jpg", "Source/b.jpg"}, "Destination"); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"a.jpg", "b.jpg"} {
					if _, err := os.Stat(filepath.Join(root, "Source", name)); !os.IsNotExist(err) {
						t.Fatal("source file remains", err)
					}
					if _, err := os.Stat(filepath.Join(root, "Destination", name)); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := c.scan(root); err != nil {
					t.Fatal(err)
				}
				moved := c.data.Images["Destination/a.jpg"]
				if moved.Name != item.Name || moved.Description != item.Description || moved.Price != item.Price || len(moved.Tags) != 1 || moved.Tags[0] != "Rare" {
					t.Fatal("metadata lost", moved)
				}
				if destinationMode == "paged" {
					if imageSlot(moved) != (Slot{1, 1, 2}) || imageSlot(c.data.Images["Destination/b.jpg"]) != (Slot{1, 2, 1}) {
						t.Fatal("wrong free slots")
					}
				} else if imageSlot(moved) != (Slot{}) {
					t.Fatal("box retained positions")
				}
				if c.data.Albums["Source"].Mode != sourceMode || c.data.Albums["Destination"].Mode != destinationMode {
					t.Fatal("album type changed")
				}
			})
		}
	}
}

func TestTransferFailuresPreserveFilesAndCatalog(t *testing.T) {
	for _, failure := range []string{"duplicate", "full", "same", "missing", "save", "unscanned collision"} {
		t.Run(failure, func(t *testing.T) {
			c := newTestCatalog(t)
			root := t.TempDir()
			for _, key := range []string{"Source/a.jpg", "Source/b.jpg", "Destination/existing-1-1-1.jpg"} {
				writeImage(t, root, key)
			}
			if _, err := c.scan(root); err != nil {
				t.Fatal(err)
			}
			c.data.Albums["Destination"] = Album{"paged", 1, 2, 2}
			paths := []string{"Source/a.jpg", "Source/b.jpg"}
			destination := "Destination"
			switch failure {
			case "duplicate":
				paths = append(paths, paths[0])
			case "full":
				c.data.Albums["Destination"] = Album{"paged", 1, 1, 1}
			case "same":
				destination = "Source"
			case "missing":
				paths = append(paths, "Source/gone.jpg")
			case "save":
				c.file = filepath.Join(t.TempDir(), "missing", "catalog.json")
			case "unscanned collision":
				writeImage(t, root, "Destination/b.jpg")
			}
			if err := c.transfer(paths, destination); err == nil {
				t.Fatal("invalid transfer accepted")
			}
			for _, key := range []string{"Source/a.jpg", "Source/b.jpg"} {
				if _, err := os.Stat(filepath.Join(root, key)); err != nil {
					t.Fatal("source lost", err)
				}
				if !c.data.Images[key].Present {
					t.Fatal("catalog changed")
				}
			}
			if _, err := os.Stat(filepath.Join(root, "Destination/a.jpg")); !os.IsNotExist(err) {
				t.Fatal("partial move left behind", err)
			}
		})
	}
}

func TestTransferRequiresAdministrator(t *testing.T) {
	c := newTestCatalog(t)
	if response := publicCall(c, "POST", "/api/transfer", `{"paths":["Source/a.jpg"],"destination":"Destination"}`, nil); response.Code != 401 {
		t.Fatal("public transfer allowed", response.Code)
	}
}
