package server

import (
        "bytes"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "strings"
        "time"

        "github.com/teacat/chaturbate-dvr/database"
        "github.com/teacat/chaturbate-dvr/entity"
)

// ─── Supabase client ──────────────────────────────────────────────────────────

var dbClient *database.Client

// GetDBClient returns the Supabase database client
func GetDBClient() *database.Client {
        if dbClient == nil && Config != nil && Config.SupabaseURL != "" && Config.SupabaseAPIKey != "" {
                dbClient = database.NewClient(Config.SupabaseURL, Config.SupabaseAPIKey)
        }
        return dbClient
}

func supabaseRestURL() string {
        if Config == nil || Config.SupabaseURL == "" {
                return ""
        }
        return Config.SupabaseURL + "/rest/v1"
}

func supabaseRestAPIKey() string {
        if Config == nil {
                return ""
        }
        return Config.SupabaseAPIKey
}

func supabaseRequest(method, path string, body []byte) (*http.Response, error) {
        baseURL := supabaseRestURL()
        apiKey := supabaseRestAPIKey()
        if baseURL == "" || apiKey == "" {
                return nil, fmt.Errorf("Supabase not configured")
        }

        req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
        if err != nil {
                return nil, fmt.Errorf("create request: %w", err)
        }
        req.Header.Set("apikey", apiKey)
        req.Header.Set("Authorization", "Bearer "+apiKey)
        if body != nil {
                req.Header.Set("Content-Type", "application/json")
                req.Header.Set("Prefer", "resolution=merge-duplicates")
        }
        client := &http.Client{Timeout: 15 * time.Second}
        return client.Do(req)
}

// CheckSupabase verifies the app_settings table is reachable via the REST API.
func CheckSupabase() error {
        client := GetDBClient()
        if client == nil {
                return fmt.Errorf("Supabase not configured")
        }
        return client.HealthCheck()
}

// ─── app_settings helpers ─────────────────────────────────────────────────────

// saveJSONSetting upserts a JSON value into the app_settings table via REST.
func saveJSONSetting(key string, data []byte) error {
        var rawJSON json.RawMessage
        if err := json.Unmarshal(data, &rawJSON); err != nil {
                return fmt.Errorf("parse json: %w", err)
        }

        body := map[string]interface{}{
                "key":   key,
                "value": rawJSON,
        }
        bodyBytes, err := json.Marshal(body)
        if err != nil {
                return fmt.Errorf("marshal: %w", err)
        }

        resp, err := supabaseRequest("POST", "/app_settings", bodyBytes)
        if err != nil {
                return fmt.Errorf("request: %w", err)
        }
        defer resp.Body.Close()
        if resp.StatusCode >= 400 {
                b, _ := io.ReadAll(resp.Body)
                return fmt.Errorf("api returned %d: %s", resp.StatusCode, string(b))
        }
        return nil
}

// loadJSONSetting reads a JSON value from the app_settings table via REST.
// Returns nil if the key is not found or on any error.
func loadJSONSetting(key string) []byte {
        resp, err := supabaseRequest("GET",
                "/app_settings?key=eq."+key+"&select=value", nil)
        if err != nil {
                return nil
        }
        defer resp.Body.Close()
        if resp.StatusCode != 200 {
                return nil
        }

        body, err := io.ReadAll(resp.Body)
        if err != nil {
                return nil
        }

        var entries []struct {
                Value json.RawMessage `json:"value"`
        }
        if err := json.Unmarshal(body, &entries); err != nil {
                return nil
        }
        if len(entries) == 0 {
                return nil
        }
        return []byte(string(entries[0].Value))
}

// ─── Channels ─────────────────────────────────────────────────────────────────

// SaveChannelsToDB saves channels to Supabase.
func SaveChannelsToDB(data []byte) error {
        client := GetDBClient()
        if client == nil {
                return fmt.Errorf("Supabase not configured")
        }

        var configs []*entity.ChannelConfig
        if err := json.Unmarshal(data, &configs); err != nil {
                return fmt.Errorf("unmarshal channels: %w", err)
        }

        for _, conf := range configs {
                ch := &database.Channel{
                        Username:    conf.Username,
                        IsPaused:    conf.IsPaused.Load(),
                        Framerate:   conf.Framerate,
                        Resolution:  conf.Resolution,
                        Pattern:     conf.Pattern,
                        MaxDuration: conf.MaxDuration,
                        MaxFilesize: conf.MaxFilesize,
                        Compress:    conf.Compress,
                        CreatedAt:   conf.CreatedAt,
                }
                if err := client.SaveChannel(ch); err != nil {
                        return fmt.Errorf("save channel %s: %w", conf.Username, err)
                }
        }

        return nil
}

// LoadChannelsFromDB loads channels from Supabase
func LoadChannelsFromDB() []byte {
        client := GetDBClient()
        if client == nil {
                return nil
        }

        channels, err := client.GetAllChannels()
        if err != nil {
                fmt.Printf("[WARN] failed to load channels from Supabase: %v\n", err)
                return nil
        }

        // Convert to entity.ChannelConfig format
        configs := make([]*entity.ChannelConfig, len(channels))
        for i, ch := range channels {
                configs[i] = &entity.ChannelConfig{
                        Username:    ch.Username,
                        Framerate:   ch.Framerate,
                        Resolution:  ch.Resolution,
                        Pattern:     ch.Pattern,
                        MaxDuration: ch.MaxDuration,
                        MaxFilesize: ch.MaxFilesize,
                        Compress:    ch.Compress,
                        CreatedAt:   ch.CreatedAt,
                }
                configs[i].IsPaused.Store(ch.IsPaused)
        }

        data, err := json.Marshal(configs)
        if err != nil {
                return nil
        }
        return data
}

// ─── Settings ─────────────────────────────────────────────────────────────────

func SaveSettingsToDB(data []byte) error {
        if err := saveJSONSetting("dvr_settings", data); err != nil {
                return fmt.Errorf("save settings to Supabase: %w", err)
        }
        return nil
}

func LoadSettingsFromDB() []byte {
        return loadJSONSetting("dvr_settings")
}

// ─── Recordings ───────────────────────────────────────────────────────────────

// SaveRecordingsToDB saves recordings to Supabase
func SaveRecordingsToDB(data []byte) error {
        client := GetDBClient()
        if client == nil {
                return fmt.Errorf("Supabase not configured")
        }

        // Parse the JSON data
        type RecordingEntry struct {
                Filename     string            `json:"filename"`
                Timestamp    string            `json:"timestamp"`
                RoomTitle    string            `json:"room_title"`
                Tags         []string          `json:"tags"`
                Viewers      int               `json:"viewers"`
                Resolution   string            `json:"resolution"`
                Framerate    int               `json:"framerate"`
                Links        map[string]string `json:"links"`
                ThumbnailURL string            `json:"thumbnail_url"`
                SpriteURL    string            `json:"sprite_url"`
                EmbedURL     string            `json:"embed_url"`
                Filesize     int64             `json:"filesize"`
        }

        type ChannelRecordings struct {
                Gender     string            `json:"gender"`
                Recordings []RecordingEntry  `json:"recordings"`
        }

        type RecordingsDB struct {
                Version  int                          `json:"version"`
                Channels map[string]*ChannelRecordings `json:"channels"`
        }

        var db RecordingsDB
        if err := json.Unmarshal(data, &db); err != nil {
                return fmt.Errorf("parse recordings: %w", err)
        }

        for username, chanData := range db.Channels {
                for _, rec := range chanData.Recordings {
                        recording := &database.Recording{
                                Username:     username,
                                Filename:     rec.Filename,
                                Timestamp:    rec.Timestamp,
                                RoomTitle:    rec.RoomTitle,
                                Tags:         rec.Tags,
                                Viewers:      rec.Viewers,
                                Resolution:   rec.Resolution,
                                Framerate:    rec.Framerate,
                                Filesize:     rec.Filesize,
                                Gender:       chanData.Gender,
                                ThumbnailURL: rec.ThumbnailURL,
                                SpriteURL:    rec.SpriteURL,
                                EmbedURL:     rec.EmbedURL,
                        }

                        if err := client.SaveRecording(recording); err != nil {
                                return fmt.Errorf("save recording %s: %w", rec.Filename, err)
                        }

                        savedRec, err := client.GetRecording(rec.Filename)
                        if err != nil {
                                return fmt.Errorf("get recording %s after save: %w", rec.Filename, err)
                        }

                        for host, url := range rec.Links {
                                link := &database.UploadLink{
                                        RecordingID: savedRec.ID,
                                        Host:        host,
                                        URL:         url,
                                }
                                if err := client.SaveUploadLink(link); err != nil {
                                        return fmt.Errorf("save upload link %s/%s: %w", rec.Filename, host, err)
                                }
                        }

                        if rec.ThumbnailURL != "" || rec.SpriteURL != "" {
                                img := &database.PreviewImage{
                                        RecordingID:  savedRec.ID,
                                        Filename:     rec.Filename,
                                        ThumbnailURL: rec.ThumbnailURL,
                                        SpriteURL:    rec.SpriteURL,
                                }
                                if err := client.SavePreviewImage(img); err != nil {
                                        return fmt.Errorf("save preview image %s: %w", rec.Filename, err)
                                }
                        }
                }
        }

        return nil
}

// LoadRecordingsFromDB loads recordings from Supabase
func LoadRecordingsFromDB() []byte {
        client := GetDBClient()
        if client == nil {
                return nil
        }

        recordings, err := client.GetAllRecordings()
        if err != nil {
                fmt.Printf("[WARN] Failed to load recordings from Supabase: %v\n", err)
                return nil
        }

        // Convert to the old JSON format for compatibility
        type RecordingEntry struct {
                Filename     string            `json:"filename"`
                Timestamp    string            `json:"timestamp"`
                RoomTitle    string            `json:"room_title"`
                Tags         []string          `json:"tags"`
                Viewers      int               `json:"viewers"`
                Resolution   string            `json:"resolution"`
                Framerate    int               `json:"framerate"`
                Links        map[string]string `json:"links"`
                ThumbnailURL string            `json:"thumbnail_url"`
                SpriteURL    string            `json:"sprite_url"`
                EmbedURL     string            `json:"embed_url"`
                Filesize     int64             `json:"filesize"`
        }

        type ChannelRecordings struct {
                Gender     string            `json:"gender"`
                Recordings []RecordingEntry  `json:"recordings"`
        }

        type RecordingsDB struct {
                Version  int                          `json:"version"`
                Channels map[string]*ChannelRecordings `json:"channels"`
        }

        db := RecordingsDB{
                Version:  2,
                Channels: make(map[string]*ChannelRecordings),
        }

        // Group recordings by username
        for _, rec := range recordings {
                chanData, ok := db.Channels[rec.Username]
                if !ok {
                        chanData = &ChannelRecordings{
                                Gender:     rec.Gender,
                                Recordings: []RecordingEntry{},
                        }
                        db.Channels[rec.Username] = chanData
                }

                // Get upload links for this recording
                links := make(map[string]string)
                if rec.ID != "" {
                        uploadLinks, err := client.GetUploadLinks(rec.ID)
                        if err == nil {
                                for _, link := range uploadLinks {
                                        links[link.Host] = link.URL
                                }
                        }
                }

                entry := RecordingEntry{
                        Filename:     rec.Filename,
                        Timestamp:    rec.Timestamp,
                        RoomTitle:    rec.RoomTitle,
                        Tags:         rec.Tags,
                        Viewers:      rec.Viewers,
                        Resolution:   rec.Resolution,
                        Framerate:    rec.Framerate,
                        Links:        links,
                        ThumbnailURL: rec.ThumbnailURL,
                        SpriteURL:    rec.SpriteURL,
                        EmbedURL:     rec.EmbedURL,
                        Filesize:     rec.Filesize,
                }

                chanData.Recordings = append(chanData.Recordings, entry)
        }

        data, err := json.Marshal(db)
        if err != nil {
                fmt.Printf("[WARN] Failed to marshal recordings: %v\n", err)
                return nil
        }

        return data
}

// SaveRecordingWithLinks saves a recording and its upload links directly to Supabase.
// Preview URLs should be saved separately via SavePreviewLinks before calling this.
// This function only saves the recording metadata and upload links.
func SaveRecordingWithLinks(username, filename, timestamp, roomTitle string, tags []string, viewers int, resolution string, framerate int, filesize int64, gender, embedURL string, links map[string]string) error {
        client := GetDBClient()
        if client == nil {
                return fmt.Errorf("Supabase not configured")
        }

        // Look up channel ID for foreign key
        rec := &database.Recording{
                Username:  username,
                Filename:  filename,
                Timestamp: timestamp,
                RoomTitle: roomTitle,
                Tags:      tags,
                Viewers:   viewers,
                Resolution: resolution,
                Framerate: framerate,
                Filesize:  filesize,
                Gender:    gender,
                EmbedURL:  embedURL,
        }
        if ch, err := client.GetChannel(username); err == nil {
                rec.ChannelID = ch.ID
        }

        // Save recording first
        if err := client.SaveRecording(rec); err != nil {
                return fmt.Errorf("save recording: %w", err)
        }

        // Get the saved recording to get its ID for upload links
        savedRec, err := client.GetRecording(filename)
        if err != nil {
                return fmt.Errorf("get recording after save: %w", err)
        }

        // Save upload links (non-fatal per link)
        var linkErrs []string
        for host, url := range links {
                link := &database.UploadLink{
                        RecordingID: savedRec.ID,
                        Host:        host,
                        URL:         url,
                }
                if err := client.SaveUploadLink(link); err != nil {
                        linkErrs = append(linkErrs, fmt.Sprintf("%s: %v", host, err))
                }
        }

        if len(linkErrs) > 0 {
                fmt.Printf("[WARN] Failed to save some upload links: %s\n", strings.Join(linkErrs, "; "))
        }

        return nil
}

// ─── Tunnels ──────────────────────────────────────────────────────────────────

// SaveTunnelToDB saves a tunnel URL to Supabase
func SaveTunnelToDB(tunnelURL string, runID int) error {
        client := GetDBClient()
        if client == nil {
                return fmt.Errorf("Supabase not configured")
        }

        if err := client.DeactivateOldTunnels(); err != nil {
                fmt.Printf("[WARN] failed to deactivate old tunnels: %v\n", err)
        }

        tunnel := &database.Tunnel{
                URL:      tunnelURL,
                RunID:    runID,
                IsActive: true,
        }

        return client.SaveTunnel(tunnel)
}

// LoadCurrentTunnel loads the active tunnel URL from Supabase
func LoadCurrentTunnel() (string, error) {
        client := GetDBClient()
        if client == nil {
                return "", nil
        }

        tunnel, err := client.GetActiveTunnel()
        if err != nil {
                return "", err
        }

        return tunnel.URL, nil
}

// ─── Preview Links ────────────────────────────────────────────────────────────

// SavePreviewLinks saves preview image URLs to Supabase
func SavePreviewLinks(filename, thumbnailURL, spriteURL string) error {
        client := GetDBClient()
        if client == nil {
                return fmt.Errorf("Supabase not configured")
        }

        img := &database.PreviewImage{
                Filename:     filename,
                ThumbnailURL: thumbnailURL,
                SpriteURL:    spriteURL,
        }

        return client.SavePreviewImage(img)
}

// LoadPreviewLinks loads preview image URLs from Supabase
func LoadPreviewLinks(filename string) (thumbnailURL, spriteURL string) {
        client := GetDBClient()
        if client == nil {
                return "", ""
        }

        img, err := client.GetPreviewImage(filename)
        if err != nil {
                return "", ""
        }

        return img.ThumbnailURL, img.SpriteURL
}

// LoadAllPreviewLinks returns a map of filename -> [thumbnailURL, spriteURL] for all preview images.
// Use this instead of calling LoadPreviewLinks in a loop to avoid N+1 queries.
func LoadAllPreviewLinks() map[string][2]string {
        client := GetDBClient()
        if client == nil {
                return nil
        }

        images, err := client.GetAllPreviewImages()
        if err != nil {
                fmt.Printf("[WARN] Failed to load all preview images: %v\n", err)
                return nil
        }

        result := make(map[string][2]string, len(images))
        for _, img := range images {
                if img.Filename != "" && (img.ThumbnailURL != "" || img.SpriteURL != "") {
                        result[img.Filename] = [2]string{img.ThumbnailURL, img.SpriteURL}
                }
        }
        return result
}

// DeleteChannelFromDB removes a channel record from Supabase.
func DeleteChannelFromDB(username string) error {
        client := GetDBClient()
        if client == nil {
                return nil
        }
        return client.DeleteChannel(username)
}

// DeleteChannelsNotInDB removes all Supabase channel rows whose username is NOT
// in the provided list. Pass an empty slice to delete all channels.
func DeleteChannelsNotInDB(usernames []string) error {
        client := GetDBClient()
        if client == nil {
                return nil
        }
        return client.DeleteChannelsNotIn(usernames)
}

// DeleteVideoCompletely removes all data associated with a video:
// - Recording from Supabase recordings table
// - Preview images from Supabase preview_images table
// - Upload links from Supabase upload_links table
// Returns a combined error if any deletion fails.
func DeleteVideoCompletely(filename string) error {
        client := GetDBClient()
        if client == nil {
                return nil // No DB configured, nothing to delete
        }

        var errs []string

        // Get recording ID first (needed for upload links)
        rec, err := client.GetRecording(filename)
        if err == nil && rec != nil {
                // Delete upload links by recording ID
                if err := client.DeleteUploadLinksByRecordingID(rec.ID); err != nil {
                        errs = append(errs, fmt.Sprintf("upload links: %v", err))
                }
        }

        // Delete preview images
        if err := client.DeletePreviewImage(filename); err != nil {
                errs = append(errs, fmt.Sprintf("preview images: %v", err))
        }

        // Delete recording
        if err := client.DeleteRecording(filename); err != nil {
                errs = append(errs, fmt.Sprintf("recording: %v", err))
        }

        if len(errs) > 0 {
                return fmt.Errorf("delete errors: %s", strings.Join(errs, "; "))
        }
        return nil
}
