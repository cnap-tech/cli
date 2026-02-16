package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultAPIURL = "https://api.cnap.tech"
	configDir     = ".cnap"
	configFile    = "config.yaml"
)

type Config struct {
	APIURL          string `yaml:"api_url"`
	ActiveWorkspace string `yaml:"active_workspace,omitempty"`
	Auth            Auth   `yaml:"auth"`
	Output          Output `yaml:"output"`
}

type Auth struct {
	Token string `yaml:"token,omitempty"`
}

type Output struct {
	Format string `yaml:"format"` // table, json, quiet
}

func DefaultConfig() *Config {
	return &Config{
		APIURL: DefaultAPIURL,
		Output: Output{Format: "table"},
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, configDir, configFile), nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, configDir), nil
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}

// Token returns the API token from env var or config file.
// Env var CNAP_API_TOKEN takes priority.
func (c *Config) Token() string {
	if t := os.Getenv("CNAP_API_TOKEN"); t != "" {
		return t
	}
	return c.Auth.Token
}

// BaseURL returns the API base URL from env var or config file.
// Env var CNAP_API_URL takes priority.
func (c *Config) BaseURL() string {
	if u := os.Getenv("CNAP_API_URL"); u != "" {
		return u
	}
	return c.APIURL
}
