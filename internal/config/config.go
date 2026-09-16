package config

import (
	"os"
	"strconv"
	"time"
)

// DefaultPortalURL is the web app of Dresden's OpenData portal, whose search
// backend lists every dataset together with its published resources.
const DefaultPortalURL = "https://opendata.dresden.de/informationsportal"

type Config struct {
	PortalURL      string
	RequestTimeout time.Duration
}

func LoadConfig() *Config {
	return &Config{
		PortalURL:      DefaultPortalURL,
		RequestTimeout: time.Duration(getEnvInt("REQUEST_TIMEOUT_MS", 30000)) * time.Millisecond,
	}
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
