package gatewaydraft

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	maxGatewayStoredRecordBytes = 4 << 20
	maxGatewayConfigRevisions   = 50
)

type Revision struct {
	Revision int             `json:"revision"`
	SavedAt  time.Time       `json:"saved_at"`
	Config   json.RawMessage `json:"config"`
}

type Record struct {
	OrganizationID  string     `json:"organization_id"`
	InstanceID      string     `json:"instance_id"`
	CurrentRevision int        `json:"current_revision"`
	Revisions       []Revision `json:"revisions"`
}

type Store struct {
	root string
	mu   sync.Mutex
}

var ErrRevisionConflict = errors.New("gateway configuration revision conflict")

func Open(root string) (*Store, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("gateway configuration store directory must be an absolute path")
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("gateway configuration store must be a real directory")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (store *Store) recordPath(organizationID, instanceID string) (string, string, error) {
	if store == nil || store.root == "" || !ValidIdentifier(organizationID) || !ValidIdentifier(instanceID) {
		return "", "", errors.New("invalid gateway configuration store identity")
	}
	directory := filepath.Join(store.root, organizationID)
	return directory, filepath.Join(directory, instanceID+".json"), nil
}

func (store *Store) List(organizationID string) ([]Record, error) {
	if store == nil || store.root == "" || !ValidIdentifier(organizationID) {
		return nil, errors.New("invalid gateway organization ID")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	directory := filepath.Join(store.root, organizationID)
	if info, err := os.Lstat(directory); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("gateway organization store is not a real directory")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		instanceID := entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
		if !ValidIdentifier(instanceID) {
			continue
		}
		record, err := store.readRecord(organizationID, instanceID)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].InstanceID < records[j].InstanceID })
	return records, nil
}

func (store *Store) Get(organizationID, instanceID string) (Record, error) {
	if store == nil {
		return Record{}, errors.New("gateway configuration store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readRecord(organizationID, instanceID)
}

func (store *Store) Save(organizationID, instanceID string, expectedRevision int, config json.RawMessage) (Record, error) {
	if store == nil {
		return Record{}, errors.New("gateway configuration store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if expectedRevision < 0 {
		return Record{}, errors.New("expected gateway revision cannot be negative")
	}
	directory, filePath, err := store.recordPath(organizationID, instanceID)
	if err != nil {
		return Record{}, err
	}
	if len(config) == 0 || len(config) > MaxConfigBytes {
		return Record{}, errors.New("gateway configuration is empty or too large")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Record{}, err
	}
	if info, err := os.Lstat(directory); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Record{}, errors.New("gateway organization store is not a real directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return Record{}, err
	}
	record, err := store.readRecord(organizationID, instanceID)
	if errors.Is(err, os.ErrNotExist) {
		record = Record{OrganizationID: organizationID, InstanceID: instanceID, Revisions: []Revision{}}
	} else if err != nil {
		return Record{}, err
	}
	if record.CurrentRevision != expectedRevision {
		return Record{}, ErrRevisionConflict
	}
	copyConfig := append(json.RawMessage(nil), config...)
	if !json.Valid(copyConfig) {
		return Record{}, errors.New("gateway configuration is not valid JSON")
	}
	revision := Revision{Revision: record.CurrentRevision + 1, SavedAt: time.Now().UTC(), Config: copyConfig}
	record.CurrentRevision = revision.Revision
	record.Revisions = append(record.Revisions, revision)
	if len(record.Revisions) > maxGatewayConfigRevisions {
		record.Revisions = append([]Revision(nil), record.Revisions[len(record.Revisions)-maxGatewayConfigRevisions:]...)
	}
	if err := store.writeRecord(directory, filePath, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (store *Store) readRecord(organizationID, instanceID string) (Record, error) {
	var record Record
	directory, filePath, err := store.recordPath(organizationID, instanceID)
	if err != nil {
		return record, err
	}
	directoryInfo, err := os.Lstat(directory)
	if err != nil {
		return record, err
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return record, errors.New("gateway organization store is not a real directory")
	}
	pathInfo, err := os.Lstat(filePath)
	if err != nil {
		return record, err
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return record, errors.New("stored gateway configuration is not a regular file")
	}
	file, err := os.Open(filePath)
	if err != nil {
		return record, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > maxGatewayStoredRecordBytes {
		return record, errors.New("stored gateway configuration has unsafe permissions or size")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxGatewayStoredRecordBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return record, errors.New("stored gateway record contains trailing data")
	}
	if record.OrganizationID != organizationID || record.InstanceID != instanceID || record.CurrentRevision < 1 || len(record.Revisions) == 0 || len(record.Revisions) > maxGatewayConfigRevisions {
		return record, errors.New("stored gateway configuration record is invalid")
	}
	for index, revision := range record.Revisions {
		if revision.Revision < 1 || !json.Valid(revision.Config) || len(revision.Config) > MaxConfigBytes || index > 0 && revision.Revision <= record.Revisions[index-1].Revision {
			return record, errors.New("stored gateway configuration revision is invalid")
		}
	}
	if record.Revisions[len(record.Revisions)-1].Revision != record.CurrentRevision {
		return record, errors.New("stored gateway current revision is inconsistent")
	}
	return record, nil
}

func (store *Store) writeRecord(directory, filePath string, record Record) error {
	data, err := json.Marshal(record)
	if err != nil || len(data) > maxGatewayStoredRecordBytes {
		return errors.New("gateway configuration record exceeds its size limit")
	}
	file, err := os.CreateTemp(directory, ".gateway-config-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filePath); err != nil {
		return fmt.Errorf("replace gateway configuration record: %w", err)
	}
	return nil
}
