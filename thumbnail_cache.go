package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

var generatedThumbnails atomic.Uint64
var thumbnailPruning sync.Mutex

func scheduleThumbnailPruning(directory string) {
	if generatedThumbnails.Add(1)%128 != 0 {
		return
	}
	go func() {
		if !thumbnailPruning.TryLock() {
			return
		}
		defer thumbnailPruning.Unlock()
		pruneThumbnailCache(directory, 256<<20, 192<<20)
	}()
}
func pruneThumbnailCache(directory string, limit, target int64) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	type cached struct {
		name           string
		size, modified int64
	}
	files := []cached{}
	var total int64
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jpg") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		total += info.Size()
		files = append(files, cached{entry.Name(), info.Size(), info.ModTime().UnixNano()})
	}
	if total <= limit {
		return
	}
	slices.SortFunc(files, func(a, b cached) int {
		if a.modified < b.modified {
			return -1
		}
		if a.modified > b.modified {
			return 1
		}
		return strings.Compare(a.name, b.name)
	})
	for _, file := range files {
		if total <= target {
			break
		}
		if os.Remove(filepath.Join(directory, file.name)) == nil {
			total -= file.size
		}
	}
}
