package uploader

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const upnshareChunkSize = 50 * 1024 * 1024

type UPnShareUploader struct {
	keys           *keyRing
	client         *http.Client
	mu             sync.Mutex
	lastPosterURL  string
	lastPreviewURL string
}

func NewUPnShareUploader(apiKeys []string) *UPnShareUploader {
	return &UPnShareUploader{
		keys: sharedKeyRing(apiKeys),
		client: &http.Client{
			Timeout: uploadClientTimeout,
			Transport: newUploadTransport(true),
		},
	}
}

func (u *UPnShareUploader) LastPosterURL() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastPosterURL
}

func (u *UPnShareUploader) LastPreviewURL() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastPreviewURL
}

func (u *UPnShareUploader) Upload(filePath string) (string, error) {
	return u.UploadWithProgress(filePath, nil)
}

func (u *UPnShareUploader) Keys() *keyRing { return u.keys }

func (u *UPnShareUploader) UploadWithProgress(filePath string, progress ProgressFunc) (string, error) {
	if u.keys.count() == 0 {
		return "", fmt.Errorf("UPnShare API key not configured")
	}

	attempts := u.keys.count()
	maxRetriesPerKey := 2
	var lastErr error

	for ki := 0; ki < attempts; ki++ {
		key := u.keys.take()
		for retry := 1; retry <= maxRetriesPerKey; retry++ {
			if retry > 1 {
				time.Sleep(uploadBackoff(retry-2, lastErr))
			}

		embedURL, posterURL, previewURL, err := u.uploadWithKey(filePath, progress, key)
		if err != nil {
			lastErr = fmt.Errorf("upload file: %w", err)
			if IsPermanentError(err) {

				break
			}
			if isUploadRateLimited(err) {
				time.Sleep(uploadBackoff(retry, err))
				lastErr = nil
				continue
			}
			if retry < maxRetriesPerKey {
				continue
			}

			break
		}

		u.mu.Lock()
		u.lastPosterURL = posterURL
		u.lastPreviewURL = previewURL
		u.mu.Unlock()

		return embedURL, nil
	}
}

return "", lastErr
}

type upnshareUploadEndpointResp struct {
	TusURL      string `json:"tusUrl"`
	AccessToken string `json:"accessToken"`
}

type upnshareManageVideo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Poster   string `json:"poster"`
	Preview  string `json:"preview"`
	AssetURL string `json:"assetUrl"`
}

type upnshareManageListResp struct {
	Data []upnshareManageVideo `json:"data"`
}

type upnsharePlayer struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	Status string `json:"status"`
}

type upnsharePlayerListResp struct {
	Data []upnsharePlayer `json:"data"`
}

func (u *UPnShareUploader) ensurePlayer(apiKey string) error {
	listResp, err := u.listPlayers(apiKey)
	if err != nil {
		return fmt.Errorf("list players: %w", err)
	}

	for _, p := range listResp.Data {
		if p.Status == "Active" {
			return nil
		}
	}

	prefix := randomUpnsharePrefix()
	domain := fmt.Sprintf("%s.upns.online", prefix)

	if err := u.createPlayer(apiKey, domain); err != nil {
		return fmt.Errorf("create player: %w", err)
	}

	fmt.Printf("[upnshare] created player with domain %s\n", domain)
	return nil
}

func randomUpnsharePrefix() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 3)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		b[i] = letters[n.Int64()]
	}
	return string(b)
}

func (u *UPnShareUploader) getPlayerDomain(apiKey string) (string, error) {
	listResp, err := u.listPlayers(apiKey)
	if err != nil {
		return "", err
	}
	for _, p := range listResp.Data {
		if p.Status == "Active" {
			return p.Domain, nil
		}
	}
	if len(listResp.Data) > 0 {
		return listResp.Data[0].Domain, nil
	}
	return "", fmt.Errorf("no players found for this account")
}

func (u *UPnShareUploader) listPlayers(apiKey string) (*upnsharePlayerListResp, error) {
	req, err := http.NewRequest("GET", "https://upnshare.com/api/v1/video/player", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", apiKey)
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

	var listResp upnsharePlayerListResp
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &listResp, nil
}

func (u *UPnShareUploader) createPlayer(apiKey, domain string) error {
	body := fmt.Sprintf(`{"domain":"%s"}`, domain)
	req, err := http.NewRequest("POST", "https://upnshare.com/api/v1/video/player", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (u *UPnShareUploader) uploadWithKey(filePath string, progress ProgressFunc, apiKey string) (string, string, string, error) {
	if err := u.ensurePlayer(apiKey); err != nil {
		fmt.Printf("[upnshare] warning: could not ensure player — %v\n", err)
	}

	uploadURL, err := u.createUpload(filePath, apiKey)
	if err != nil {
		return "", "", "", fmt.Errorf("create upload: %w", err)
	}

	filename := filepath.Base(filePath)

	if err := u.uploadTUSRaw(uploadURL, filePath, progress); err != nil {
		return "", "", "", fmt.Errorf("tus upload: %w", err)
	}

	fmt.Printf("[upnshare] upload complete — polling manage API for %s\n", filename)

	video := u.pollForVideo(filename, apiKey)
	videoID := video.ID
	if videoID == "" {
		parts := strings.Split(strings.TrimRight(uploadURL, "/"), "/")
		videoID = parts[len(parts)-1]
		fmt.Printf("[upnshare] manage API did not return video yet — falling back to TUS UUID: %s\n", videoID)
	}

	var posterURL, previewURL string
	if video.AssetURL != "" {
		if video.Poster != "" {
			posterURL = video.AssetURL + video.Poster
			fmt.Printf("[upnshare] poster URL: %s\n", posterURL)
		}
		if video.Preview != "" {
			previewURL = video.AssetURL + video.Preview
			fmt.Printf("[upnshare] preview URL: %s\n", previewURL)
		}
	}

	playerDomain, err := u.getPlayerDomain(apiKey)
	if err != nil {
		return "", "", "", fmt.Errorf("get player domain: %w", err)
	}

	embedURL := fmt.Sprintf("https://%s/%s%s", playerDomain, "#", videoID)
	fmt.Printf("[upnshare] embed URL: %s\n", embedURL)
	return embedURL, posterURL, previewURL, nil
}

func (u *UPnShareUploader) pollForVideo(filename, apiKey string) upnshareManageVideo {
	maxAttempts := 12
	delay := 5 * time.Second

	for i := 0; i < maxAttempts; i++ {
		if i > 0 {
			time.Sleep(delay)
		}

		v, err := u.searchVideoByName(filename, apiKey)
		if err != nil {
			fmt.Printf("[upnshare] search attempt %d/%d failed: %v\n", i+1, maxAttempts, err)
			continue
		}
		if v == nil {
			fmt.Printf("[upnshare] search attempt %d/%d — video not found yet\n", i+1, maxAttempts)
			continue
		}

		return *v
	}

	return upnshareManageVideo{}
}

// ExtractUPnShareVideoID returns the video ID from a UPnShare embed URL of the
// form https://<domain>/#<id>.
func ExtractUPnShareVideoID(embedURL string) string {
	if idx := strings.LastIndex(embedURL, "#"); idx >= 0 {
		return embedURL[idx+1:]
	}
	return ""
}

// GetUPnShareMediaURLs fetches the poster and preview URLs for a UPnShare video.
//
// The embed URL stores UPnShare's *player* ID, which is NOT the same as the
// manage-API video id, so a direct by-ID lookup 404s. The reliable way to find
// the video is to search the manage list by the recording's username and then
// pick the entry whose name contains this recording's exact timestamp token
// (the unique YYYY-MM-DD_HH-MM-SS portion). We fall back to a prefix/ID match
// only if the token isn't present.
func GetUPnShareMediaURLs(apiKey, videoID, filename string) (posterURL, previewURL string, err error) {
	fetchByID := func() (*upnshareManageVideo, error) {
		if videoID == "" {
			return nil, fmt.Errorf("empty video ID")
		}
		req, e := http.NewRequest("GET", fmt.Sprintf("https://upnshare.com/api/v1/video/manage/%s", videoID), nil)
		if e != nil {
			return nil, fmt.Errorf("create request: %w", e)
		}
		req.Header.Set("api-token", apiKey)
		req.Header.Set("User-Agent", defaultUserAgent)

		resp, e := mediaFetchClient.Do(req)
		if e != nil {
			return nil, fmt.Errorf("request: %w", e)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status %d", resp.StatusCode)
		}
		var detail upnshareManageVideo
		if e := json.NewDecoder(resp.Body).Decode(&detail); e != nil {
			return nil, fmt.Errorf("decode: %w", e)
		}
		if detail.ID == "" {
			return nil, fmt.Errorf("empty response")
		}
		return &detail, nil
	}

	// Fast path: the by-ID endpoint occasionally works (e.g. when the player ID
	// happens to equal the manage id).
	if detail, e := fetchByID(); e == nil {
		p, pr := buildUPnShareURLs(detail)
		return p, pr, nil
	}

	// Otherwise resolve via the username search + timestamp-token match.
	if filename != "" {
		user := upnshareUsername(filename)
		token := upnshareTimestampToken(filename)
		if list, se := searchUPnShareByName(user, apiKey); se == nil {
			if v := matchUPnShareVideo(list, filename, token, videoID); v != nil {
				p, pr := buildUPnShareURLs(v)
				return p, pr, nil
			}
		}
	}

	return "", "", fmt.Errorf("UPnShare media not available yet for %s", videoID)
}

// buildUPnShareURLs assembles the absolute poster/preview URLs from a manage
// video record.
func buildUPnShareURLs(v *upnshareManageVideo) (posterURL, previewURL string) {
	if v == nil || v.AssetURL == "" {
		return "", ""
	}
	if v.Poster != "" {
		posterURL = v.AssetURL + v.Poster
	}
	if v.Preview != "" {
		previewURL = v.AssetURL + v.Preview
	}
	return posterURL, previewURL
}

// matchUPnShareVideo picks the best manage-list match for our recording.
// Priority: exact name > name contains the timestamp token > name starts with
// the recording stem (suffix-stripped) > matches the stored video ID.
func matchUPnShareVideo(list []upnshareManageVideo, filename, token, videoID string) *upnshareManageVideo {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	stem = strings.TrimPrefix(stem, "merged-")

	var fallback *upnshareManageVideo
	for i := range list {
		v := &list[i]
		switch {
		case v.Name == filename:
			return v
		case token != "" && strings.Contains(v.Name, token):
			return v
		case strings.HasPrefix(v.Name, stem):
			if fallback == nil {
				fallback = v
			}
		case v.ID == videoID && fallback == nil:
			fallback = v
		}
	}
	return fallback
}

// upnshareUsername extracts the chaturbate username from a recording filename
// of the form "username_YYYY-MM-DD_HH-MM-SS[.suffix].mp4". It mirrors the
// parsing in channel/channel_file.go so the watcher and the recorder agree on
// where the username ends (handling usernames that contain "_20" or hyphens).
func upnshareUsername(filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	stem := strings.TrimPrefix(base, "merged-")
	idx := upnshareDateRe.FindStringSubmatchIndex(stem)
	if idx == nil {
		return ""
	}
	candidate := stem[:idx[0]] // index of the leading "_" before the date

	// Deduplicate merged "<user>-<user>" usernames.
	searchFrom := 0
	for {
		hyphen := strings.Index(candidate[searchFrom:], "-")
		if hyphen < 0 {
			break
		}
		hyphen += searchFrom
		if candidate[:hyphen] == candidate[hyphen+1:] {
			return candidate[:hyphen]
		}
		searchFrom = hyphen + 1
	}
	return candidate
}

// upnshareTimestampToken returns the unique "YYYY-MM-DD_HH-MM-SS" portion of a
// recording filename, used to pinpoint the exact video among a user's list.
func upnshareTimestampToken(filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	m := upnshareDateTokenRe.FindString(base)
	return m
}

var (
	// Matches the leading "_" of the "_YYYY-MM-DD_" timestamp separator.
	upnshareDateRe = regexp.MustCompile(`_(20\d{2}-\d{2}-\d{2})[_-]`)
	// Matches the full "YYYY-MM-DD_HH-MM-SS" timestamp token.
	upnshareDateTokenRe = regexp.MustCompile(`20\d{2}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}`)
)

func searchUPnShareByName(filename, apiKey string) ([]upnshareManageVideo, error) {
	reqURL := fmt.Sprintf("https://upnshare.com/api/v1/video/manage?search=%s&perPage=10", url.QueryEscape(filename))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", apiKey)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := mediaFetchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var listResp upnshareManageListResp
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return listResp.Data, nil
}

func (u *UPnShareUploader) searchVideoByName(filename, apiKey string) (*upnshareManageVideo, error) {
	reqURL := fmt.Sprintf("https://upnshare.com/api/v1/video/manage?search=%s&perPage=5", url.QueryEscape(filename))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", apiKey)
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

	var listResp upnshareManageListResp
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

func (u *UPnShareUploader) createUpload(filePath, apiKey string) (string, error) {
	uploadURL, accessToken, err := u.getUploadEndpoint(apiKey)
	if err != nil {
		return "", err
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	filename := filepath.Base(filePath)
	filetype := mimeTypeByExt(filepath.Ext(filename))

	b64 := func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	}

	metadata := fmt.Sprintf("accessToken %s,filename %s,filetype %s", b64(accessToken), b64(filename), b64(filetype))

	tusReq, err := http.NewRequest("POST", uploadURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create tus request: %w", err)
	}
	tusReq.Header.Set("Tus-Resumable", "1.0.0")
	tusReq.Header.Set("Upload-Length", fmt.Sprintf("%d", fi.Size()))
	tusReq.Header.Set("Upload-Metadata", metadata)
	tusReq.Header.Set("User-Agent", defaultUserAgent)

	tusResp, err := u.client.Do(tusReq)
	if err != nil {
		return "", fmt.Errorf("tus create: %w", err)
	}
	defer tusResp.Body.Close()

	if tusResp.StatusCode != http.StatusCreated {
		tusBody, _ := io.ReadAll(tusResp.Body)
		return "", fmt.Errorf("tus create status %d: %s", tusResp.StatusCode, string(tusBody))
	}

	location := tusResp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("missing Location header in tus create response")
	}

	return location, nil
}

func (u *UPnShareUploader) getUploadEndpoint(apiKey string) (tusURL, accessToken string, err error) {
	req, err := http.NewRequest("GET", "https://upnshare.com/api/v1/video/upload", nil)
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-token", apiKey)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := u.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("status 429: rate limit — %s", strings.TrimSpace(string(body)))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized || isQuotaExceeded(string(body)) {
			return "", "", &permanentError{err: errMsg}
		}
		return "", "", errMsg
	}

	var ep upnshareUploadEndpointResp
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}

	if ep.TusURL == "" || ep.AccessToken == "" {
		return "", "", fmt.Errorf("empty tus URL or access token in response")
	}

	return ep.TusURL, ep.AccessToken, nil
}

func (u *UPnShareUploader) uploadTUSRaw(uploadURL, filePath string, progress ProgressFunc) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, _ := os.Stat(filePath)
	fileSize := fi.Size()

	offset, err := u.getTUSOffset(uploadURL)
	if err != nil {
		return fmt.Errorf("get offset: %w", err)
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("seek to offset %d: %w", offset, err)
		}
	}

	buf := make([]byte, upnshareChunkSize)
	for offset < fileSize {
		chunkSize := int64(upnshareChunkSize)
		if remaining := fileSize - offset; remaining < chunkSize {
			chunkSize = remaining
		}

		n, err := io.ReadFull(f, buf[:chunkSize])
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("read chunk at offset %d: %w", offset, err)
		}
		if int64(n) == 0 {
			break
		}

		chunkBody := bytes.NewReader(buf[:n])
		req, err := http.NewRequest("PATCH", uploadURL, chunkBody)
		if err != nil {
			return fmt.Errorf("create patch request: %w", err)
		}
		req.Header.Set("Tus-Resumable", "1.0.0")
		req.Header.Set("Content-Type", "application/offset+octet-stream")
		req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
		req.ContentLength = int64(n)
		req.Header.Set("User-Agent", defaultUserAgent)

		resp, err := u.client.Do(req)
		if err != nil {
			return fmt.Errorf("tus upload chunk at offset %d: %w", offset, err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("tus upload status %d at offset %d: %s", resp.StatusCode, offset, string(respBody))
		}

		newOffset := resp.Header.Get("Upload-Offset")
		if newOffset != "" {
			offset, err = strconv.ParseInt(newOffset, 10, 64)
			if err != nil {
				return fmt.Errorf("parse upload-offset header: %w", err)
			}
		} else {
			offset += int64(n)
		}

		if progress != nil {
			progress("UPnShare", offset, fileSize)
		}
	}

	return nil
}

func (u *UPnShareUploader) getTUSOffset(uploadURL string) (int64, error) {
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
