//go:build linux || darwin

package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const endpointBindingFile = "runtime-binding.jsonl"

var errEndpointBindingUncertain = errors.New("endpoint binding reservation is incomplete; automatic creation is forbidden")

// endpointBindingStore stores public metadata only. Its dedicated directory and
// ancestors are trusted operator configuration, not caller-supplied paths.
// No reset, adoption, rotation, deletion, or repair operation is provided.
type endpointBindingStore struct {
	directory string
	reference string
	syncFile  func(*os.File) error
}

type endpointBindingRecord struct {
	Version     int    `json:"version"`
	Reference   string `json:"reference"`
	State       string `json:"state"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// createEndpointBindingStore is explicit first provisioning, never an open
// fallback. Even an existing empty directory is refused. Failed provisioning
// leaves its directory in place so it cannot silently be initialized again.
func createEndpointBindingStore(directory, reference string) (*endpointBindingStore, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || !validText(reference) {
		return nil, ErrInvalid
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return nil, err
	}
	root, err := openIdentityDirectory(directory, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.OpenFile(endpointBindingFile, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	record, err := json.Marshal(endpointBindingRecord{Version: 1, Reference: reference, State: "empty"})
	if err == nil {
		_, err = file.Write(append(record, '\n'))
	}
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		return nil, err
	}
	return openEndpointBindingStore(directory, reference)
}

// openEndpointBindingStore never creates missing state. Complete records left
// by an uncertain fsync are re-synced before use; partial records fail closed.
func openEndpointBindingStore(directory, reference string) (*endpointBindingStore, error) {
	if !validText(reference) {
		return nil, ErrInvalid
	}
	store := &endpointBindingStore{directory: directory, reference: reference, syncFile: (*os.File).Sync}
	err := store.locked(context.Background(), func(file *os.File, _ endpointBindingRecord) error {
		if err := file.Sync(); err != nil {
			return err
		}
		// Persist both the store entry and its dedicated directory entry before
		// an empty store can permit a key reservation.
		for _, path := range []string{directory, filepath.Dir(directory)} {
			dir, err := os.Open(path)
			if err != nil {
				return err
			}
			if err := errors.Join(dir.Sync(), dir.Close()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (s *endpointBindingStore) Load(ctx context.Context, reference string) (fingerprint string, found bool, err error) {
	if reference != s.reference {
		return "", false, errFixtureReferenceConflict
	}
	err = s.locked(ctx, func(_ *os.File, record endpointBindingRecord) error {
		switch record.State {
		case "reserved":
			return errEndpointBindingUncertain
		case "bound":
			fingerprint, found = record.Fingerprint, true
		}
		return nil
	})
	return
}

// Reserve must finish durably before vault.Create. A crash at any later point
// leaves either an exact bound fingerprint or a terminal uncertain reservation,
// even if the vault lost its key or never received the create request.
func (s *endpointBindingStore) Reserve(ctx context.Context, reference string) error {
	if reference != s.reference {
		return errFixtureReferenceConflict
	}
	return s.locked(ctx, func(file *os.File, record endpointBindingRecord) error {
		if record.State != "empty" {
			return errEndpointBindingUncertain
		}
		record.State = "reserved"
		return s.appendRecord(file, record)
	})
}

func (s *endpointBindingStore) Bind(ctx context.Context, reference, fingerprint string) error {
	if reference != s.reference {
		return errFixtureReferenceConflict
	}
	if !validFingerprint(fingerprint) {
		return ErrInvalid
	}
	return s.locked(ctx, func(file *os.File, record endpointBindingRecord) error {
		if record.State == "bound" {
			if record.Fingerprint != fingerprint {
				return errFixtureReferenceConflict
			}
			return s.syncFile(file)
		}
		if record.State != "reserved" {
			return errEndpointBindingUncertain
		}
		record.State, record.Fingerprint = "bound", fingerprint
		return s.appendRecord(file, record)
	})
}

func (s *endpointBindingStore) appendRecord(file *os.File, record endpointBindingRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return s.syncFile(file)
}

func (s *endpointBindingStore) locked(ctx context.Context, action func(*os.File, endpointBindingRecord) error) error {
	root, err := openIdentityDirectory(s.directory, false)
	if err != nil {
		return err
	}
	defer root.Close()
	entry, err := root.Lstat(endpointBindingFile)
	if err != nil {
		return err
	}
	if !entry.Mode().IsRegular() || !privateOwned(entry) {
		return errors.New("endpoint binding requires a private owner-only regular file")
	}
	file, err := root.OpenFile(endpointBindingFile, os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	info, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := root.Lstat(endpointBindingFile)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 || !info.Mode().IsRegular() || !privateOwned(info) ||
		!current.Mode().IsRegular() || !privateOwned(current) ||
		!os.SameFile(entry, info) || !os.SameFile(current, info) {
		return errors.New("endpoint binding file is unsafe or changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil {
		return err
	}
	record, err := decodeEndpointBinding(data, s.reference)
	if err != nil {
		return err
	}
	// A complete previous append may have returned an uncertain sync error.
	// Never turn that into success without establishing durability again.
	if err := s.syncFile(file); err != nil {
		return err
	}
	return action(file, record)
}

func decodeEndpointBinding(data []byte, reference string) (endpointBindingRecord, error) {
	bad := errors.New("corrupt or mismatched endpoint binding journal")
	lines := bytes.Split(data, []byte{'\n'})
	if len(data) > 16384 || len(lines) < 2 || len(lines) > 4 || len(lines[len(lines)-1]) != 0 {
		return endpointBindingRecord{}, bad
	}
	var record endpointBindingRecord
	for i, line := range lines[:len(lines)-1] {
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		record = endpointBindingRecord{}
		if err := decoder.Decode(&record); err != nil {
			return endpointBindingRecord{}, bad
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF || record.Version != 1 || record.Reference != reference ||
			record.State != []string{"empty", "reserved", "bound"}[i] ||
			(i < 2 && record.Fingerprint != "") || (i == 2 && !validFingerprint(record.Fingerprint)) {
			return endpointBindingRecord{}, bad
		}
		// Reject duplicate fields and alternate encodings as well as corruption.
		canonical, err := json.Marshal(record)
		if err != nil || !bytes.Equal(canonical, line) {
			return endpointBindingRecord{}, fmt.Errorf("%w: noncanonical record", bad)
		}
	}
	return record, nil
}
