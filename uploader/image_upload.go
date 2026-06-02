package uploader

import (
	"fmt"
	"strings"
	"time"
)

type imageHost struct {
	name   string
	upload func(string) (string, error)
}

// MultiImageUploader uploads thumbnails/sprites in parallel to all hosts:
// Freeimage → ImgBB → Catbox → Pixhost (NSFW fallback).
type MultiImageUploader struct {
	hosts []imageHost
}

// NewMultiImageUploader creates the default thumbnail upload chain.
func NewMultiImageUploader() *MultiImageUploader {
	freeimage := NewFreeimageUploader()
	imgbb := NewImgBBUploader()
	catbox := NewCatboxUploader()
	pixhost := NewThumbnailUploader("")

	hosts := []imageHost{
		{name: "Freeimage", upload: freeimage.Upload},
		{name: "ImgBB", upload: imgbb.Upload},
		{name: "Catbox", upload: catbox.Upload},
		{name: "Pixhost", upload: pixhost.Upload},
	}

	return &MultiImageUploader{hosts: hosts}
}

const (
	imageUploadRetries    = 2
	imageUploadBaseDelay  = 2 * time.Second
)

// Upload uploads to all hosts in parallel and returns the first success.
// Retries the entire batch up to imageUploadRetries times with backoff.
func (m *MultiImageUploader) Upload(filePath string) (url, host string, err error) {
	type result struct {
		url  string
		host string
		err  error
	}

	var lastErrors []string
	for attempt := 0; attempt <= imageUploadRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(imageUploadBaseDelay * time.Duration(1<<(attempt-1)))
		}

		ch := make(chan result, len(m.hosts))
		for _, h := range m.hosts {
			h := h
			go func() {
				u, e := h.upload(filePath)
				ch <- result{url: u, host: h.name, err: e}
			}()
		}

		var firstErr error
		for i := 0; i < len(m.hosts); i++ {
			res := <-ch
			if res.err == nil {
				return res.url, res.host, nil
			}
			lastErrors = append(lastErrors, fmt.Sprintf("%s: %v", res.host, res.err))
			if firstErr == nil {
				firstErr = res.err
			}
		}

		if firstErr == nil {
			return "", "", fmt.Errorf("no hosts configured")
		}
	}
	return "", "", fmt.Errorf("all image hosts failed after %d attempts: [%s]", imageUploadRetries+1, strings.Join(lastErrors, "; "))
}
