package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type NATSConfig struct {
	Servers       []string `mapstructure:"servers"`
	ClientID      string   `mapstructure:"client_id"`
	Subject       string   `mapstructure:"subject"`
	SubjectPrefix string   `mapstructure:"subject_prefix"`
}

type SafeSocketConfig struct {
	Port int `mapstructure:"port"`
}

type Config struct {
	TelegramToken string           `mapstructure:"TB_TOKEN"`
	ChatID        string           `mapstructure:"TB_CHATID"`
	BindIP        string           `mapstructure:"TB_IP"`
	BindPort      int              `mapstructure:"TB_PORT"`
	LogLevel      string           `mapstructure:"LOG_LEVEL"`
	Nats          NATSConfig       `mapstructure:"nats"`
	SafeSocket    SafeSocketConfig `mapstructure:"safesocket"`
}

// -----------------------------------------------------------------------------
// LoadConfig initializes Viper and parses the environment variables
func LoadConfig() (*Config, error) {
	viper.SetEnvPrefix("TELEREMOTE")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults similar to Python constants
	viper.SetDefault("TB_IP", "0.0.0.0")
	viper.SetDefault("TB_PORT", 50051) // updated to standard gRPC port
	viper.SetDefault("LOG_LEVEL", "DEBUG")

	// Optional config file loading
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	_ = viper.ReadInConfig() // ignore if not found, we rely on ENV vars mainly

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into config struct, %v", err)
	}

	return &cfg, nil
}
