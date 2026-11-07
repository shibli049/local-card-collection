package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnitZipContainsStandaloneFiles(t *testing.T) {
	directory := t.TempDir()
	files := map[string][]byte{"CardAtlas": []byte("standalone executable"), "README.md": []byte("Run the app without installing a runtime.")}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range files {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, data, 0755); err != nil {
			t.Fatal(err)
		}
		if err := addFile(writer, path); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != len(files) {
		t.Fatal("missing files in download")
	}
	for _, file := range archive.File {
		expected, exists := files[file.Name]
		if !exists {
			t.Fatalf("archive exposes a source path or unexpected file: %s", file.Name)
		}
		source, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(source)
		source.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("download has incorrect content for %s", file.Name)
		}
		if runtime.GOOS != "windows" && file.Name == "CardAtlas" && file.Mode().Perm()&0111 == 0 {
			t.Fatal("macOS download lost executable permission")
		}
	}
}
func TestUnitZipRejectsMissingFile(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err := addFile(writer, filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing input should fail: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutableCopyReplacesFileInsteadOfOverwritingIt(t *testing.T) {
	directory := t.TempDir()
	source, destination := filepath.Join(directory, "source"), filepath.Join(directory, "CardAtlas")
	if err := os.WriteFile(source, []byte("new executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old executable"), 0600); err != nil {
		t.Fatal(err)
	}
	// Stat through an open handle: on Windows, os.Stat defers reading the file
	// identity until SameFile, which would then see the replacement file.
	file, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	before, err := file.Stat()
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(source, destination); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("executable was overwritten in place")
	}
	raw, err := os.ReadFile(destination)
	if err != nil || string(raw) != "new executable" {
		t.Fatal("incorrect installed executable", err)
	}
	if runtime.GOOS != "windows" && after.Mode().Perm() != 0755 {
		t.Fatal("executable permission missing")
	}
	files, err := filepath.Glob(filepath.Join(directory, ".card-atlas-executable-*"))
	if err != nil || len(files) != 0 {
		t.Fatal("temporary executable left behind", err)
	}
}

func TestFailedExecutableCopyKeepsExistingFile(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "CardAtlas")
	if err := os.WriteFile(destination, []byte("working executable"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(filepath.Join(directory, "missing"), destination); !os.IsNotExist(err) {
		t.Fatal("missing build output accepted", err)
	}
	raw, err := os.ReadFile(destination)
	if err != nil || string(raw) != "working executable" {
		t.Fatal("failed copy damaged previous executable", err)
	}
}
