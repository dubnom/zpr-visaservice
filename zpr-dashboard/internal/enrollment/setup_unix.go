//go:build linux || darwin

package enrollment

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func readSetupFile(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 ||
		(int(stat.Uid) != os.Geteuid() && stat.Uid != 0) {
		return nil, errors.New("setup configuration and CA files must be regular, owned by root or the current user, and not group/other writable")
	}
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, errors.New("setup configuration or CA file exceeds 65536 bytes")
	}
	return data, nil
}

// LoadUserSetupConfig derives private state from the desktop user's home.
func LoadUserSetupConfig(path string) (SetupConfig, error) {
	if os.Geteuid() == 0 {
		return SetupConfig{}, errors.New("desktop setup must run as the logged-in user, not root")
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return SetupConfig{}, errors.New("desktop setup requires an absolute clean home directory")
	}
	state := filepath.Join(home, ".local", "state", "zpr-enrollment-development")
	config, err := loadSetupConfig(path, state)
	if err != nil {
		return SetupConfig{}, err
	}
	parents := []string{home, filepath.Join(home, ".local"), filepath.Join(home, ".local", "state")}
	if runtime.GOOS == "darwin" && config.KeyProtection == "macos-keychain" {
		for _, trustedPath := range []string{path, config.CAFile} {
			if trustedPath == "" {
				continue
			}
			if err := checkMacSetupParents(home, trustedPath); err != nil {
				return SetupConfig{}, err
			}
		}
		library := filepath.Join(home, "Library")
		support := filepath.Join(library, "Application Support")
		parent := filepath.Join(support, "ZPR")
		config.StateDirectory = filepath.Join(parent, "EnrollmentDevelopment")
		parents = []string{home, library, support, parent}
	}

	for _, directory := range parents {
		if directory != home {
			if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return SetupConfig{}, err
			}
		}
		info, err := os.Lstat(directory)
		if err != nil {
			return SetupConfig{}, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || int(stat.Uid) != os.Geteuid() || info.Mode().Perm()&0022 != 0 {
			return SetupConfig{}, errors.New("desktop home/state parents must be owned by the current user, not symlinks or group/other writable")
		}
	}
	return config, nil
}

func checkMacSetupParents(home, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(home, absolute)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("Mac Keychain setup configuration and CA must be provisioned inside the trusted desktop home")
	}
	for parent := filepath.Dir(absolute); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			(int(stat.Uid) != os.Geteuid() && stat.Uid != 0) || info.Mode().Perm()&0022 != 0 {
			return errors.New("Mac Keychain setup home/configuration/CA parents must be trusted, not symlinked or shared-writable")
		}
		if parent == home {
			return nil
		}
	}
}
