package config

import (
	"fmt"
	"strings"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
)

// -----------------------------------------------------------------------------
// Configuration Types
// -----------------------------------------------------------------------------

// NATSConfig matches the NATS setup in the ecosystem
type NATSConfig struct {
	Servers       []string `json:"servers"`
	ClientID      string   `json:"client_id"`
	SubjectPrefix string   `json:"subject_prefix"`
}

// SafeSocketConfig matches the SafeSocket setup in the ecosystem
type SafeSocketConfig struct {
	Port int `json:"port"`
}

// TeleRemoteCap matches the specific capability for this service
type TeleRemoteCap struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
	URL    string `json:"url"`
	IP     string `json:"ip"`
	Port   int    `json:"port"`
}

// -----------------------------------------------------------------------------
// Unified Config Wrapper
// -----------------------------------------------------------------------------

// Config is the unified configuration object for tele-remote
type Config struct {
	*toolbox_config.AppConfig

	// Specific domains (cached for easy access)
	TelegramToken string
	TelegramURL   string
	ChatID        string
	
	BindIP        string
	BindPort      int

	Nats          NATSConfig
	SafeSocket    SafeSocketConfig
}

// -----------------------------------------------------------------------------
// Factory
// -----------------------------------------------------------------------------

// LoadConfig initializes the toolbox config and extracts tele-remote specific values
func LoadConfig(profile string) (*Config, error) {
	// Use toolbox to load everything (Env, standalone.yaml, Config Server)
	appConfig, err := toolbox_config.LoadConfig(profile, nil)
	if err != nil {
		return nil, fmt.Errorf("toolbox load failed: %w", err)
	}

	cfg := &Config{
		AppConfig: appConfig,
	}

	// 1. Extract TeleRemote Capability
	var tr TeleRemoteCap
	if err := appConfig.Config.GetCapability("tele_remote", &tr); err == nil {
		cfg.TelegramToken = tr.Token
		cfg.ChatID = tr.ChatID
		cfg.TelegramURL = tr.URL
		if cfg.TelegramURL == "" {
			cfg.TelegramURL = "https://api.telegram.org"
		}
		
		cfg.BindIP = tr.IP
		if cfg.BindIP == "" {
			cfg.BindIP = "0.0.0.0"
		}
		cfg.BindPort = tr.Port
		if cfg.BindPort == 0 {
			cfg.BindPort = 50051
		}
	}

	// 2. Extract NATS Settings
	_ = appConfig.Config.GetCapability("nats", &cfg.Nats)

	// 3. Extract SafeSocket Settings
	_ = appConfig.Config.GetCapability("safesocket", &cfg.SafeSocket)

	// Clean potential quotes
	cfg.TelegramToken = strings.Trim(cfg.TelegramToken, "\"")
	cfg.ChatID = strings.Trim(cfg.ChatID, "\"")

	return cfg, nil
}
