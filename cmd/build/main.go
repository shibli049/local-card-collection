// Build standalone macOS and Windows executables from any Go development host.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	if err := build(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func build() error {
	if err := os.MkdirAll("dist", 0755); err != nil {
		return err
	}
	for _, target := range []struct{ os, arch, label, binary string }{
		{"darwin", "arm64", "macOS-AppleSilicon", "CardAtlas"},
		{"darwin", "amd64", "macOS-Intel", "CardAtlas"},
		{"windows", "amd64", "Windows-x64", "CardAtlas.exe"},
		{"windows", "arm64", "Windows-ARM64", "CardAtlas.exe"},
	} {
		directory := filepath.Join("dist", target.label)
		if err := os.MkdirAll(directory, 0755); err != nil {
			return err
		}
		binary := filepath.Join(directory, target.binary)
		command := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binary, ".")
		command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.os, "GOARCH="+target.arch)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		fmt.Println("Building", target.label)
		if err := command.Run(); err != nil {
			return err
		}
		if target.os == runtime.GOOS && target.arch == runtime.GOARCH {
			alias := filepath.Join("dist", target.binary)
			if err := copyExecutable(binary, alias); err != nil {
				return err
			}
		}
		zipFile, err := os.Create(filepath.Join("dist", "CardAtlas-"+target.label+".zip"))
		if err != nil {
			return err
		}
		writer := zip.NewWriter(zipFile)
		for _, source := range []string{binary, "README.md"} {
			if err := addFile(writer, source); err != nil {
				writer.Close()
				zipFile.Close()
				return err
			}
		}
		if err := writer.Close(); err != nil {
			zipFile.Close()
			return err
		}
		if err := zipFile.Close(); err != nil {
			return err
		}
	}
	fmt.Println("Executables and ZIP downloads are ready in dist/.")
	return nil
}

// Replace the executable with a new file. Overwriting an existing Mach-O in
// place can leave macOS using stale code-signature state and killing it at launch.
func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".card-atlas-executable-*")
	if err != nil {
		return err
	}
	temporary := output.Name()
	defer os.Remove(temporary)
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Chmod(0755); err != nil {
		output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, destination)
}
func addFile(writer *zip.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.Base(path)
	header.Method = zip.Deflate
	destination, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(destination, file)
	return err
}
