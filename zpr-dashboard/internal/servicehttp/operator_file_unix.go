//go:build linux || darwin

package servicehttp

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// ReadOperatorFile reads a bounded, protected, single-link regular file.
func ReadOperatorFile(path string, limit int64, private bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("operator configuration file is unavailable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 ||
		(stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) || info.Mode().Perm()&0o022 != 0 ||
		(private && info.Mode().Perm()&0o077 != 0) {
		return nil, errors.New("operator files require a regular, single-link, root/current-user-owned file with protected permissions")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("operator configuration file could not be opened safely")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || info.Mode() != opened.Mode() || info.Size() != opened.Size() {
		return nil, errors.New("operator configuration file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		clear(data)
		return nil, errors.New("operator configuration file could not be read or exceeds its size limit")
	}
	return data, nil
}
