package entity

import (
	"regexp"
	"strings"
	"sync/atomic"
)

// Event represents the type of event for the channel.
type Event = string

const (
	EventUpdate Event = "update"
	EventLog    Event = "log"
)

// ChannelConfig represents the configuration for a channel.
type ChannelConfig struct {
	IsPaused    atomic.Bool `json:"-"`
	Username    string      `json:"username"`
	Framerate   int         `json:"framerate"`
	Resolution  int         `json:"resolution"`
	Pattern     string      `json:"pattern"`
	MaxDuration int         `json:"max_duration"`
	MaxFilesize int         `json:"max_filesize"`
	Compress    bool        `json:"compress"`
	CreatedAt   int64       `json:"created_at"`
}

func (c *ChannelConfig) Sanitize() {
	c.Username = regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(c.Username, "")
	c.Username = strings.TrimSpace(c.Username)
}

// ChannelInfo represents the information about a channel,
// mostly used for the template rendering.
type ChannelInfo struct {
	IsOnline      bool
	IsConnecting  bool
	IsPaused      bool
	IsCompressing bool
	RoomStatus    string // public, private, group, away, offline, hidden
	Username      string
	Duration      string
	Filesize      string
	Filename      string
	StreamedAt    string
	MaxDuration   string
	MaxFilesize   string
	CreatedAt     int64
	Logs          []string
	GlobalConfig  *Config // for nested template to access $.Config
}

// DiskInfo holds disk usage information for the UI.
type DiskInfo struct {
	Total   string // formatted, e.g. "256.00 GB"
	Used    string // formatted, e.g. "120.50 GB"
	Free    string // formatted, e.g. "135.50 GB"
	Percent int    // 0-100
	UsedGB  float64
	TotalGB float64
}

// Config holds the configuration for the application.
type Config struct {
	Version       string
	Username      string
	AdminUsername string
	AdminPassword string
	Framerate     int
	Resolution    int
	Pattern       string
	MaxDuration   int
	MaxFilesize   int
	Compress      bool
	Port          string
	Interval      int
	Cookies       string
	SessionID     string
	Csrftoken     string
	UserAgent     string
	Domain        string
	ProxyURL      string
	ProxyUsername string
	ProxyPassword string

	OutputDir               string
	PerModelFolder          bool
	DeleteLocalAfterUpload  bool
	OrphanCleanupInterval   int  // minutes between periodic orphan/thumbnail sweeps (0 = disabled)
	DiskWarningPercent      int  // log warning when disk usage exceeds this (0 = disabled, default 80)
	DiskCriticalPercent     int  // auto-delete oldest recordings when disk exceeds this (0 = disabled, default 90)
	MaxLocalAgeDays         int  // delete local files older than N days if uploaded (0 = disabled)

	TurboViPlayAPIKey string
	VoeSXAPIKey       string
	SendCMAPIKey      string
	ByseAPIKey        string
	StreamtapeLogin   string
	StreamtapeKey     string
	MixdropEmail      string
	MixdropToken      string
	PixelDrainToken   string

	GitHubToken       string
	GitHubRepo        string
	GitHubBranch      string
	GitHubPreviewPath string

	SupabaseURL    string
	SupabaseAPIKey string

	FFmpegPath string
}
