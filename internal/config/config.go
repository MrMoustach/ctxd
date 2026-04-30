package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	DBPath string `json:"db_path"`
}

func DefaultDir() (string, error) {
	if v := os.Getenv("CTX_HOME"); v != "" {
		return v, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "ctxd"), nil
}

func DefaultDBPath() (string, error) {
	if v := os.Getenv("CTX_DB"); v != "" {
		return v, nil
	}
	d, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "ctx.db"), nil
}

func Init() (Config, error) {
	db, err := DefaultDBPath()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{DBPath: db}
	dir := filepath.Dir(db)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Config{}, err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}
	return cfg, os.WriteFile(filepath.Join(dir, "config.json"), b, 0o644)
}
