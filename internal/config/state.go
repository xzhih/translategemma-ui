package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	configFileName = "config.json"
	stateFileName  = "state.json"
)

type AppConfig struct {
	ActiveModelID string `json:"active_model_id,omitempty"`
	ListenAddr    string `json:"listen_addr,omitempty"`
	PreferredUI   string `json:"preferred_ui,omitempty"`
}

type InstalledArtifact struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	FileName  string `json:"file_name"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type AppState struct {
	Artifacts       []InstalledArtifact `json:"artifacts,omitempty"`
	ActiveModelPath string              `json:"active_model_path,omitempty"`
	BackendURL      string              `json:"backend_url,omitempty"`
	RuntimeMode     string              `json:"runtime_mode,omitempty"`
}

func LoadAppConfig(root string) (AppConfig, error) {
	var cfg AppConfig
	path := filepath.Join(root, configFileName)
	if err := readJSON(path, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AppConfig{}, nil
		}
		return AppConfig{}, err
	}
	return cfg, nil
}

func SaveAppConfig(root string, cfg AppConfig) error {
	path := filepath.Join(root, configFileName)
	return writeJSONAtomic(path, cfg)
}

func LoadAppState(root string) (AppState, error) {
	var st AppState
	path := filepath.Join(root, stateFileName)
	if err := readJSON(path, &st); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AppState{}, nil
		}
		return AppState{}, err
	}
	return st, nil
}

func SaveAppState(root string, st AppState) error {
	path := filepath.Join(root, stateFileName)
	return writeJSONAtomic(path, st)
}

func readJSON(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
