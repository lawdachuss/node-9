package uploader

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	gofileAPIBase   = "https://api.gofile.io"
	gofileUploadURL = "https://upload.gofile.io/uploadfile"
)

// GoFileUploader handles uploading files to GoFile.io
type GoFileUploader struct {
	client *http.Client
}

// NewGoFileUploader creates a new GoFile uploader instance
func NewGoFileUploader() *GoFileUploader {
	return &GoFileUploader{
		client: &http.Client{
			Timeout: uploadClientTimeout, // Long timeout for large video uploads
			Transport: newUploadTransport(false),
		},
	}
}

// gofileToken returns the optional GoFile API token (Bearer). When empty,
// GoFile creates an anonymous guest account per upload (legacy behaviour).
// Set GOFILE_API_TOKEN in the environment to authenticate with a real account
// and skip guest-account creation — this avoids the "error-createGuestAccount"
// 500s GoFile returns when its guest service is overloaded or rate-limiting an
// IP. A free GoFile account's API token is sufficient for uploads.
func gofileToken() string {
	return strings.TrimSpace(os.Getenv("GOFILE_API_TOKEN"))
}

type uploadResponse struct {
	Status string `json:"status"`
	Data   struct {
		DownloadPage string `json:"downloadPage"`
		Code         string `json:"code"`
		ParentFolder string `json:"parentFolder"`
		FileID       string `json:"fileId"`
		FileName     string `json:"fileName"`
		MD5          string `json:"md5"`
	} `json:"data"`
}

// Upload uploads a file to GoFile and returns the download page link.
func (u *GoFileUploader) Upload(filePath string) (string, error) {
	return u.UploadWithProgress(filePath, nil)
}

// UploadWithProgress uploads a file to GoFile and reports progress through fn.
func (u *GoFileUploader) UploadWithProgress(filePath string, progress ProgressFunc) (string, error) {
	var lastErr error

	// GoFile's guest-account service intermittently returns 500
	// "error-createGuestAccount".  Retry a few times with backoff; if a
	// GOFILE_API_TOKEN is configured we skip guest creation and won't hit it.
	maxAttempts := 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(uploadBackoff(attempt-2, lastErr))
		}

		url, err := u.uploadFile(filePath, progress)
		if err != nil {
			lastErr = fmt.Errorf("upload file: %w", err)
			if isUploadRateLimited(err) {
				// Rate limit — backoff and clear lastErr to avoid double-sleep next iteration
				time.Sleep(uploadBackoff(attempt, err))
				lastErr = nil
				continue
			}
			if attempt < maxAttempts {
				continue
			}
			return "", lastErr
		}

		// Success!
		return url, nil
	}

	return "", lastErr
}

func (u *GoFileUploader) uploadFile(filePath string, progress ProgressFunc) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Use pipe to stream the file without loading it all into memory
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	token := gofileToken()

	// Start writing in a goroutine
	errChan := make(chan error, 1)
	go func() {
		defer pipeWriter.Close()

		// When authenticated, send the token as a form field (GoFile accepts
		// both the Bearer header and/or this field).  Must be written before
		// the file part.
		if token != "" {
			if err := writer.WriteField("token", token); err != nil {
				errChan <- fmt.Errorf("write token field: %w", err)
				writer.Close()
				return
			}
		}

		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			errChan <- fmt.Errorf("create form file: %w", err)
			writer.Close()
			return
		}

		// Wrap file with ProgressReader for live upload tracking
		fi, _ := file.Stat()
		var fileSize int64
		if fi != nil {
			fileSize = fi.Size()
		}
		progressFile := NewProgressReaderWithCallback(file, fileSize, "GoFile", progress)

		// Use a larger buffer for faster copying (4MB chunks)
		buf := make([]byte, 4*1024*1024)
		if _, err := io.CopyBuffer(part, progressFile, buf); err != nil {
			errChan <- fmt.Errorf("copy file: %w", err)
			writer.Close()
			return
		}

		// Close writer before signaling success to flush multipart boundary
		if err := writer.Close(); err != nil {
			errChan <- fmt.Errorf("close writer: %w", err)
			return
		}

		errChan <- nil
	}()

	req, err := http.NewRequest("POST", gofileUploadURL, pipeReader)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		pipeReader.CloseWithError(err) // unblock the writer goroutine
		// Drain error channel to avoid goroutine leak
		select {
		case <-errChan:
		case <-time.After(5 * time.Second):
			// Timeout waiting for goroutine - it may be stuck
		}
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors from the goroutine
	select {
	case err := <-errChan:
		if err != nil {
			return "", err
		}
	case <-time.After(30 * time.Second):
		// Goroutine took too long - this shouldn't happen but prevents deadlock
		return "", fmt.Errorf("timeout waiting for file copy to complete")
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var uploadResp uploadResponse
	if err := json.Unmarshal(bodyBytes, &uploadResp); err != nil {
		return "", fmt.Errorf("decode upload response: %w (body: %s)", err, string(bodyBytes))
	}

	if uploadResp.Status != "ok" {
		return "", fmt.Errorf("upload status not ok: %s (body: %s)", uploadResp.Status, string(bodyBytes))
	}

	return uploadResp.Data.DownloadPage, nil
}
