package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func defaultConfig() Config {
	return Config{
		IP:     "192.168.0.100",
		Port:   8554,
		Width:  1280,
		Height: 720,
	}
}

func configPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "DioupeCamDesktop", "config.json")
}

func loadConfig() Config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		saveConfig(cfg)
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg Config) {
	path := configPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}
