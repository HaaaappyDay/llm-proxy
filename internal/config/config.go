package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultListenHost = "127.0.0.1"
	DefaultListenPort = "15721"
	UserAgent         = "llm-proxy/0.1"
)

type Config struct {
	ListenHost string
	ListenPort string
	DataDir    string
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".llm-proxy")
	return &Config{
		ListenHost: DefaultListenHost,
		ListenPort: DefaultListenPort,
		DataDir:    dataDir,
	}
}

func (c *Config) ListenAddr() string {
	return c.ListenHost + ":" + c.ListenPort
}

func (c *Config) BaseURL() string {
	return "http://" + c.ListenAddr()
}
