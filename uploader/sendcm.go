package uploader

import (
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
			Timeout: 120 * time.Minute, // Extended timeout for large files
			Transport: &http.Transport{
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       90 * time.Second,
				DisableCompression:    true,
				TLSHandshakeTimeout:   30 * time.Second,
				ResponseHeaderTimeout: 120 * time.Second, // Increased for large uploads
				ExpectContinueTimeout: 1 * time.Second,
				DisableKeepAlives:     true, // Disable keep-alive to prevent stale connections
				WriteBufferSize:       64 * 1024, // 64KB write buffer for better throughput
				ReadBufferSize:        64 * 1024, // 64KB read buffer
				ForceAttemptHTTP2:     false, // Stick to HTTP/1.1 for better compatibility
			},
		},
	}
}

type sendcmServerResponse struct {
	Status int             `json:"status"`
	Msg    string          `json:"msg"`
	Result json.RawMessage `json:"result"`
}

// GetResult extracts the upload server URL from the result field
// Handles both string and object response formats
func (r *sendcmServerResponse) GetResult() (string, error) {
	// Try as string first
	var strResult string
	if err := json.Unmarshal(r.Result, &strResult); err == nil && strResult != "" {
		return strResult, nil
	}
	
	// Try as object with "url" field
	var objResult struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(r.Result, &objResult); err == nil && objResult.URL != "" {
		return objResult.URL, nil
	}
	
	return "", fmt.Errorf("unexpected result format: %s", string(r.Result))
}

type sendcmUploadResponseItem struct {
	FileStatus string `json:"file_status"`
	FileCode   string `json:"file_code"`
}

type sendcmUploadResponse []sendcmUploadResponseItem

// Upload uploads a file to SendCM and returns the download link
func (u *SendCMUploader) Upload(filePath string) (string, error) {
	var lastErr error

	maxAttempts := 5 // Increased from 3 to 5 attempts
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			// Exponential backoff: 5s, 10s, 20s, 40s
			backoff := time.Duration((1<<uint(attempt-2))*5) * time.Second
			time.Sleep(backoff)
		}

		downloadLink, err := u.uploadFile(filePath)
		if err != nil {
			lastErr = fmt.Errorf("upload file (attempt %d/%d): %w", attempt, maxAttempts, err)
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

	uploadServer, err := serverResp.GetResult()
	if err != nil {
		return "", fmt.Errorf("no upload server URL in response: %w", err)
	}
	if uploadServer == "" {
		return "", fmt.Errorf("no upload server URL in response")
	}

	return uploadServer, nil
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

	// Use a pipe to stream the multipart data directly without buffering
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	// Channel to capture errors from the goroutine
	errChan := make(chan error, 1)

	// Write multipart data in a goroutine
	go func() {
		defer func() {
			writer.Close()
			pipeWriter.Close()
		}()

		if u.apiKey != "" {
			if err := writer.WriteField("key", u.apiKey); err != nil {
				errChan <- fmt.Errorf("write key field: %w", err)
				pipeWriter.CloseWithError(err)
				return
			}
		}

		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			errChan <- fmt.Errorf("create form file: %w", err)
			pipeWriter.CloseWithError(err)
			return
		}

		buf := make([]byte, 1024*1024) // 1MB buffer
		if _, err := io.CopyBuffer(part, file, buf); err != nil {
			errChan <- fmt.Errorf("copy file: %w", err)
			pipeWriter.CloseWithError(err)
			return
		}
		
		errChan <- nil
	}()

	req, err := http.NewRequest("POST", uploadServer, pipeReader)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Connection", "close") // Force connection close after request
	// Don't set Content-Length for streaming uploads - let it be chunked transfer encoding

	resp, err := u.client.Do(req)
	if err != nil {
		// Check if there was an error in the goroutine
		select {
		case goroutineErr := <-errChan:
			if goroutineErr != nil {
				return "", fmt.Errorf("multipart write error: %w (request error: %v)", goroutineErr, err)
			}
		default:
		}
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
