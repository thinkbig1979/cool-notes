package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSaveBacksUpExternalChangeNotYetSeen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Save(1, []string{"mine"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	theirs := Serialize([]string{"theirs"})
	if err := os.WriteFile(path, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}
	// No CheckExternal in between: the save itself must notice.
	backup, err := f.Save(2, []string{"mine edited"})
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("save overwrote an external change without a backup")
	}
	if data, _ := os.ReadFile(backup); string(data) != theirs {
		t.Fatalf("backup = %q", data)
	}
	if got, _ := f.Load(); !reflect.DeepEqual(got, []string{"mine edited"}) {
		t.Fatalf("file = %q", got)
	}
	if backup, _ := f.Save(3, []string{"again"}); backup != "" {
		t.Fatalf("own write backed up again: %s", backup)
	}
}

func TestBackupsInSameSecondDoNotCollide(t *testing.T) {
	f, err := Open(filepath.Join(t.TempDir(), "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.Backup([]byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.Backup([]byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("both backups named %s", a)
	}
	if data, _ := os.ReadFile(a); string(data) != "first" {
		t.Fatalf("first backup = %q", data)
	}
}

func TestSaveKeepsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "synced", "notes.txt")
	os.MkdirAll(filepath.Dir(target), 0o700)
	if err := os.WriteFile(target, []byte(Serialize([]string{"old"})), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "notes.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	f, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Save(1, []string{"new"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link replaced by a regular file: %v", err)
	}
	data, _ := os.ReadFile(target)
	if got := Parse(string(data)); !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatalf("target = %q", got)
	}
}
