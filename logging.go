package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const logRetention = 5 * 24 * time.Hour

type fileLogger struct {
	mu          sync.Mutex
	lastCleanup map[string]time.Time
}

var applicationLogs = &fileLogger{}

func logDirectory(base string) (string, error) {
	if base == "" {
		return "", errors.New("no folder is available for logs")
	}
	directory := filepath.Join(base, ".log")
	if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New(".log must be a regular directory, not a symbolic link")
	}
	return directory, nil
}

// Only files created by this logger are eligible for retention cleanup.
func pruneLogFiles(directory string, now time.Time) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range entries {
		name := entry.Name()
		date := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(name, "error-"), "debug-"), ".log")
		if (!strings.HasPrefix(name, "error-") && !strings.HasPrefix(name, "debug-")) || !strings.HasSuffix(name, ".log") {
			continue
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if info.ModTime().Before(now.Add(-logRetention)) {
			if err := os.Remove(filepath.Join(directory, name)); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func (l *fileLogger) cleanupLocked(directory string, now time.Time) error {
	if l.lastCleanup == nil {
		l.lastCleanup = map[string]time.Time{}
	}
	if last, ok := l.lastCleanup[directory]; ok && now.Sub(last) < time.Hour {
		return nil
	}
	if err := pruneLogFiles(directory, now); err != nil {
		return err
	}
	l.lastCleanup[directory] = now
	return nil
}

func (l *fileLogger) write(base, level, event string, fields map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	directory, err := logDirectory(base)
	if err != nil {
		return err
	}
	cleanupErr := l.cleanupLocked(directory, now)
	entry := map[string]any{"time": now.Format(time.RFC3339Nano), "level": level, "event": event}
	for key, value := range fields {
		entry[key] = value
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return errors.Join(err, cleanupErr)
	}
	filePath := filepath.Join(directory, strings.ToLower(level)+"-"+now.Format("2006-01-02")+".log")
	if info, err := os.Lstat(filePath); err == nil && !info.Mode().IsRegular() {
		return errors.New("log file must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Join(err, cleanupErr)
	}
	_, writeErr := file.Write(append(raw, '\n'))
	return errors.Join(writeErr, file.Close(), cleanupErr)
}

func (c *Catalog) logBase() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data.Root != "" {
		return c.data.Root
	}
	if c.bootstrap != "" {
		return c.bootstrap
	}
	if c.file != "" {
		return filepath.Dir(c.file)
	}
	return ""
}

func logEvent(base, level, event string, fields map[string]any) {
	if base == "" {
		return
	}
	if err := applicationLogs.write(base, level, event, fields); err != nil {
		fmt.Fprintln(os.Stderr, "Card Atlas could not write its log:", err)
	}
}

type loggedResponse struct {
	http.ResponseWriter
	status int
	detail error
}

func (w *loggedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *loggedResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *loggedResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	if err != nil && w.detail == nil {
		w.detail = err
	}
	return n, err
}
func recordError(w http.ResponseWriter, err error) {
	if logged, ok := w.(*loggedResponse); ok && logged.detail == nil {
		logged.detail = err
	}
}

func (c *Catalog) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		response := &loggedResponse{ResponseWriter: w}
		defer func() {
			failure := recover()
			var stack string
			if failure != nil {
				stack = string(debug.Stack())
				response.detail = fmt.Errorf("request panic: %v", failure)
				if response.status == 0 {
					writeJSON(response, 500, map[string]string{"error": "Something went wrong. Please try again. If it happens again, check the .log folder."})
				}
			}
			if response.status == 0 {
				response.status = http.StatusOK
			}
			fields := map[string]any{"method": r.Method, "path": r.URL.Path, "status": response.status, "duration_ms": time.Since(start).Milliseconds()}
			base := c.logBase()
			logEvent(base, "DEBUG", "request", fields)
			if response.status >= 400 || response.detail != nil {
				if stack != "" {
					fields["stack"] = stack
				}
				if response.detail != nil {
					fields["error"] = response.detail.Error()
				}
				logEvent(base, "ERROR", "request_failed", fields)
			}
		}()
		next.ServeHTTP(response, r)
	})
}

// Also prune while the app is idle, rather than waiting for the next request.
func (c *Catalog) maintainLogs(stop <-chan struct{}) {
	cleanup := func() {
		bases := []string{c.logBase()}
		if c.bootstrap != "" && c.bootstrap != bases[0] {
			bases = append(bases, c.bootstrap)
		}
		for _, base := range bases {
			if base == "" {
				continue
			}
			applicationLogs.mu.Lock()
			directory, err := logDirectory(base)
			if err == nil {
				err = applicationLogs.cleanupLocked(directory, time.Now())
			}
			applicationLogs.mu.Unlock()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Card Atlas could not clean old logs:", err)
			}
		}
	}
	cleanup()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
