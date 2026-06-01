package uploader

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/teacat/chaturbate-dvr/server"
)

type imageHost struct {
	name   string
	upload func(string) (string, error)
}

// MultiImageUploader uploads thumbnails/sprites with durable fallbacks:
// Pixhost (NSFW API) → Catbox (permanent) → Freeimage → GitHub (if configured).
type MultiImageUploader struct {
	hosts []imageHost
}

// NewMultiImageUploader creates the default thumbnail upload chain.
func NewMultiImageUploader() *MultiImageUploader {
	pixhost := NewThumbnailUploader("")
	catbox := NewCatboxUploader()
	freeimage := NewFreeimageUploader()

	hosts := []imageHost{
		{name: "Pixhost", upload: pixhost.Upload},
		{name: "Catbox", upload: catbox.Upload},
		{name: "Freeimage", upload: freeimage.Upload},
	}

	// Add GitHub as last-resort fallback if configured
	githubToken := os.Getenv("GITHUB_TOKEN")
	githubRepo := os.Getenv("GITHUB_REPO")
	githubBranch := os.Getenv("GITHUB_BRANCH")
	githubPreviewPath := os.Getenv("GITHUB_PREVIEW_PATH")

	if server.Config != nil {
		if server.Config.GitHubToken != "" {
			githubToken = server.Config.GitHubToken
		}
		if server.Config.GitHubRepo != "" {
			githubRepo = server.Config.GitHubRepo
		}
		if server.Config.GitHubBranch != "" {
			githubBranch = server.Config.GitHubBranch
		}
		if server.Config.GitHubPreviewPath != "" {
			githubPreviewPath = server.Config.GitHubPreviewPath
		}
	}

	if githubToken != "" && githubRepo != "" {
		github := NewGitHubUploader(githubToken, githubRepo, githubBranch, githubPreviewPath)
		hosts = append(hosts, imageHost{name: "GitHub", upload: github.Upload})
	}

	return &MultiImageUploader{hosts: hosts}
}

const (
	imageUploadRetries    = 2
	imageUploadBaseDelay  = 2 * time.Second
)

// Upload tries each host in order until one succeeds.
// Retries the entire fallback chain up to imageUploadRetries times.
func (m *MultiImageUploader) Upload(filePath string) (url, host string, err error) {
	var lastErrors []string
	for attempt := 0; attempt <= imageUploadRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(imageUploadBaseDelay * time.Duration(1<<(attempt-1)))
		}
		for _, h := range m.hosts {
			var upErr error
			url, upErr = h.upload(filePath)
			if upErr == nil {
				return url, h.name, nil
			}
			lastErrors = append(lastErrors, fmt.Sprintf("%s: %v", h.name, upErr))
		}
	}
	return "", "", fmt.Errorf("all image hosts failed after %d attempts: [%s]", imageUploadRetries+1, strings.Join(lastErrors, "; "))
}
