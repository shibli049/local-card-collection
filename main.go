package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	// Wait reaps the short-lived launcher; browser lifetime is independent.
	return command.Run()
}
func run() (runErr error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	data := flag.String("data", filepath.Join(home, ".soft-inventory"), "Local catalog directory")
	port := flag.Int("port", 8765, "Local server port (0 selects an available port)")
	noBrowser := flag.Bool("no-browser", false, "Do not open the browser automatically")
	folder := flag.String("folder", "", "Image folder to scan, including subfolders")
	flag.Parse()
	directory, err := expandPath(*data)
	if err != nil {
		return err
	}
	var catalog *Catalog
	defer func() {
		if runErr != nil {
			base := directory
			if catalog != nil {
				base = catalog.logBase()
			}
			logEvent(base, "ERROR", "startup_failed", map[string]any{"error": runErr.Error()})
		}
	}()
	catalog, err = openCatalog(directory)
	if err != nil {
		return err
	}
	if *folder != "" {
		if _, err = catalog.scan(*folder); err != nil {
			return err
		}
	}
	if *folder == "" && catalog.data.Root != "" && !catalog.data.Portable {
		if _, err = catalog.scan(""); err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil && *port == 8765 {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return err
	}
	address := fmt.Sprintf("http://localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	logEvent(catalog.logBase(), "DEBUG", "server_started", map[string]any{"address": address})
	stopLogs := make(chan struct{})
	defer close(stopLogs)
	go catalog.maintainLogs(stopLogs)
	fmt.Printf("Card Atlas: %s\nKeep this window open. Press Ctrl+C to stop.\n", address)
	if catalog.data.Root != "" {
		fmt.Printf("Catalog: %s\n", catalog.file)
	} else {
		fmt.Println("Choose an image folder in the app to store its catalog.")
	}
	if !*noBrowser {
		go func() {
			if err := openBrowser(address); err != nil {
				logEvent(catalog.logBase(), "ERROR", "browser_open_failed", map[string]any{"error": err.Error()})
				fmt.Printf("Open %s in your browser.\n", address)
			}
		}()
	}
	server := &http.Server{Handler: catalog.handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	return server.Serve(listener)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Could not start Card Atlas:", userError(err))
		if runtime.GOOS == "windows" {
			fmt.Println("Press Enter to close.")
			fmt.Scanln()
		}
		os.Exit(1)
	}
}
