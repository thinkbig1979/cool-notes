package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// File is the notes file on disk. It writes atomically and can tell its own
// writes apart from changes made by other programs.
type File struct {
	Path string

	mu      sync.Mutex
	known   os.FileInfo // file as of our last read or write; nil if absent
	lastSeq uint64
}

// Open returns a File for path, creating its directory if needed. A symlink
// is resolved, so saving replaces the file it points to, not the link.
func Open(path string) (*File, error) {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return &File{Path: path}, nil
}

// Load reads and parses the file. A missing file holds no notes.
func (f *File) Load() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, info, err := f.read()
	if err != nil {
		return nil, err
	}
	f.known = info
	return Parse(string(data)), nil
}

func (f *File) read() ([]byte, os.FileInfo, error) {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(f.Path)
	if err != nil {
		return nil, nil, err
	}
	return data, info, nil
}

// Save writes notes atomically. seq orders concurrent saves: a save with a
// seq older than one already written is skipped, so a slow write can never
// replace newer content. If another program changed the file since our last
// read or write, its version is backed up first and the backup's name is
// returned.
func (f *File) Save(seq uint64, notes []string) (backup string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if seq != 0 && seq < f.lastSeq {
		return "", nil
	}
	if info, err := os.Stat(f.Path); err == nil && f.changed(info) {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return "", err
		}
		if backup, err = f.Backup(data); err != nil {
			return "", err
		}
	}
	if err := WriteAtomic(f.Path, []byte(Serialize(notes))); err != nil {
		return backup, err
	}
	f.lastSeq = seq
	info, err := os.Stat(f.Path)
	if err != nil {
		return backup, err
	}
	f.known = info
	return backup, nil
}

// changed reports whether info differs from the file as of our last read or
// write.
func (f *File) changed(info os.FileInfo) bool {
	return f.known == nil || !os.SameFile(f.known, info) ||
		!f.known.ModTime().Equal(info.ModTime()) || f.known.Size() != info.Size()
}

// External is a change made to the file by another program.
type External struct {
	Notes []string
	Raw   []byte
}

// CheckExternal reports whether the file changed since our last read or
// write. A change is reported once: the new state becomes the known state.
func (f *File) CheckExternal() (*External, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info, err := os.Stat(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		// Deleted outside the app. The next save recreates it.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !f.changed(info) {
		return nil, nil
	}
	data, info, err := f.read()
	if err != nil || info == nil {
		return nil, err
	}
	f.known = info
	return &External{Notes: Parse(string(data)), Raw: data}, nil
}

// Backup writes data next to the notes file under a timestamped name and
// returns that name. A name already taken gets a counter, so a second
// conflict in the same second never replaces the first backup.
func (f *File) Backup(data []byte) (string, error) {
	stamp := fmt.Sprintf("%s.conflict-%s", f.Path, time.Now().Format("20060102-150405"))
	name := stamp + ".bak"
	for i := 2; ; i++ {
		// Reserve the name, so two backups can't pick the same one.
		r, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			r.Close()
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		name = fmt.Sprintf("%s-%d.bak", stamp, i)
	}
	return name, WriteAtomic(name, data)
}

// WriteAtomic writes to a temp file in the same directory, syncs it and
// renames it over path, so readers see either the old or the new file.
func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}
