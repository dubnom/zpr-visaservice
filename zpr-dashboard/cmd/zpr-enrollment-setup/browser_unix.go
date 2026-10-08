//go:build linux

package main

import "errors"

func openSetupBrowser(string) error {
	return errors.New("-open-browser is supported only by the macOS/Windows desktop launchers; open the private URL manually")
}
