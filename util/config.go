package util

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// AppConfig holds enterprise-level configuration for the application
// Add new fields as needed for future features
// Tag with mapstructure for viper compatibility
// Example: export ACCELIRA_PORT=8080
//          or set in accelira.yaml

type AppConfig struct {
	Port          int    `mapstructure:"port"`
	LogLevel      string `mapstructure:"log_level"`
	DashboardBind string `mapstructure:"dashboard_bind"`
	PrivateKey    string `mapstructure:"private_key"`
}

var config *AppConfig

// LoadConfig loads configuration from environment variables and config files
func LoadConfig() (*AppConfig, error) {
	if config != nil {
		return config, nil
	}
	v := viper.New()
	v.SetConfigName("accelira")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/accelira/")
	v.AutomaticEnv()
	v.SetEnvPrefix("ACCELIRA")

	// Set default values
	v.SetDefault("port", 8080)
	v.SetDefault("log_level", "info")
	v.SetDefault("dashboard_bind", ":8081")
	v.SetDefault("private_key", "")

	if err := v.ReadInConfig(); err != nil {
		// Only error if config file exists and is invalid
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}

	var c AppConfig
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	// Optionally, load secrets from env if not set
	if c.PrivateKey == "" {
		c.PrivateKey = os.Getenv("ACCELIRA_PRIVATE_KEY")
	}

	config = &c
	return config, nil
}

// GetConfig returns the loaded config, or panics if not loaded
func GetConfig() *AppConfig {
	if config == nil {
		panic("Config not loaded. Call LoadConfig() first.")
	}
	return config
}
