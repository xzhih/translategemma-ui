package config

import (
	"os"
	"path/filepath"
)

const dataDirName = ".translategemma-ui"

// DefaultDataRoot resolves the per-user application data directory.
func DefaultDataRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dataDirName), nil
}

// EnsureDataDirs creates and returns the app data root.
func EnsureDataDirs(override string) (string, error) {
	root := override
	if root == "" {
		var err error
		root, err = DefaultDataRoot()
		if err != nil {
			return "", err
		}
	}

	dirs := []string{
		root,
		filepath.Join(root, "logs"),
		filepath.Join(root, "runtimes"),
		filepath.Join(root, "tmp"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	return root, nil
}
