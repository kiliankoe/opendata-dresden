package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Version is also parsed by flake.nix and bumped by `make release`, keep the
// declaration on one line.
const Version = "0.2.1"

// DefaultPortalURL is the web app of Dresden's OpenData portal, whose search
// backend lists every dataset together with its published resources.
const DefaultPortalURL = "https://opendata.dresden.de/informationsportal"

// DefaultIndexURL is where the nightly index of all datasets is published
const DefaultIndexURL = "https://raw.githubusercontent.com/kiliankoe/opendata-dresden/main/data/index.json"

// DefaultRepoAPI is the GitHub API of this repository, whose commit history
// records how the mirrored statistics tables change
const DefaultRepoAPI = "https://api.github.com/repos/kiliankoe/opendata-dresden"

type Config struct {
	PortalURL string
	// IndexURL locates the dataset index, either over HTTP or as a local path
	IndexURL string
	// CacheDir keeps a copy of the downloaded index; empty disables caching
	CacheDir string
	// RepoAPI locates the repository whose history is read by history
	RepoAPI string
	// GitHubToken raises the API's rate limit of 60 requests per hour when set
	GitHubToken    string
	RequestTimeout time.Duration
}

func LoadConfig() *Config {
	cfg := &Config{
		PortalURL:      DefaultPortalURL,
		IndexURL:       DefaultIndexURL,
		RepoAPI:        DefaultRepoAPI,
		GitHubToken:    os.Getenv("GITHUB_TOKEN"),
		RequestTimeout: time.Duration(getEnvInt("REQUEST_TIMEOUT_MS", 30000)) * time.Millisecond,
	}
	if url := os.Getenv("INDEX_URL"); url != "" {
		cfg.IndexURL = url
	}
	if url := os.Getenv("REPO_API"); url != "" {
		cfg.RepoAPI = url
	}
	if dir, err := os.UserCacheDir(); err == nil {
		cfg.CacheDir = filepath.Join(dir, "od3")
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
