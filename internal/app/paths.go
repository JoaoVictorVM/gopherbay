package app

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	appDirName     = "gopherbay"
	configFileName = "config.yaml"
)

// DefaultConfigDir returns the gopherbay folder inside the user config directory,
// which holds both the log and the default config file.
func DefaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("localizar pasta de configuração: %w", err)
	}
	return filepath.Join(base, appDirName), nil
}

// ResolveConfigPath returns the config file gbay uses: the --config value made
// absolute against the working folder, or config.yaml inside configDir when the flag is empty.
func ResolveConfigPath(flagValue, configDir string) (string, error) {
	if flagValue == "" {
		return filepath.Join(configDir, configFileName), nil
	}
	abs, err := filepath.Abs(flagValue)
	if err != nil {
		return "", fmt.Errorf("resolver caminho de --config: %w", err)
	}
	return abs, nil
}
