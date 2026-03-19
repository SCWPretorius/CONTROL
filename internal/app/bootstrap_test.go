package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestPrepareCreatesConfiguredDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Config{
		Paths: config.PathConfig{
			RuntimeDir: filepath.Join(root, "runtime"),
			StorageDir: filepath.Join(root, "storage"),
		},
		Session: config.SessionConfig{
			ConfigDir:  filepath.Join(root, "runtime", "copilot"),
			WorkingDir: root,
		},
	}

	if err := Prepare(cfg); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	for _, dir := range []string{cfg.Paths.RuntimeDir, cfg.Paths.StorageDir, cfg.Session.ConfigDir} {
		if info, err := os.Stat(dir); err != nil {
			t.Fatalf("Stat(%q) error = %v", dir, err)
		} else if !info.IsDir() {
			t.Fatalf("%q was not created as a directory", dir)
		}
	}
}
