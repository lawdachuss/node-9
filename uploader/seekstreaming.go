package uploader

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const seekStreamingChunkSize = 50 * 1024 * 1024

type SeekStreamingUploader struct {
	key           string
	client        *http.Client
	mu            sync.Mutex
	lastPosterURL string
	lastPreviewURL string
}

func NewSeekStreamingUploader(key string) *SeekStreamingUploader {
	return &SeekStreamingUploader{
		key: key,
		client: &http.Client{
			Timeout: 120 * time.Minute,
			Transport: &http.Transport{
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       90 * time.Second,
				DisableCompression:    true,
				TLSHandshakeTimeout:   30 * time.Second,
				ResponseHeaderTimeout: 120 * time.Second,
				DisableKeepAlives:     true,
				DialContext:           (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
			},
		},
	}
}

func (u *SeekStreamingUploader) LastPosterURL() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastPosterURL
}

func (u *SeekStreamingUploader) LastPreviewURL() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastPreviewURL
}

type seekStreamingUploadEndpointResp struct {
	TusURL      string `json:"tusUrl"`
	AccessToken string `json:"accessToken"`
}

type seekStreamingManageVideo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Poster   string `json:"poster"`
	Preview  string `json:"preview"`
	AssetURL string `json:"assetUrl"`
}

type seekStreamingManageListResp struct {
	Data []seekStreamingManageVideo `json:"data"`
}

func (u *SeekStreamingUploader) Upload(filePath string) (string, error) {
	return u.UploadWithProgress(filePath, nil)
}

func isUploadPayloadTooLarge(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "status 413") ||
		strings.Contains(err.Error(), "413 Payload Too Large")
}

func (u *SeekStreamingUploader) UploadWithProgress(filePath string, progress ProgressFunc) (string, error) {
	if u.key == "" {
		return "", fmt.Errorf("SeekStreaming API key not configured")
	}

	filename := filepath.Base(filePath)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			time.Sleep(uploadBackoff(attempt-2, lastErr))
		}

		ep, err := u.getUploadEndpoint()
		if err != nil {
			lastErr = fmt.Errorf("get upload endpoint: %w", err)
			if isUploadRateLimited(err) {
				time.Sleep(uploadBackoff(attempt, err))
				lastErr = nil
				continue
			}
			if attempt < 3 {
				continue
			}
			return "", lastErr
		}
		fmt.Printf("[seekstreaming] got upload endpoint — tusUrl=%s accessToken=%s\n",
			maskSensitiveURL(ep.TusURL), maskSensitive(ep.AccessToken))

		uploadURL, err := u.createTUSUpload(ep, filePath)
		if err != nil {
			lastErr = fmt.Errorf("create tus upload: %w", err)
			if isUploadRateLimited(err) {
				time.Sleep(uploadBackoff(attempt, err))
				lastErr = nil
				continue
			}
			if isUploadPayloadTooLarge(err) {
				return "", lastErr
			}
			if attempt < 3 {
				continue
			}
			return "", lastErr
		}

		fmt.Printf("[seekstreaming] tus upload created — Location: %s\n", maskSensitiveURL(uploadURL))

		_, err = u.uploadFileTUS(uploadURL, filePath, progress)
		if err != nil {
			lastErr = fmt.Errorf("upload file: %w", err)
			if isUploadRateLimited(err) {
				time.Sleep(uploadBackoff(attempt, err))
				lastErr = nil
				continue
			}
			if isUploadPayloadTooLarge(err) {
				return "", lastErr
			}
			if attempt < 3 {
				continue
			}
			return "", lastErr
		}

		fmt.Printf("[seekstreaming] upload complete — polling manage API for %s\n", filename)

		embedURL, posterURL, previewURL := u.pollForVideo(filename)
		if embedURL != "" {
			u.mu.Lock()
			u.lastPosterURL = posterURL
			u.lastPreviewURL = previewURL
			u.mu.Unlock()
			fmt.Printf("[seekstreaming] embed URL: %s\n", embedURL)
			if posterURL != "" {
				fmt.Printf("[seekstreaming] poster URL: %s\n", posterURL)
			}
			if previewURL != "" {
				fmt.Printf("[seekstreaming] preview URL: %s\n", previewURL)
			}
			return embedURL, nil
		}

		// Fallback: build embed URL from the TUS upload UUID
		tusParts := strings.Split(strings.TrimRight(uploadURL, "/"), "/")
		tusID := tusParts[len(tusParts)-1]
		embedURL = fmt.Sprintf("https://chuglii.seeks.cloud/#%s", tusID)
		fmt.Printf("[seekstreaming] manage API did not return video yet — falling back to TUS embed: %s\n", embedURL)
		return embedURL, nil
	}
	return "", lastErr
}

// pollForVideo polls the manage list API until the uploaded video appears
// by searching for its filename. Returns (embedURL, posterURL, previewURL).
// Returns empty embedURL if the video was not found within the timeout.
func (u *SeekStreamingUploader) pollForVideo(filename string) (string, string, string) {
	maxAttempts := 12
	delay := 5 * time.Second

	for i := 0; i < maxAttempts; i++ {
		if i > 0 {
			time.Sleep(delay)
		}

		v, err := u.searchVideoByName(filename)
		if err != nil {
			fmt.Printf("[seekstreaming] search attempt %d/%d failed: %v\n", i+1, maxAttempts, err)
			continue
		}
		if v == nil {
			fmt.Printf("[seekstreaming] search attempt %d/%d — video not found yet\n", i+1, maxAttempts)
			continue
		}

		embedURL := fmt.Sprintf("https://chuglii.seeks.cloud/#%s", v.ID)

		var posterURL string
		if v.Poster != "" && v.AssetURL != "" {
			posterURL = v.AssetURL + v.Poster
		}

		var previewURL string
		if v.Preview != "" && v.AssetURL != "" {
			previewURL = v.AssetURL + v.Preview
		}

		return embedURL, posterURL, previewURL
	}

	return "", "", ""
}

func (u *SeekStreamingUploader) searchVideoByName(filename string) (*seekStreamingManageVideo, error) {
	reqURL := fmt.Sprintf("https://seekstreaming.com/api/v1/video/manage?search=%s&perPage=5", url.QueryEscape(filename))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", u.key)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var listResp seekStreamingManageListResp
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	for _, v := range listResp.Data {
		if v.Name == filename {
			return &v, nil
		}
	}

	return nil, nil
}

func (u *SeekStreamingUploader) getUploadEndpoint() (*seekStreamingUploadEndpointResp, error) {
	req, err := http.NewRequest("GET", "https://seekstreaming.com/api/v1/video/upload", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", u.key)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status 429: rate limit — %s", strings.TrimSpace(string(body)))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var ep seekStreamingUploadEndpointResp
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if ep.TusURL == "" || ep.AccessToken == "" {
		return nil, fmt.Errorf("empty tus URL or access token in response")
	}

	return &ep, nil
}

func (u *SeekStreamingUploader) createTUSUpload(ep *seekStreamingUploadEndpointResp, filePath string) (string, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	filename := filepath.Base(filePath)
	filetype := mimeTypeByExt(filepath.Ext(filename))

	b64 := func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	}

	metadata := fmt.Sprintf("accessToken %s,filename %s,filetype %s", b64(ep.AccessToken), b64(filename), b64(filetype))

	req, err := http.NewRequest("POST", ep.TusURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", fmt.Sprintf("%d", fi.Size()))
	req.Header.Set("Upload-Metadata", metadata)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := u.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tus create: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("tus create status %d: %s", resp.StatusCode, string(body))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("missing Location header in tus create response")
	}

	return location, nil
}

func (u *SeekStreamingUploader) uploadFileTUS(uploadURL, filePath string, progress ProgressFunc) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, _ := os.Stat(filePath)
	fileSize := fi.Size()

	offset, err := u.getTUSOffset(uploadURL)
	if err != nil {
		return "", fmt.Errorf("get offset: %w", err)
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek to offset %d: %w", offset, err)
		}
	}

	buf := make([]byte, seekStreamingChunkSize)
	for offset < fileSize {
		chunkSize := int64(seekStreamingChunkSize)
		if remaining := fileSize - offset; remaining < chunkSize {
			chunkSize = remaining
		}

		n, err := io.ReadFull(f, buf[:chunkSize])
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return "", fmt.Errorf("read chunk at offset %d: %w", offset, err)
		}
		if int64(n) == 0 {
			break
		}

		chunkBody := bytes.NewReader(buf[:n])
		req, err := http.NewRequest("PATCH", uploadURL, chunkBody)
		if err != nil {
			return "", fmt.Errorf("create patch request: %w", err)
		}
		req.Header.Set("Tus-Resumable", "1.0.0")
		req.Header.Set("Content-Type", "application/offset+octet-stream")
		req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
		req.ContentLength = int64(n)
		req.Header.Set("User-Agent", defaultUserAgent)

		resp, err := u.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("tus upload chunk at offset %d: %w", offset, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("tus upload status %d at offset %d: %s", resp.StatusCode, offset, string(body))
		}

		newOffset := resp.Header.Get("Upload-Offset")
		if newOffset != "" {
			offset, err = strconv.ParseInt(newOffset, 10, 64)
			if err != nil {
				return "", fmt.Errorf("parse upload-offset header: %w", err)
			}
		} else {
			offset += int64(n)
		}

		if progress != nil {
			progress("SeekStreaming", offset, fileSize)
		}
	}

	parts := strings.Split(strings.TrimRight(uploadURL, "/"), "/")
	return parts[len(parts)-1], nil
}

func (u *SeekStreamingUploader) getTUSOffset(uploadURL string) (int64, error) {
	req, err := http.NewRequest("HEAD", uploadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create head request: %w", err)
	}
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := u.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("head request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return 0, nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return 0, nil
	}

	offsetStr := resp.Header.Get("Upload-Offset")
	if offsetStr == "" {
		return 0, nil
	}

	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		return 0, nil
	}
	return offset, nil
}

func ExtractSeekStreamingVideoID(embedURL string) string {
	if idx := strings.LastIndex(embedURL, "#"); idx >= 0 {
		return embedURL[idx+1:]
	}
	return ""
}

// GetSeekStreamingMediaURLs fetches both the poster and preview URLs for a SeekStreaming video
// in a single API call.
func GetSeekStreamingMediaURLs(key, videoID string) (posterURL, previewURL string, err error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://seekstreaming.com/api/v1/video/manage/%s", videoID), nil)
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", key)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	_body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d — %s", resp.StatusCode, strings.TrimSpace(string(_body)))
	}

	var detail struct {
		Poster   string `json:"poster"`
		Preview  string `json:"preview"`
		AssetURL string `json:"assetUrl"`
	}
	if err := json.Unmarshal(_body, &detail); err != nil {
		return "", "", fmt.Errorf("decode: %w (body: %s)", err, string(_body))
	}

	if detail.Poster != "" && detail.AssetURL != "" {
		posterURL = detail.AssetURL + detail.Poster
	}
	if detail.Preview != "" && detail.AssetURL != "" {
		previewURL = detail.AssetURL + detail.Preview
	}
	return posterURL, previewURL, nil
}

// GetSeekStreamingPosterURL fetches the poster URL for a SeekStreaming video.
// Also returns the preview URL if available.
func GetSeekStreamingPosterURL(key, videoID string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://seekstreaming.com/api/v1/video/manage/%s", videoID), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", key)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	_body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d — %s", resp.StatusCode, strings.TrimSpace(string(_body)))
	}

	var detail struct {
		Poster   string `json:"poster"`
		Preview  string `json:"preview"`
		AssetURL string `json:"assetUrl"`
	}
	if err := json.Unmarshal(_body, &detail); err != nil {
		return "", fmt.Errorf("decode: %w (body: %s)", err, string(_body))
	}
	if detail.Poster == "" || detail.AssetURL == "" {
		return "", fmt.Errorf("no poster available — response: %s", string(_body))
	}
	return detail.AssetURL + detail.Poster, nil
}

// GetSeekStreamingPreviewURL fetches the preview URL for a SeekStreaming video.
func GetSeekStreamingPreviewURL(key, videoID string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://seekstreaming.com/api/v1/video/manage/%s", videoID), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", key)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	_body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d — %s", resp.StatusCode, strings.TrimSpace(string(_body)))
	}

	var detail struct {
		Preview  string `json:"preview"`
		AssetURL string `json:"assetUrl"`
	}
	if err := json.Unmarshal(_body, &detail); err != nil {
		return "", fmt.Errorf("decode: %w (body: %s)", err, string(_body))
	}
	if detail.Preview == "" || detail.AssetURL == "" {
		return "", fmt.Errorf("no preview available — response: %s", string(_body))
	}
	return detail.AssetURL + detail.Preview, nil
}

func maskSensitive(s string) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) < 12 {
		return "<masked>"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func maskSensitiveURL(rawURL string) string {
	if rawURL == "" {
		return "<empty>"
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}
	return u.Scheme + "://" + u.Host + "/..."
}

func mimeTypeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".flv":
		return "video/x-flv"
	case ".ts":
		return "video/mp2t"
	case ".m4v":
		return "video/x-m4v"
	default:
		return "video/mp4"
	}
}
