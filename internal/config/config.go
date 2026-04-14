package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultServer = "https://liaison.cloud"
	EnvToken      = "LIAISON_TOKEN"
	EnvServer     = "LIAISON_SERVER"
)

// Config is persisted to ~/.liaison/config.yaml
type Config struct {
	Server string `yaml:"server"`
	Token  string `yaml:"token"`
}

// DefaultPath returns ~/.liaison/config.yaml
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".liaison", "config.yaml"), nil
}

// Load reads the config file. Returns an empty Config (not an error) if the
// file does not exist — first-run case.
func Load(path string) (*Config, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config file, creating parent dirs and setting mode 0600
// because it contains the auth token.
func Save(path string, cfg *Config) error {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return err
		}
		path = p
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Resolve merges persisted config with environment variables and explicit
// overrides from command-line flags. Precedence (highest first):
//
//  1. Explicit flag value (passed in)
//  2. Environment variable
//  3. Config file
//  4. Built-in default
func Resolve(persisted *Config, flagServer, flagToken string) *Config {
	out := &Config{
		Server: DefaultServer,
	}
	if persisted != nil {
		if persisted.Server != "" {
			out.Server = persisted.Server
		}
		if persisted.Token != "" {
			out.Token = persisted.Token
		}
	}
	if v := os.Getenv(EnvServer); v != "" {
		out.Server = v
	}
	if v := os.Getenv(EnvToken); v != "" {
		out.Token = v
	}
	if flagServer != "" {
		out.Server = flagServer
	}
	if flagToken != "" {
		out.Token = flagToken
	}
	return out
}
