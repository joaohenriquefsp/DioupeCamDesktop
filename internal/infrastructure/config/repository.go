package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"dioupecamdesktop/internal/domain"
)

func Load() domain.Config {
	cfg := domain.DefaultConfig()
	data, err := os.ReadFile(Path())
	if err != nil {
		Save(cfg)
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func Save(cfg domain.Config) {
	path := Path()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

func Path() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "DioupeCamDesktop", "config.json")
}
