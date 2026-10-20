package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Keep full-image decoding bounded even when several browser tabs scroll at once.
var thumbnailWorkers = make(chan struct{}, 2)

const thumbnailPixels = 25000000

func (c *Catalog) serveThumbnail(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("path")
	c.mu.RLock()
	root := c.data.Root
	item, ok := c.data.Images[key]
	c.mu.RUnlock()
	if root == "" || !ok || !item.Present {
		http.NotFound(w, r)
		return
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer directory.Close()
	file, err := directory.Open(filepath.FromSlash(key))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("v1:%s:%d:%d", key, info.Size(), info.ModTime().UnixNano())))
	tag := fmt.Sprintf(`"%x"`, digest)
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheDir := filepath.Join(root, ".card-atlas-cache", "thumbnails")
	cachePath := ""
	if err = os.MkdirAll(cacheDir, 0700); err == nil {
		if resolved, err := filepath.EvalSymlinks(cacheDir); err == nil && within(root, resolved) {
			cachePath = filepath.Join(resolved, fmt.Sprintf("%x.jpg", digest))
		}
	}
	serveCached := func() bool {
		if cachePath == "" {
			return false
		}
		raw, err := os.ReadFile(cachePath)
		if err != nil {
			return false
		}
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeContent(w, r, "thumbnail.jpg", info.ModTime(), bytes.NewReader(raw))
		return true
	}
	if serveCached() {
		return
	}
	select {
	case thumbnailWorkers <- struct{}{}:
		defer func() { <-thumbnailWorkers }()
	case <-r.Context().Done():
		return
	}
	if serveCached() {
		return
	}
	config, _, err := image.DecodeConfig(io.LimitReader(file, maxImageBytes))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > thumbnailPixels {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Preview unavailable; open the original image", http.StatusUnsupportedMediaType)
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "Preview unavailable", 415)
		return
	}
	source, _, err := image.Decode(io.LimitReader(file, maxImageBytes))
	if err != nil {
		http.Error(w, "Preview unavailable", 415)
		return
	}
	width, height := config.Width, config.Height
	if width > 320 || height > 320 {
		if width >= height {
			height = max(1, height*320/width)
			width = 320
		} else {
			width = max(1, width*320/height)
			height = 320
		}
	}
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			resized.Set(x, y, source.At(bounds.Min.X+x*config.Width/width, bounds.Min.Y+y*config.Height/height))
		}
	}
	var buffer bytes.Buffer
	if err = jpeg.Encode(&buffer, resized, &jpeg.Options{Quality: 78}); err != nil {
		http.Error(w, "Preview unavailable", 415)
		return
	}
	if cachePath != "" {
		if atomicCatalogWrite(cachePath, buffer.Bytes()) == nil {
			scheduleThumbnailPruning(cacheDir)
		}
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, "thumbnail.jpg", info.ModTime(), bytes.NewReader(buffer.Bytes()))
}
