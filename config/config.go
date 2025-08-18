package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Port           string            `yaml:"port"`
	UpstreamURL    string            `yaml:"upstreamURL"`
	UpstreamAPIKey string            `yaml:"upstreamAPIKey"`
	ModelMappings  map[string]string `yaml:"modelMappings"`
	LogLevel       string            `yaml:"logLevel"`
}

func Load() (*Config, error) {
	configPaths := []string{
		"./config.yaml",
		"/etc/llm-proxy/config.yaml",
		os.Getenv("HOME") + "/.llm-proxy/config.yaml",
	}

	var config = Config{
		Port: "4000",
	}

	for _, path := range configPaths {
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, &config); err != nil {
				return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
			}
			break
		}
	}

	if config.UpstreamURL == "" {
		return nil, fmt.Errorf("upstream_url is required")
	}

	return &config, nil
}
