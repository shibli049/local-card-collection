package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogRetentionDeletesOnlyOwnedFilesOlderThanFiveDays(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for _, file := range []struct {
		name   string
		age    time.Duration
		remove bool
	}{
		{"error-2026-10-14.log", 6 * 24 * time.Hour, true},
		{"debug-2026-10-14.log", 6 * 24 * time.Hour, true},
		{"debug-2026-10-16.log", 4 * 24 * time.Hour, false},
		{"error-2026-10-15.log", 5 * 24 * time.Hour, false},
		{"notes.log", 10 * 24 * time.Hour, false},
		{"debug-invalid.log", 10 * 24 * time.Hour, false},
		{"debug-2026-10-14.txt", 10 * 24 * time.Hour, false},
	} {
		path := filepath.Join(directory, file.name)
		if err := os.WriteFile(path, []byte("log"), 0600); err != nil {
			t.Fatal(err)
		}
		modified := now.Add(-file.age)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(directory, "debug-2026-10-01.log"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := pruneLogFiles(directory, now); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 6 {
		t.Fatalf("unexpected remaining files: %v", entries)
	}
	for _, name := range []string{"error-2026-10-14.log", "debug-2026-10-14.log"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatal("old log remains", name)
		}
	}
}

func TestConcurrentLogsAreValidAndPrunedOnFirstWrite(t *testing.T) {
	base := t.TempDir()
	directory, err := logDirectory(base)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(directory, "debug-2020-01-01.log")
	if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, time.Now().Add(-6*24*time.Hour), time.Now().Add(-6*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	logger := &fileLogger{}
	var workers sync.WaitGroup
	for i := 0; i < 40; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			if err := logger.write(base, "DEBUG", "concurrent", map[string]any{"number": i}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	workers.Wait()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("first write did not prune old logs")
	}
	files, err := filepath.Glob(filepath.Join(directory, "debug-*.log"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	file, err := os.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	seen := map[float64]bool{}
	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal("interleaved log entry", err)
		}
		seen[entry["number"].(float64)] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 40 {
		t.Fatal("log entries lost", len(seen))
	}
}

func TestRequestsLogDetailsWithoutCredentialsOrQueryStrings(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Box/a.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "http://localhost/api/login?password=do-not-log-query", strings.NewReader(`{"username":"admin","password":"do-not-log-body"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", sessionCookie+"=do-not-log-cookie")
	response := httptest.NewRecorder()
	c.handler().ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatal(response.Code)
	}
	files, err := filepath.Glob(filepath.Join(root, ".log", "*.log"))
	if err != nil || len(files) != 2 {
		t.Fatal("missing error/debug logs", files, err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, secret := range []string{"do-not-log-query", "do-not-log-body", "do-not-log-cookie"} {
			if strings.Contains(text, secret) {
				t.Fatal("sensitive request data logged")
			}
		}
		if !strings.Contains(text, "/api/login") || !strings.Contains(text, `"status":401`) {
			t.Fatal("request context missing", text)
		}
	}
	secretPath := filepath.Join(root, "private", "catalog.json")
	handler := c.logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badRequest(w, &os.PathError{Op: "open", Path: secretPath, Err: os.ErrPermission})
	}))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "http://localhost/api/description", nil))
	if strings.Contains(response.Body.String(), secretPath) || !strings.Contains(response.Body.String(), "permissions") {
		t.Fatal("technical filesystem error shown to user", response.Body.String())
	}
	files, _ = filepath.Glob(filepath.Join(root, ".log", "error-*.log"))
	raw, _ := os.ReadFile(files[0])
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if detail, ok := entry["error"].(string); ok && strings.Contains(detail, secretPath) {
			found = true
		}
	}
	if !found {
		t.Fatal("technical error was not retained in logs")
	}
}

func TestLoggingFailureDoesNotBreakRequests(t *testing.T) {
	c := newTestCatalog(t)
	if err := os.WriteFile(filepath.Join(c.bootstrap, ".log"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	response := publicCall(c, "GET", "/api/session", "", nil)
	if response.Code != 200 {
		t.Fatal("logging failure broke requests", response.Code)
	}
}

func TestPanicReturnsFriendlyErrorAndIsLogged(t *testing.T) {
	c := newTestCatalog(t)
	handler := c.logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("internal failure") }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "http://localhost/api/session", nil))
	if response.Code != 500 || strings.Contains(response.Body.String(), "internal failure") {
		t.Fatal("panic was exposed", response.Body.String())
	}
	files, _ := filepath.Glob(filepath.Join(c.bootstrap, ".log", "error-*.log"))
	if len(files) != 1 {
		t.Fatal("panic was not logged")
	}
	raw, err := os.ReadFile(files[0])
	if err != nil || !strings.Contains(string(raw), "internal failure") {
		t.Fatal("missing panic detail", err)
	}
}

func TestLogsExcludedFromScanAndImportFingerprint(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "plain.jpg")
	writeImage(t, root, ".log/hidden.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	if c.list(Filters{}).Total != 1 {
		t.Fatal("logs appeared as photos")
	}
	options := ImportOptions{Name: ".", Mode: "box"}
	first, err := c.planImport(ImportReview{Options: options, Files: []string{"new.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := applicationLogs.write(root, "ERROR", "test", map[string]any{"error": "test"}); err != nil {
		t.Fatal(err)
	}
	second, err := c.planImport(ImportReview{Options: options, Files: []string{"new.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("logging invalidated import review")
	}
}

func TestLoggerRejectsSymlinks(t *testing.T) {
	for _, target := range []string{"directory", "file"} {
		t.Run(target, func(t *testing.T) {
			base := t.TempDir()
			outside := t.TempDir()
			destination := filepath.Join(base, ".log")
			if target == "file" {
				var err error
				destination, err = logDirectory(base)
				if err != nil {
					t.Fatal(err)
				}
				destination = filepath.Join(destination, "debug-"+time.Now().Format("2006-01-02")+".log")
				outside = filepath.Join(outside, "outside.log")
				if err := os.WriteFile(outside, []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(outside, destination); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if err := (&fileLogger{}).write(base, "DEBUG", "test", nil); err == nil {
				t.Fatal("linked log target accepted")
			}
		})
	}
}

func TestUserErrorMessages(t *testing.T) {
	for _, scenario := range []struct {
		err  error
		want string
	}{
		{&os.PathError{Op: "open", Path: "/secret/catalog.json", Err: os.ErrPermission}, "permissions"},
		{&os.PathError{Op: "open", Path: "/secret/catalog.json", Err: os.ErrNotExist}, "no longer available"},
		{&http.MaxBytesError{Limit: 100}, "too large"},
		{fmt.Errorf("invalid request: %w", &json.SyntaxError{}), "Reload the page"},
		{errors.New("select between 1 and 1000 photos"), "Select between 1 and 1000 photos."},
		{errors.New("failure; recovery files kept in /secret/backup"), "files need recovery"},
		{errors.New("cannot read catalog; file left untouched"), "Keep the existing catalog file"},
	} {
		message := userError(scenario.err)
		if !strings.Contains(message, scenario.want) || strings.Contains(message, "/secret") {
			t.Fatalf("unhelpful message: %q", message)
		}
	}
}

func TestIdleCleanupPrunesBothSettingsAndCollectionLogs(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "Box/a.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	var oldFiles []string
	for _, base := range []string{c.bootstrap, root} {
		directory, err := logDirectory(base)
		if err != nil {
			t.Fatal(err)
		}
		old := filepath.Join(directory, "error-2020-01-01.log")
		if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
		modified := time.Now().Add(-6 * 24 * time.Hour)
		if err := os.Chtimes(old, modified, modified); err != nil {
			t.Fatal(err)
		}
		oldFiles = append(oldFiles, old)
	}
	stop := make(chan struct{})
	close(stop)
	c.maintainLogs(stop)
	for _, old := range oldFiles {
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Fatal("idle cleanup left an old log", old, err)
		}
	}
}
