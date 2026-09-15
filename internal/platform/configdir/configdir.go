package configdir

import (
	"os"
	"path/filepath"
	"runtime"
)

func AskDir() (string, error) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "ask"), nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "ask"), nil
}
