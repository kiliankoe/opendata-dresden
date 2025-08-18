package config

import (
	"os"
	"strconv"
	"time"
)

const (
	PortalURL    = "https://opendata.dresden.de"
	KommisDDURL  = "https://kommisdd.dresden.de"
	IkxPath      = "/net3/public/ogc.ashx"
	OGCAPIPath   = "/net4/public/ogcapi"
	SearchAPIURL = "https://opendata.dresden.de/informationsportal/service/app/search/all"
)

type Config struct {
	RequestTimeout time.Duration
}

func LoadConfig() *Config {
	cfg := &Config{
		RequestTimeout: time.Duration(getEnvInt("REQUEST_TIMEOUT_MS", 30000)) * time.Millisecond,
	}

	return cfg
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
