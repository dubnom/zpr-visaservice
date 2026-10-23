package changewatcher

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const maxCookie = 64 << 10

// Store is locked for a single organization/source for its entire watcher lifetime.
// Cookies are opaque secrets, never event metadata or public health fields.
type Store struct {
	path    string
	binding string
	lock    *os.File
}

type checkpoint struct {
	Version int    `json:"version"`
	Binding string `json:"binding"`
	Cookie  []byte `json:"cookie"`
}

func OpenStore(dir, organization, source, binding string) (*Store, []byte, error) {
	fail := func() (*Store, []byte, error) {
		return nil, nil, errors.New("trusted watcher checkpoint is unavailable or invalid")
	}
	if dir == "" || organization == "" || source == "" || binding == "" {
		return fail()
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fail()
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fail()
	}
	identity, _ := json.Marshal([]string{organization, source})
	digest := sha256.Sum256(identity)
	path := filepath.Join(dir, hex.EncodeToString(digest[:]))
	fd, err := syscall.Open(path+".lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return fail()
	}
	lock := os.NewFile(uintptr(fd), path+".lock")
	info, err = lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		lock.Close()
		return fail()
	}
	store := &Store{path: path + ".json", binding: binding, lock: lock}
	fd, err = syscall.Open(store.path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil, nil
	}
	if err != nil {
		store.Close()
		return fail()
	}
	file := os.NewFile(uintptr(fd), store.path)
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		store.Close()
		return fail()
	}
	data, err := io.ReadAll(io.LimitReader(file, 128<<10))
	var saved checkpoint
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err != nil || len(data) >= 128<<10 || decoder.Decode(&saved) != nil || decoder.Decode(new(any)) != io.EOF || saved.Version != 1 || saved.Binding != binding || len(saved.Cookie) > maxCookie {
		store.Close()
		return fail()
	}
	return store, saved.Cookie, nil
}

func (store *Store) Save(cookie []byte) error {
	if len(cookie) > maxCookie {
		return errors.New("trusted watcher cookie exceeds limits")
	}
	data, err := json.Marshal(checkpoint{Version: 1, Binding: store.binding, Cookie: cookie})
	if err != nil {
		return err
	}
	pending := store.path + ".pending"
	// A previous interrupted write is never a checkpoint.
	if err := os.Remove(pending); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(pending)
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(pending, store.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(store.path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (store *Store) Close() error { return store.lock.Close() }
