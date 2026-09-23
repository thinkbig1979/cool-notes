package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateOldConfigDir(t *testing.T) {
	base := t.TempDir()
	oldDir, newDir := filepath.Join(base, "cool-note"), filepath.Join(base, "cool-notes")
	os.MkdirAll(oldDir, 0o700)
	os.WriteFile(filepath.Join(oldDir, "config.json"), []byte(`{"file":"/x/notes.txt"}`), 0o600)

	migrate(oldDir, newDir)
	c, err := Load(newDir)
	if err != nil || c.File != "/x/notes.txt" {
		t.Fatalf("after migrate: %+v, %v", c, err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("old dir should be gone")
	}

	// A second old dir appearing later is ignored: the new one wins.
	os.MkdirAll(oldDir, 0o700)
	os.WriteFile(filepath.Join(oldDir, "config.json"), []byte(`{"file":"/other.txt"}`), 0o600)
	migrate(oldDir, newDir)
	if c, _ := Load(newDir); c.File != "/x/notes.txt" {
		t.Fatalf("new config overwritten: %+v", c)
	}
}
