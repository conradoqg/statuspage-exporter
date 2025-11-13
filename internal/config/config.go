package config

import (
    "fmt"
    "os"
    "time"

    "gopkg.in/yaml.v3"
)

type Server struct {
    Listen string `yaml:"listen"`
}

type Common struct {
    // Global scrape timeout
    Timeout time.Duration `yaml:"timeout"`
    // Default: 30s
    Interval time.Duration `yaml:"interval"`
    // Optional HTTP user agent
    UserAgent string `yaml:"user_agent"`
}

type Page struct {
    // A short name you choose for this target
    Name string `yaml:"name"`
    // Provider type: statuspage|instatus|statusio|azuredevops|gcp|aws_rss|betterstack
    Type string `yaml:"type"`

    // Base URL or API endpoint depending on provider
    URL string `yaml:"url"`

    // Optional: Human-friendly status page URL to display in dashboards
    UserFriendlyURL string `yaml:"user_friendly_url"`

    // Optional: For providers that need credentials
    APIToken string `yaml:"api_token"`
    // Optional: For Better Stack status page id, or other ids
    PageID string `yaml:"page_id"`

    // AWS RSS: list of feeds to track (service/region specific)
    Feeds []Feed `yaml:"feeds"`

    // Override intervals per page
    Interval *time.Duration `yaml:"interval"`
    Timeout  *time.Duration `yaml:"timeout"`
}

type Feed struct {
    URL     string `yaml:"url"`
    Service string `yaml:"service"`
    Region  string `yaml:"region"`
}

type Config struct {
    Server Server `yaml:"server"`
    Common Common `yaml:"common"`
    Pages  []Page `yaml:"pages"`
}

func Load(path string) (*Config, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }
    var c Config
    if err := yaml.Unmarshal(b, &c); err != nil {
        return nil, fmt.Errorf("parse yaml: %w", err)
    }
    // Defaults
    if c.Server.Listen == "" {
        c.Server.Listen = ":8080"
    }
    if c.Common.Interval == 0 {
        c.Common.Interval = 30 * time.Second
    }
    if c.Common.Timeout == 0 {
        c.Common.Timeout = 10 * time.Second
    }
    if c.Common.UserAgent == "" {
        c.Common.UserAgent = "statuspage-exporter/0.1"
    }
    return &c, nil
}
