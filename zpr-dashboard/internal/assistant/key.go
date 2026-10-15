package assistant

import (
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
)

// ReadAPIKeyFile reads a caller-selected, owner-only, single-link regular file.
func ReadAPIKeyFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("assistant key requires an owner-only regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 || info.Size() > 4096 {
		return "", errors.New("assistant key file has invalid link count or size")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return "", errors.New("assistant key file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return "", err
	}
	if len(data) > 4096 {
		return "", errors.New("assistant key file exceeds size limit")
	}
	key := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if key == "" || strings.TrimSpace(key) != key || strings.ContainsAny(key, "\r\n\x00") {
		return "", errors.New("assistant key file contains invalid key text")
	}
	return key, nil
}
