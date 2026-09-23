// Package config stores user settings and session state between runs.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config holds user settings.
type Config struct {
	File  string `json:"file"`            // notes file
	Theme string `json:"theme,omitempty"` // theme name, "" means auto
}

// Cursor is a saved cursor position in one note.
type Cursor struct {
	Row    int `json:"row"`
	Col    int `json:"col"`
	Scroll int `json:"scroll"`
}

// State is where the user left off, so the next run can continue there.
type State struct {
	File    string   `json:"file"`
	Active  int      `json:"active"`
	Cursors []Cursor `json:"cursors"`
}

// Dir returns the config directory. COOL_NOTES_CONFIG_DIR overrides it.
func Dir() (string, error) {
	if d := os.Getenv("COOL_NOTES_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "cool-notes")
	migrate(filepath.Join(base, "cool-note"), dir)
	return dir, nil
}

// migrate moves settings from the app's old name (cool-note) to the new
// one, once. If both exist, the new one wins and the old one is left alone.
func migrate(oldDir, newDir string) {
	if _, err := os.Stat(newDir); err == nil {
		return
	}
	if info, err := os.Stat(oldDir); err == nil && info.IsDir() {
		os.Rename(oldDir, newDir)
	}
}

// DefaultNotesPath is suggested on first run.
func DefaultNotesPath() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "cool-notes", "notes.txt")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "notes.txt"
	}
	return filepath.Join(home, ".local", "share", "cool-notes", "notes.txt")
}

// ExpandPath resolves a leading ~ and makes path absolute.
func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	return filepath.Abs(path)
}

// ShortenPath replaces the home directory prefix with ~.
func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + path[len(home):]
	}
	return path
}

func readJSON(name string, v any) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(name string, v any) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}

// Load reads the config. A missing config returns a zero Config and no error.
func Load(dir string) (Config, error) {
	var c Config
	err := readJSON(filepath.Join(dir, "config.json"), &c)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	return c, err
}

// Save writes the config.
func Save(dir string, c Config) error {
	return writeJSON(filepath.Join(dir, "config.json"), c)
}

// LoadState reads the saved state. Errors yield an empty state: losing the
// cursor position is never worth failing over.
func LoadState(dir string) State {
	var s State
	if readJSON(filepath.Join(dir, "state.json"), &s) != nil {
		return State{}
	}
	return s
}

// SaveState writes the session state.
func SaveState(dir string, s State) error {
	return writeJSON(filepath.Join(dir, "state.json"), s)
}
