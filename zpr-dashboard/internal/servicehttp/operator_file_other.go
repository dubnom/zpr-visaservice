//go:build !linux && !darwin

package servicehttp

import "errors"

func ReadOperatorFile(path string, limit int64, private bool) ([]byte, error) {
	return nil, errors.New("protected operator file checks require Linux or macOS")
}
