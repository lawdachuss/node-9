package server

import (
	"encoding/json"
	"fmt"

	"github.com/teacat/chaturbate-dvr/entity"
)

var Config *entity.Config

type persistedSettings struct {
	Cookies   string `json:"cookies"`
	UserAgent string `json:"user_agent"`
	ByparrURL string `json:"byparr_url"`
}

// SaveSettings writes the runtime cookies and user-agent to Supabase.
func SaveSettings() error {
	s := persistedSettings{
		Cookies:   Config.Cookies,
		UserAgent: Config.UserAgent,
		ByparrURL: Config.ByparrURL,
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return SaveSettingsToDB(b)
}

// LoadSettings reads persisted cookies and user-agent from Supabase.
func LoadSettings() error {
	b := LoadSettingsFromDB()
	if b == nil {
		return nil
	}

	var s persistedSettings
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("unmarshal settings: %w", err)
	}
	if Config.Cookies == "" && s.Cookies != "" {
		Config.Cookies = s.Cookies
	}
	if Config.UserAgent == "" && s.UserAgent != "" {
		Config.UserAgent = s.UserAgent
	}
	if Config.ByparrURL == "" && s.ByparrURL != "" {
		Config.ByparrURL = s.ByparrURL
	}
	return nil
}
