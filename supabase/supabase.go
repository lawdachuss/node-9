package supabase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles interactions with Supabase
type Client struct {
	url        string
	apiKey     string
	httpClient *http.Client
}

// UploadRecord represents a Gofile upload record
type UploadRecord struct {
	ID           int       `json:"id,omitempty"`
	StreamerName string    `json:"streamer_name"`
	GofileLink   string    `json:"gofile_link"`
	UploadDate   time.Time `json:"upload_date"`
}

// NewClient creates a new Supabase client
func NewClient(url, apiKey string) *Client {
	return &Client{
		url:    url,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// InsertUploadRecord stores a new Gofile upload record in Supabase
func (c *Client) InsertUploadRecord(streamerName, gofileLink string) error {
	record := UploadRecord{
		StreamerName: streamerName,
		GofileLink:   gofileLink,
		UploadDate:   time.Now(),
	}

	jsonData, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	req, err := http.NewRequest("POST", c.url+"/rest/v1/gofile_uploads", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
