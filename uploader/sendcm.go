package uploader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	sendcmAPIBase = "https://send.now/api"
)

// SendCMUploader handles uploading files to SendCM
type SendCMUploader struct {
	apiKey string
	client *http.Client
}

// NewSendCMUploader creates a new SendCM uploader instance
func NewSendCMUploader(apiKey string) *SendCMUploader {
	return &SendCMUploader{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 60 * time.Minute, // Increased timeout for large files
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   100,
				IdleConnTimeout:       90 * time.Second,
				DisableCompression:    true,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}
}

type sendcmServerResponse struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Result string `json:"result"`
}

type sendcmUploadResponseItem struct {
	FileStatus string `json:"file_status"`
	FileCode   string `json:"file_code"`
}

type sendcmUploadResponse []sendcmUploadResponseItem

// Upload uploads a file to SendCM and returns the download link
func (u *SendCMUploader) Upload(filePath string) (string, error) {
	var lastErr error

	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			backoff := time.Duration((1<<uint(attempt-2))*5) * time.Second
			time.Sleep(backoff)
		}

		downloadLink, err := u.uploadFile(filePath)
		if err != nil {
			lastErr = fmt.Errorf("upload file: %w", err)
			if attempt < maxAttempts {
				continue
			}
			return "", lastErr
		}

		return downloadLink, nil
	}

	return "", lastErr
}

func (u *SendCMUploader) getUploadServer() (string, error) {
	var url string
	if u.apiKey != "" {
		url = fmt.Sprintf("%s/upload/server?key=%s", sendcmAPIBase, u.apiKey)
	} else {
		url = fmt.Sprintf("%s/upload/server", sendcmAPIBase)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := u.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request upload server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get upload server failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var serverResp sendcmServerResponse
	if err := json.NewDecoder(resp.Body).Decode(&serverResp); err != nil {
		return "", fmt.Errorf("decode server response: %w", err)
	}

	if serverResp.Status != 200 {
		return "", fmt.Errorf("server status not ok: %d (msg: %s)", serverResp.Status, serverResp.Msg)
	}

	if serverResp.Result == "" {
		return "", fmt.Errorf("no upload server URL in response")
	}

	return serverResp.Result, nil
}

func (u *SendCMUploader) uploadFile(filePath string) (string, error) {
	uploadServer, err := u.getUploadServer()
	if err != nil {
		return "", fmt.Errorf("get upload server: %w", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Get file size for progress tracking
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	fileSize := fileInfo.Size()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if u.apiKey != "" {
		if err := writer.WriteField("key", u.apiKey); err != nil {
			return "", fmt.Errorf("write key field: %w", err)
		}
	}

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close writer: %w", err)
	}

	req, err := http.NewRequest("POST", uploadServer, body)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.ContentLength = int64(body.Len())

	resp, err := u.client.Do(req)
	if err != nil {
		// Provide more context about the error
		return "", fmt.Errorf("do request (file size: %d bytes, server: %s): %w", fileSize, uploadServer, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed with status %d (file size: %d bytes): %s", resp.StatusCode, fileSize, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	var uploadResp sendcmUploadResponse
	if err := json.Unmarshal(bodyBytes, &uploadResp); err != nil {
		return "", fmt.Errorf("decode upload response (body: %s): %w", string(bodyBytes), err)
	}

	if len(uploadResp) == 0 {
		return "", fmt.Errorf("empty upload response (body: %s)", string(bodyBytes))
	}

	if uploadResp[0].FileStatus != "OK" {
		return "", fmt.Errorf("upload failed: file_status=%s (body: %s)", uploadResp[0].FileStatus, string(bodyBytes))
	}

	if uploadResp[0].FileCode == "" {
		return "", fmt.Errorf("no file code in response (body: %s)", string(bodyBytes))
	}

	viewURL := fmt.Sprintf("https://send.now/%s", uploadResp[0].FileCode)
	return viewURL, nil
}
