package app

import (
	"fmt"
	"os"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

// Prepare validates filesystem prerequisites needed by the local entrypoint.
func Prepare(cfg config.Config) error {
	if err := ensureDir(cfg.Paths.RuntimeDir, 0o700); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}

	if err := ensureDir(cfg.Paths.StorageDir, 0o700); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	if err := ensureDir(cfg.Session.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("create copilot config dir: %w", err)
	}

	if info, err := os.Stat(cfg.Session.WorkingDir); err != nil {
		return fmt.Errorf("stat working dir: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("working dir is not a directory: %s", cfg.Session.WorkingDir)
	}

	return nil
}

func ensureDir(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
